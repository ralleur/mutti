// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Content areas group kinds for clients. They are module names, never backend
// product names; backends stay an administrative detail.
const AreaMedia = "media"

var areaOrder = []string{AreaMedia, ModulePhotos, ModuleDocuments}

var kindArea = map[string]string{"movie": AreaMedia, "series": AreaMedia, "photo": ModulePhotos, "video": ModulePhotos, "document": ModuleDocuments}

// ContentRef is the stable, opaque reference of contracts.md. Revision is nil
// when the backend offers no reliable revision; it is never guessed.
type ContentRef struct {
	MuttiID   string  `json:"muttiId"`
	ContentID string  `json:"contentId"`
	Revision  *string `json:"revision"`
}

// ContentTime states what a timestamp means and how its zone is known:
// "utc" (absolute instant), "local" (wall clock without zone) or "date".
type ContentTime struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Zone  string `json:"zone"`
}

// MediaTarget lets the native player open an item of its own library.
type MediaTarget struct {
	ItemID string `json:"itemId"`
	Type   string `json:"type"`
}

type ContentItem struct {
	Ref          ContentRef    `json:"ref"`
	Kind         string        `json:"kind"`
	Area         string        `json:"area"`
	Title        string        `json:"title"`
	Subtitle     string        `json:"subtitle,omitempty"`
	Snippet      string        `json:"snippet,omitempty"`
	Times        []ContentTime `json:"timestamps"`
	Actions      []string      `json:"actions"`
	Availability string        `json:"availability"`
	Preview      string        `json:"preview,omitempty"`
	Media        *MediaTarget  `json:"media,omitempty"`
	Photo        *PhotoAsset   `json:"photo,omitempty"`
	Document     *Document     `json:"document,omitempty"`
}

// SourceLocator says which part of a content item a source covers. Scope
// "object" means the whole item: page or time accuracy is not available.
type SourceLocator struct {
	Scope string `json:"scope"`
	Page  *int   `json:"page,omitempty"`
}

// contentKey is the server-side mapping target of a content ID.
type contentKey struct {
	Area     string `json:"area"`
	Instance string `json:"instance"`
	Object   string `json:"object"`
}

// contentIndex persists opaque content IDs. IDs are random, never reused and
// carry no rights: every use re-authorizes against the backend.
type contentIndex struct {
	mu   sync.Mutex
	path string
	ids  map[string]contentKey
	keys map[contentKey]string
}

type contentIndexFile struct {
	Version int                   `json:"version"`
	Items   map[string]contentKey `json:"items"`
}

func openContentIndex(dir string) (*contentIndex, error) {
	x := &contentIndex{path: filepath.Join(dir, "content-ids.json"), ids: map[string]contentKey{}, keys: map[contentKey]string{}}
	b, err := os.ReadFile(x.path)
	switch {
	case os.IsNotExist(err):
		return x, nil
	case err != nil:
		return nil, err
	}
	var f contentIndexFile
	if err = json.Unmarshal(b, &f); err != nil || f.Version != 1 {
		return nil, errors.New("invalid content index")
	}
	for id, key := range f.Items {
		x.ids[id] = key
		x.keys[key] = id
	}
	return x, nil
}

// assign returns the IDs for keys, creating and persisting new ones at once.
func (x *contentIndex) assign(keys []contentKey) ([]string, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	out := make([]string, len(keys))
	added := []string{}
	for i, key := range keys {
		id, ok := x.keys[key]
		if !ok {
			id = randomID()
			x.ids[id], x.keys[key] = key, id
			added = append(added, id)
		}
		out[i] = id
	}
	if len(added) == 0 {
		return out, nil
	}
	if err := writePrivateJSON(x.path, contentIndexFile{Version: 1, Items: x.ids}); err != nil {
		for _, id := range added {
			delete(x.keys, x.ids[id])
			delete(x.ids, id)
		}
		return nil, err
	}
	return out, nil
}

func (x *contentIndex) lookup(id string) (contentKey, bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	key, ok := x.ids[id]
	return key, ok
}

// instance returns the current backend identity of an area.
func instanceOf(cfg Config, area string) string {
	if area == AreaMedia {
		return cfg.MediaInstance
	}
	if m := cfg.Modules[area]; m != nil {
		return m.Instance
	}
	return ""
}

func revisionRef(area, raw string) *string {
	if raw == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(area + "\x00" + raw))
	rev := hex.EncodeToString(sum[:8])
	return &rev
}

// rightsRevision changes whenever anything that decides this profile's
// effective access changes: Jellyfin policy, device, module state, grant,
// account link or backend instance. Cursors bind to it.
func (h *Hub) rightsRevision(id Identity) string {
	cfg := h.store.Read()
	sum := sha256.New()
	fmt.Fprintf(sum, "%s\x00%s\x00%s\x00%s\x00", id.UserID, id.Device, id.Policy, cfg.MediaInstance)
	for _, name := range moduleIDs {
		m := cfg.Modules[name]
		link := ""
		if l := m.Links[id.UserID]; l != nil {
			link = l.KeyID + "/" + l.Account + "/" + l.Created.String()
		}
		fmt.Fprintf(sum, "%s:%t:%t:%s:%s:%s\x00", name, m.Enabled, m.Grants[id.UserID], m.ServiceURL, m.Instance, link)
	}
	return hex.EncodeToString(sum.Sum(nil)[:12])
}

// areaAccess reports whether the profile may search an area now.
func (h *Hub) areaAccess(id Identity, area string) error {
	if area == AreaMedia {
		return nil
	}
	_, err := h.allowed(id, area)
	return err
}

// ---- Search ----

type searchQuery struct {
	Text  string   `json:"q"`
	Kinds []string `json:"k"`
	From  string   `json:"f,omitempty"`
	To    string   `json:"t,omitempty"`
	Size  int      `json:"s"`
}

func (q searchQuery) wants(kind string) bool { return slices.Contains(q.Kinds, kind) }

func (q searchQuery) areas() []string {
	out := []string{}
	for _, area := range areaOrder {
		for _, k := range q.Kinds {
			if kindArea[k] == area {
				out = append(out, area)
				break
			}
		}
	}
	return out
}

type searchCursor struct {
	Query   searchQuery    `json:"q"`
	User    string         `json:"u"`
	Rights  string         `json:"r"`
	Next    map[string]int `json:"n"`
	Expires int64          `json:"e"`
}

type SearchArea struct {
	Area    string `json:"area"`
	State   string `json:"state"` // searched, unavailable, not_available
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	More    bool   `json:"more"`
}

type SearchPage struct {
	Items        []ContentItem `json:"items"`
	Next         *string       `json:"next"`
	Completeness string        `json:"completeness"` // complete, partial
	Areas        []SearchArea  `json:"areas"`
}

var errCursor = apiErr(409, "cursor_invalid", "Die Suche ist abgelaufen oder die Freigaben haben sich geändert. Bitte neu suchen.")

const cursorLifetime = 30 * time.Minute

func (h *Hub) sealCursor(c searchCursor) string {
	b, _ := json.Marshal(c)
	mac := hmac.New(sha256.New, h.cursorKey)
	mac.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (h *Hub) openCursor(s string, id Identity) (searchCursor, error) {
	body, sig, ok := strings.Cut(s, ".")
	if !ok || len(s) > 4096 {
		return searchCursor{}, errCursor
	}
	b, err1 := base64.RawURLEncoding.DecodeString(body)
	got, err2 := base64.RawURLEncoding.DecodeString(sig)
	mac := hmac.New(sha256.New, h.cursorKey)
	mac.Write(b)
	if err1 != nil || err2 != nil || !hmac.Equal(got, mac.Sum(nil)) {
		return searchCursor{}, errCursor
	}
	var c searchCursor
	if json.Unmarshal(b, &c) != nil || c.User != id.UserID || c.Rights != h.rightsRevision(id) || time.Now().Unix() > c.Expires {
		return searchCursor{}, errCursor
	}
	return c, nil
}

func parseSearch(r *http.Request) (searchQuery, error) {
	v := r.URL.Query()
	q := searchQuery{Text: strings.TrimSpace(v.Get("q")), From: v.Get("from"), To: v.Get("to"), Size: queryInt(r, "size", 20, 1, 50)}
	if len(q.Text) > 200 || (q.From != "" && !dateParam.MatchString(q.From)) || (q.To != "" && !dateParam.MatchString(q.To)) {
		return q, errInvalid
	}
	if kinds := v.Get("kinds"); kinds != "" {
		for _, k := range strings.Split(kinds, ",") {
			if _, ok := kindArea[k]; !ok {
				return q, errInvalid
			}
			if !slices.Contains(q.Kinds, k) {
				q.Kinds = append(q.Kinds, k)
			}
		}
	} else {
		q.Kinds = []string{"movie", "series", "photo", "video", "document"}
	}
	slices.Sort(q.Kinds)
	return q, nil
}

// pendingItem is a result whose content ID is not assigned yet.
type pendingItem struct {
	key  contentKey
	rev  string
	item ContentItem
}

// contentSearch federates over the existing adapters with the caller's own
// backend accounts. No shared index; groups follow a fixed area order and
// each backend's own order inside its group. An outage is "partial", never
// an empty result.
func (h *Hub) contentSearch(w http.ResponseWriter, r *http.Request, id Identity) error {
	var cursor searchCursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		var err error
		if cursor, err = h.openCursor(raw, id); err != nil {
			return err
		}
	} else {
		q, err := parseSearch(r)
		if err != nil {
			return err
		}
		cursor = searchCursor{Query: q, Next: map[string]int{}}
		for _, area := range q.areas() {
			cursor.Next[area] = 0
		}
	}
	q := cursor.Query
	cfg := h.store.Read()
	type result struct {
		items []pendingItem
		next  int
		err   error
	}
	results := map[string]*result{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	page := SearchPage{Items: []ContentItem{}, Completeness: "complete", Areas: []SearchArea{}}
	lang := responseLanguage(r)
	for _, area := range q.areas() {
		pos, ok := cursor.Next[area]
		if !ok {
			continue // exhausted on an earlier page
		}
		if err := h.areaAccess(id, area); err != nil {
			entry := SearchArea{Area: area, State: "not_available", Code: "not_available"}
			var api *APIError
			if errors.As(err, &api) {
				entry.Code, entry.Message = api.Code, lang.say(api.Message)
			}
			page.Areas = append(page.Areas, entry)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			items, next, err := h.searchArea(ctx, id, cfg, area, q, pos)
			mu.Lock()
			results[area] = &result{items, next, err}
			mu.Unlock()
		}()
	}
	wg.Wait()
	pending := []pendingItem{}
	next := searchCursor{Query: q, User: id.UserID, Rights: h.rightsRevision(id), Next: map[string]int{}, Expires: time.Now().Add(cursorLifetime).Unix()}
	for _, area := range areaOrder {
		res := results[area]
		if res == nil {
			continue
		}
		entry := SearchArea{Area: area, State: "searched"}
		if res.err != nil {
			page.Completeness = "partial"
			entry.State, entry.Code, entry.Message = "unavailable", "unavailable", lang.say(areaUnavailable(area))
			if errors.Is(res.err, errUnauthorized) {
				return res.err
			}
			// Not searched is not finished: later pages try this area again.
			next.Next[area] = cursor.Next[area]
			entry.More = true
		} else {
			pending = append(pending, res.items...)
			if res.next > 0 {
				entry.More = true
				next.Next[area] = res.next
			}
		}
		page.Areas = append(page.Areas, entry)
	}
	items, err := h.finishItems(cfg, pending)
	if err != nil {
		return err
	}
	page.Items = items
	if len(next.Next) > 0 {
		s := h.sealCursor(next)
		page.Next = &s
	}
	return writeOK(w, page)
}

func areaUnavailable(area string) string {
	if area == AreaMedia {
		return "Die Mediathek ist gerade nicht erreichbar."
	}
	return unavailableMessage(area)
}

// finishItems assigns content IDs and server-built preview paths.
func (h *Hub) finishItems(cfg Config, pending []pendingItem) ([]ContentItem, error) {
	keys := make([]contentKey, len(pending))
	for i, p := range pending {
		keys[i] = p.key
	}
	ids, err := h.content.assign(keys)
	if err != nil {
		return nil, err
	}
	out := make([]ContentItem, len(pending))
	for i, p := range pending {
		item := p.item
		item.Ref = ContentRef{MuttiID: cfg.MuttiID, ContentID: ids[i], Revision: revisionRef(p.key.Area, p.rev)}
		if slices.Contains(item.Actions, "thumbnail") {
			item.Preview = "content/items/" + ids[i] + "/thumbnail"
		}
		out[i] = item
	}
	return out, nil
}

// searchArea returns one page of an area and the next position (0 = none).
func (h *Hub) searchArea(ctx context.Context, id Identity, cfg Config, area string, q searchQuery, pos int) ([]pendingItem, int, error) {
	instance := instanceOf(cfg, area)
	out := []pendingItem{}
	switch area {
	case AreaMedia:
		types := []string{}
		if q.wants("movie") {
			types = append(types, "Movie")
		}
		if q.wants("series") {
			types = append(types, "Series")
		}
		params := url.Values{"IncludeItemTypes": {strings.Join(types, ",")}, "Recursive": {"true"}, "Fields": {"Etag,DateCreated,PremiereDate,ProductionYear"},
			"StartIndex": {strconv.Itoa(pos)}, "Limit": {strconv.Itoa(q.Size)}, "EnableImages": {"false"}, "EnableTotalRecordCount": {"true"}}
		if q.Text != "" {
			params.Set("SearchTerm", q.Text)
		} else {
			params.Set("SortBy", "DateCreated,SortName")
			params.Set("SortOrder", "Descending,Ascending")
		}
		if q.From != "" {
			params.Set("MinPremiereDate", q.From+"T00:00:00Z")
		}
		if q.To != "" {
			params.Set("MaxPremiereDate", q.To+"T23:59:59Z")
		}
		var res struct {
			Items []jellyItem `json:"Items"`
			Total int         `json:"TotalRecordCount"`
		}
		if err := h.jf.call(ctx, "GET", "/Users/"+id.UserID+"/Items?"+params.Encode(), id.token, nil, &res); err != nil {
			return nil, 0, err
		}
		for _, it := range res.Items {
			out = append(out, mediaPending(instance, it))
		}
		next := 0
		if len(res.Items) > 0 && pos+len(res.Items) < res.Total {
			next = pos + len(res.Items)
		}
		return out, next, nil
	case ModulePhotos:
		pq := photoQuery{Text: q.Text, From: q.From, To: q.To}
		switch {
		case q.wants("photo") && !q.wants("video"):
			pq.Type = "IMAGE"
		case q.wants("video") && !q.wants("photo"):
			pq.Type = "VIDEO"
		}
		number := max(pos, 1)
		items, more, _, err := h.photos.findPage(ctx, id, pq, number, q.Size)
		if err != nil {
			return nil, 0, err
		}
		for _, p := range items {
			out = append(out, photoPending(instance, p))
		}
		if more {
			return out, number + 1, nil
		}
		return out, 0, nil
	case ModuleDocuments:
		number := max(pos, 1)
		res, err := h.docs.searchDated(ctx, id, q.Text, q.From, q.To, number, q.Size)
		if err != nil {
			return nil, 0, err
		}
		for _, d := range res.Items {
			out = append(out, documentPending(instance, d))
		}
		if res.Next != nil {
			return out, number + 1, nil
		}
		return out, 0, nil
	}
	return nil, 0, errNotFound
}

func mediaPending(instance string, it jellyItem) pendingItem {
	kind := "movie"
	if it.Type == "Series" {
		kind = "series"
	}
	m := it.movie()
	item := ContentItem{Kind: kind, Area: AreaMedia, Title: it.Name, Subtitle: movieSubtitle(m), Times: []ContentTime{}, Actions: []string{"open", "play"},
		Availability: "ready", Media: &MediaTarget{ItemID: m.ID, Type: it.Type}}
	if it.PremiereDate != "" {
		item.Times = append(item.Times, ContentTime{Type: "premiere", Value: dateOnly(it.PremiereDate), Zone: "date"})
	}
	if it.DateCreated != "" {
		item.Times = append(item.Times, ContentTime{Type: "added", Value: it.DateCreated, Zone: "utc"})
	}
	return pendingItem{key: contentKey{Area: AreaMedia, Instance: instance, Object: m.ID}, rev: it.Etag, item: item}
}

func photoPending(instance string, p PhotoAsset) pendingItem {
	kind, actions := "photo", []string{"open", "thumbnail", "preview", "original"}
	if p.Type == "video" {
		kind, actions = "video", append(actions, "video")
	}
	title := p.Description
	if title == "" {
		title = p.FileName
	}
	photo := p
	item := ContentItem{Kind: kind, Area: ModulePhotos, Title: title, Subtitle: p.City, Times: []ContentTime{}, Actions: actions, Availability: "ready", Photo: &photo}
	if p.Taken != "" {
		// Immich's local date time is the wall clock where it was taken.
		item.Times = append(item.Times, ContentTime{Type: "taken", Value: p.Taken, Zone: "local"})
	}
	return pendingItem{key: contentKey{Area: ModulePhotos, Instance: instance, Object: p.ID}, rev: p.revision, item: item}
}

func documentPending(instance string, d Document) pendingItem {
	doc := d
	doc.Content = nil
	item := ContentItem{Kind: "document", Area: ModuleDocuments, Title: d.Title, Snippet: d.Snippet, Times: []ContentTime{},
		Actions: []string{"open", "thumbnail", "preview", "original"}, Availability: "ready", Document: &doc}
	if d.Created != "" {
		item.Times = append(item.Times, ContentTime{Type: "created", Value: dateOnly(d.Created), Zone: "date"})
	}
	if d.Added != "" {
		item.Times = append(item.Times, ContentTime{Type: "added", Value: d.Added, Zone: "utc"})
	}
	return pendingItem{key: contentKey{Area: ModuleDocuments, Instance: instance, Object: strconv.Itoa(d.ID)}, rev: d.revision, item: item}
}

// ---- Opening ----

// resolveContent maps an ID to its key if it belongs to the current backend
// instance and the profile may use the area now. Unknown, replaced and
// forbidden IDs are indistinguishable to the caller.
func (h *Hub) resolveContent(id Identity, contentID string) (contentKey, error) {
	if !validID(contentID) {
		return contentKey{}, errNotFound
	}
	key, ok := h.content.lookup(contentID)
	if !ok || key.Instance != instanceOf(h.store.Read(), key.Area) {
		return contentKey{}, errNotFound
	}
	if err := h.areaAccess(id, key.Area); err != nil {
		return contentKey{}, errNotFound
	}
	return key, nil
}

// probe re-authorizes one object with the caller's backend account and
// returns its current item and raw revision.
func (h *Hub) probe(ctx context.Context, id Identity, key contentKey) (pendingItem, error) {
	switch key.Area {
	case AreaMedia:
		var it jellyItem
		if err := h.jf.call(ctx, "GET", "/Users/"+id.UserID+"/Items/"+url.PathEscape(key.Object), id.token, nil, &it); err != nil {
			if errors.Is(err, errUnauthorized) || errors.Is(err, errNotFound) {
				return pendingItem{}, err
			}
			return pendingItem{}, apiErr(502, "unavailable", areaUnavailable(AreaMedia))
		}
		return mediaPending(key.Instance, it), nil
	case ModulePhotos:
		if !uuidPattern.MatchString(key.Object) {
			return pendingItem{}, errNotFound
		}
		base, headers, err := h.photos.session(id)
		if err != nil {
			return pendingItem{}, err
		}
		var raw immichAsset
		if err = serviceCall(ctx, h.photos.client, "GET", base+"/api/assets/"+key.Object, headers, nil, &raw); err != nil {
			return pendingItem{}, err
		}
		return photoPending(key.Instance, raw.public()), nil
	case ModuleDocuments:
		n, err := strconv.Atoi(key.Object)
		if err != nil {
			return pendingItem{}, errNotFound
		}
		d, err := h.docs.fetch(ctx, id, n, false)
		if err != nil {
			return pendingItem{}, err
		}
		return documentPending(key.Instance, d), nil
	}
	return pendingItem{}, errNotFound
}

// probeAccess only re-authorizes. For documents a single request suffices;
// the full item with names costs one request per tag and correspondent.
func (h *Hub) probeAccess(ctx context.Context, id Identity, key contentKey) error {
	if key.Area == ModuleDocuments {
		if _, err := strconv.Atoi(key.Object); err != nil {
			return errNotFound
		}
		base, headers, err := h.docs.session(id)
		if err != nil {
			return err
		}
		return serviceCall(ctx, h.docs.client, "GET", base+"/api/documents/"+key.Object+"/", headers, nil, nil)
	}
	_, err := h.probe(ctx, id, key)
	return err
}

// revoked reports errors that mean "this profile may not see it any more",
// as opposed to a temporary outage.
func revoked(err error) bool {
	var api *APIError
	if errors.Is(err, errUnauthorized) {
		return true
	}
	return errors.As(err, &api) && (api.Status == 401 || api.Status == 403 || api.Status == 404)
}

func revisionStatus(requested string, current *string) string {
	switch {
	case current == nil:
		return "unknown"
	case requested == *current:
		return "current"
	default:
		return "changed"
	}
}

func (h *Hub) contentItem(w http.ResponseWriter, r *http.Request, id Identity) error {
	key, err := h.resolveContent(id, r.PathValue("id"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	p, err := h.probe(ctx, id, key)
	if err != nil {
		return err
	}
	items, err := h.finishItems(h.store.Read(), []pendingItem{p})
	if err != nil {
		return err
	}
	out := map[string]any{"item": items[0]}
	if rev := r.URL.Query().Get("revision"); rev != "" {
		out["revisionStatus"] = revisionStatus(rev, items[0].Ref.Revision)
	}
	return writeOK(w, out)
}

// contentMedia opens previews and originals through the area adapter. Besides
// the module guard, the object itself is re-checked while bytes flow, so a
// backend-side revocation ends the transfer within about five seconds.
func (h *Hub) contentMedia(w http.ResponseWriter, r *http.Request, id Identity) error {
	key, err := h.resolveContent(id, r.PathValue("id"))
	if err != nil {
		return err
	}
	kind := r.PathValue("kind")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	check, stop := context.WithTimeout(ctx, 10*time.Second)
	_, err = h.probe(check, id, key)
	stop()
	if err != nil {
		return err
	}
	go func() {
		t := time.NewTicker(4 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			check, stop := context.WithTimeout(ctx, 3*time.Second)
			err := h.probeAccess(check, id, key)
			stop()
			if err != nil && revoked(err) {
				cancel()
				return
			}
		}
	}()
	r = r.WithContext(ctx)
	r.SetPathValue("id", key.Object)
	r.SetPathValue("kind", kind)
	switch key.Area {
	case ModulePhotos:
		return h.photos.media(w, r, id)
	case ModuleDocuments:
		return h.docs.media(w, r, id)
	}
	// Movies and shows play through the native client's own media library.
	return errNotFound
}

// ---- Sources and history ----

// stampSources gives the sources a run touched their content reference,
// retrieval time and locator. Without an assignable ID provenance stays
// absent rather than invented.
func (h *Hub) stampSources(book *sourceBook) {
	cfg := h.store.Read()
	refs, keys := []string{}, []contentKey{}
	for _, ref := range slices.Sorted(maps.Keys(book.touched)) {
		s := book.Items[ref]
		if s == nil || !slices.Contains(areaOrder, s.Service) {
			continue
		}
		instance := instanceOf(cfg, s.Service)
		if s.Instance != "" && s.Instance != instance {
			continue // from a replaced backend: never re-stamped
		}
		refs = append(refs, ref)
		keys = append(keys, contentKey{Area: s.Service, Instance: instance, Object: s.ObjectID})
	}
	ids, err := h.content.assign(keys)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for i, ref := range refs {
		s := book.Items[ref]
		s.Content = &ContentRef{MuttiID: cfg.MuttiID, ContentID: ids[i], Revision: revisionRef(keys[i].Area, s.Revision)}
		s.Retrieved = &now
		s.Locator = &SourceLocator{Scope: "object"}
	}
}

// sourceKey maps a stored source to its object in the current backend
// instance. A source of a replaced backend no longer maps.
func (h *Hub) sourceKey(s *Source) (contentKey, bool) {
	key := contentKey{Area: s.Service, Instance: instanceOf(h.store.Read(), s.Service), Object: s.ObjectID}
	if !slices.Contains(areaOrder, s.Service) || (s.Instance != "" && s.Instance != key.Instance) {
		return key, false
	}
	if s.Content != nil {
		if k, ok := h.content.lookup(s.Content.ContentID); !ok || k != key {
			return key, false
		}
	}
	return key, true
}

// revokedHistory returns earlier answers whose sources this profile can no
// longer use. Every source is re-authorized with the profile's current
// backend accounts; an unverifiable source counts as revoked (fail closed).
func (a *AI) revokedHistory(ctx context.Context, id Identity, c *Conversation, runID string) map[string]bool {
	run := findRun(c, runID)
	byMessage := map[string][]string{}
	refs := map[string]bool{}
	for _, m := range c.Messages {
		if run != nil && m.ID == run.User {
			break
		}
		if m.Role != "assistant" {
			continue
		}
		for _, ref := range slices.Concat(m.Sources, m.Used) {
			byMessage[m.ID] = append(byMessage[m.ID], ref)
			refs[ref] = true
		}
	}
	allowed := map[string]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	limit := make(chan struct{}, 6)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for ref := range refs {
		s := c.Sources.Items[ref]
		ok := false
		switch {
		case s == nil:
		case s.Service == "attachment":
			ok = findAttachment(c, s.ObjectID) != nil
		default:
			key, mapped := a.hub.sourceKey(s)
			if !mapped || a.hub.areaAccess(id, key.Area) != nil {
				break
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				limit <- struct{}{}
				defer func() { <-limit }()
				err := a.hub.probeAccess(ctx, id, key)
				mu.Lock()
				allowed[ref] = err == nil
				mu.Unlock()
			}()
			continue
		}
		mu.Lock()
		allowed[ref] = ok
		mu.Unlock()
	}
	wg.Wait()
	omit := map[string]bool{}
	for msg, list := range byMessage {
		for _, ref := range list {
			if !allowed[ref] {
				omit[msg] = true
			}
		}
	}
	return omit
}

// ---- Jobs ----

type ContentJob struct {
	ID           string      `json:"id"`
	Type         string      `json:"type"`  // ai.answer, document.import, action.favorite
	State        string      `json:"state"` // queued, running, completed, failed, cancelled, outcome_unknown, unknown
	Title        string      `json:"title,omitempty"`
	Message      string      `json:"message,omitempty"`
	Conversation string      `json:"conversation,omitempty"`
	Content      *ContentRef `json:"content,omitempty"`
	Updated      *time.Time  `json:"updated,omitempty"`
}

// contentJobs lists the profile's running and recent work in one shape:
// AI answers, document imports and confirmed actions.
func (h *Hub) contentJobs(w http.ResponseWriter, r *http.Request, id Identity) error {
	jobs := []ContentJob{}
	areas := []SearchArea{}
	lang := responseLanguage(r)
	h.ai.mu.Lock()
	queued := map[string]bool{}
	for _, l := range h.ai.queue {
		queued[l.id] = true
	}
	for _, l := range h.ai.live {
		if l.owner != id.UserID {
			continue
		}
		state := "running"
		if queued[l.id] {
			state = "queued"
		}
		jobs = append(jobs, ContentJob{ID: l.id, Type: "ai.answer", State: state, Conversation: l.conversation})
	}
	h.ai.mu.Unlock()
	slices.SortFunc(jobs, func(a, b ContentJob) int { return strings.Compare(a.ID, b.ID) })
	if err := h.areaAccess(id, ModuleDocuments); err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		tasks, err := h.docs.taskList(ctx, id)
		cancel()
		if err != nil {
			areas = append(areas, SearchArea{Area: ModuleDocuments, State: "unavailable", Code: "unavailable", Message: lang.say(areaUnavailable(ModuleDocuments))})
		}
		cfg := h.store.Read()
		for _, t := range tasks {
			job := ContentJob{ID: fmt.Sprint(t["id"]), Type: "document.import", Title: fmt.Sprint(t["fileName"]), State: "unknown"}
			if msg, ok := t["message"].(string); ok {
				job.Message = lang.say(msg)
			}
			switch t["state"] {
			case "processing":
				job.State = "running"
			case "done":
				job.State = "completed"
			case "failed", "duplicate":
				job.State = "failed"
			}
			if created, ok := t["created"].(time.Time); ok {
				job.Updated = &created
			}
			if n, ok := t["document"].(int); ok {
				if ids, err := h.content.assign([]contentKey{{Area: ModuleDocuments, Instance: instanceOf(cfg, ModuleDocuments), Object: strconv.Itoa(n)}}); err == nil {
					job.Content = &ContentRef{MuttiID: cfg.MuttiID, ContentID: ids[0]}
				}
			}
			jobs = append(jobs, job)
		}
	}
	for _, e := range h.journal.list(id.UserID, 20) {
		updated := e.Updated
		job := ContentJob{ID: e.ID, Type: "action." + strings.TrimPrefix(e.Task, "media."), State: e.State, Title: e.Title, Message: lang.say(e.Result), Updated: &updated}
		if e.ContentID != "" {
			job.Content = &ContentRef{MuttiID: h.store.Read().MuttiID, ContentID: e.ContentID}
		}
		jobs = append(jobs, job)
	}
	return writeOK(w, map[string]any{"items": jobs, "areas": areas})
}

func newCursorKey() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
