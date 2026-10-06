// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// query serves the paged Jellyfin item search used by federated search.
// Without search parameters it behaves like the plain library listing.
func (f *fakeJellyfin) query(user string, r *http.Request) map[string]any {
	v := r.URL.Query()
	types := strings.Split(v.Get("IncludeItemTypes"), ",")
	term := strings.ToLower(v.Get("SearchTerm"))
	items := []jellyItem{}
	for _, it := range f.movies[user] {
		if (term == "" || strings.Contains(strings.ToLower(it.Name), term)) && slices.Contains(types, it.Type) {
			items = append(items, it)
		}
	}
	total := len(items)
	start, _ := strconv.Atoi(v.Get("StartIndex"))
	limit, err := strconv.Atoi(v.Get("Limit"))
	if err != nil {
		limit = total
	}
	items = items[min(start, total):min(start+limit, total)]
	return map[string]any{"Items": items, "TotalRecordCount": total}
}

// fakeImmich serves two profile keys. Shared assets are visible to both, as
// an Immich partner/album share would be; private ones only to their owner.
type fakeImmich struct {
	mu      sync.Mutex
	visible map[string][]immichAsset // api key -> assets
	slow    map[string]bool          // asset id -> stream original slowly
}

func (f *fakeImmich) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	assets, ok := f.visible[r.Header.Get("x-api-key")]
	assets = slices.Clone(assets)
	f.mu.Unlock()
	if !ok {
		w.WriteHeader(401)
		return
	}
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	find := func(id string) *immichAsset {
		for i := range assets {
			if assets[i].ID == id {
				return &assets[i]
			}
		}
		return nil
	}
	switch {
	case r.URL.Path == "/api/server/features":
		reply(map[string]bool{"smartSearch": false})
	case r.URL.Path == "/api/search/metadata":
		var filter map[string]any
		_ = json.NewDecoder(r.Body).Decode(&filter)
		hits := []immichAsset{}
		for _, a := range assets {
			desc := ""
			if a.ExifInfo != nil {
				desc = a.ExifInfo.Description
			}
			if q, ok := filter["description"].(string); ok && !strings.Contains(strings.ToLower(desc), strings.ToLower(q)) {
				continue
			}
			if q, ok := filter["originalFileName"].(string); ok && !strings.Contains(a.OriginalFileName, q) {
				continue
			}
			if _, ok := filter["city"]; ok {
				continue
			}
			if typ, ok := filter["type"].(string); ok && typ != a.Type {
				continue
			}
			hits = append(hits, a)
		}
		page, size := int(filter["page"].(float64)), int(filter["size"].(float64))
		start := min((page-1)*size, len(hits))
		end := min(start+size, len(hits))
		var next *string
		if end < len(hits) {
			n := strconv.Itoa(page + 1)
			next = &n
		}
		reply(map[string]any{"assets": map[string]any{"items": hits[start:end], "nextPage": next, "total": len(hits)}})
	case strings.HasPrefix(r.URL.Path, "/api/assets/"):
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/assets/"), "/")
		a := find(parts[0])
		if a == nil {
			w.WriteHeader(404)
			return
		}
		if len(parts) == 1 {
			reply(a)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if parts[1] == "original" && f.slow[a.ID] {
			for i := 0; i < 200; i++ {
				if _, err := w.Write(make([]byte, 1024)); err != nil {
					return
				}
				w.(http.Flusher).Flush()
				select {
				case <-r.Context().Done():
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
			return
		}
		_, _ = w.Write([]byte("png-" + a.ID))
	default:
		w.WriteHeader(404)
	}
}

func (f *fakeImmich) revoke(key, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.visible[key] = slices.DeleteFunc(f.visible[key], func(a immichAsset) bool { return a.ID == id })
}

func asset(id, name, desc, typ, updated string) immichAsset {
	a := immichAsset{ID: id, Type: typ, OriginalFileName: name, OriginalMimeType: "image/png", LocalDateTime: "2026-07-01T10:00:00.000Z", UpdatedAt: updated}
	a.ExifInfo = &struct {
		Description    string  `json:"description"`
		City           string  `json:"city"`
		Country        string  `json:"country"`
		Make           string  `json:"make"`
		Model          string  `json:"model"`
		FileSizeInByte float64 `json:"fileSizeInByte"`
		ExifImageWidth *int    `json:"exifImageWidth"`
		ExifImageH     *int    `json:"exifImageHeight"`
	}{Description: desc}
	return a
}

// fakePaperless serves two profile tokens with the same visibility model.
type fakePaperless struct {
	mu      sync.Mutex
	visible map[string][]paperlessDocument
	tasks   map[string]string // task id -> status
}

func (f *fakePaperless) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	docs, ok := f.visible[strings.TrimPrefix(r.Header.Get("Authorization"), "Token ")]
	docs = slices.Clone(docs)
	tasks := f.tasks
	f.mu.Unlock()
	if !ok {
		w.Header().Set("WWW-Authenticate", "Token")
		w.WriteHeader(401)
		return
	}
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch {
	case r.URL.Path == "/api/documents/":
		v := r.URL.Query()
		hits := []paperlessDocument{}
		for _, d := range docs {
			q := strings.ToLower(v.Get("query"))
			if q != "" && !strings.Contains(strings.ToLower(d.Title+" "+d.Content), q) {
				continue
			}
			if from := v.Get("created__date__gte"); from != "" && d.Created < from {
				continue
			}
			if to := v.Get("created__date__lte"); to != "" && d.Created > to {
				continue
			}
			if q != "" {
				d.SearchHit = &struct {
					Highlights string `json:"highlights"`
				}{Highlights: "<span>" + d.Content + "</span>"}
			}
			hits = append(hits, d)
		}
		page, _ := strconv.Atoi(v.Get("page"))
		size, _ := strconv.Atoi(v.Get("page_size"))
		start := min((page-1)*size, len(hits))
		end := min(start+size, len(hits))
		var next *string
		if end < len(hits) {
			n := "more"
			next = &n
		}
		reply(map[string]any{"count": len(hits), "next": next, "results": hits[start:end]})
	case r.URL.Path == "/api/tasks/":
		status, ok := tasks[r.URL.Query().Get("task_id")]
		if !ok {
			reply([]any{})
			return
		}
		related := "7"
		reply([]map[string]any{{"status": status, "related_document": related}})
	case strings.HasPrefix(r.URL.Path, "/api/documents/"):
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/documents/"), "/"), "/")
		n, _ := strconv.Atoi(parts[0])
		for _, d := range docs {
			if d.ID == n {
				if len(parts) == 1 {
					reply(d)
				} else {
					_, _ = w.Write([]byte("pdf-" + parts[0]))
				}
				return
			}
		}
		w.WriteHeader(404)
	default:
		w.WriteHeader(404)
	}
}

func (f *fakePaperless) set(token string, docs []paperlessDocument) {
	f.mu.Lock()
	f.visible[token] = docs
	f.mu.Unlock()
}

const (
	photoShared  = "00000000-0000-4000-8000-000000000001"
	photoPrivA   = "00000000-0000-4000-8000-000000000002"
	photoPrivB   = "00000000-0000-4000-8000-000000000003"
	photoVideoA  = "00000000-0000-4000-8000-000000000004"
	secretTitleA = "Geheimrechnung Alpha"
	secretPhotoA = "Strand privat Alpha"
	secretTitleB = "Arztbrief Beta"
)

type contentEnv struct {
	*testEnv
	immich    *fakeImmich
	paperless *fakePaperless
	photosURL string
	docsURL   string
	docsSrv   *httptest.Server
}

// newContentEnv links both profiles to the fake services, as an owner would.
func newContentEnv(t *testing.T, state string) *contentEnv {
	e := newTestEnv(t, state)
	im := &fakeImmich{visible: map[string][]immichAsset{
		"key-a": {asset(photoShared, "fest.png", "Familienfest Sommer", "IMAGE", "u1"), asset(photoPrivA, "strand.png", secretPhotoA, "IMAGE", "u1"),
			asset(photoVideoA, "clip.mov", "Sommer Clip", "VIDEO", "u1")},
		"key-b": {asset(photoShared, "fest.png", "Familienfest Sommer", "IMAGE", "u1"), asset(photoPrivB, "b.png", "Nur Beta", "IMAGE", "u1")},
	}, slow: map[string]bool{}}
	pl := &fakePaperless{visible: map[string][]paperlessDocument{
		"tok-a": {{ID: 1, Title: secretTitleA, Content: "Stadtwerke Betrag 128,40", Created: "2026-03-01", Modified: "m1"},
			{ID: 3, Title: "Mietvertrag gemeinsam", Content: "Miete 840,00", Created: "2025-01-15", Modified: "m1"}},
		"tok-b": {{ID: 2, Title: secretTitleB, Content: "Praxis Betrag 999,99", Created: "2026-04-01", Modified: "m1"},
			{ID: 3, Title: "Mietvertrag gemeinsam", Content: "Miete 840,00", Created: "2025-01-15", Modified: "m1"}},
	}, tasks: map[string]string{}}
	ps, ds := httptest.NewServer(im), httptest.NewServer(pl)
	t.Cleanup(ps.Close)
	t.Cleanup(ds.Close)
	ce := &contentEnv{testEnv: e, immich: im, paperless: pl, photosURL: ps.URL, docsURL: ds.URL, docsSrv: ds}
	if err := e.hub.store.Update(func(c *Config) error {
		for module, secrets := range map[string][2]string{ModulePhotos: {"key-a", "key-b"}, ModuleDocuments: {"tok-a", "tok-b"}} {
			m := c.Modules[module]
			m.Enabled, m.ServiceURL, m.Instance = true, map[string]string{ModulePhotos: ps.URL, ModuleDocuments: ds.URL}[module], randomID()
			m.Grants[userA], m.Grants[userB] = true, true
			m.Links[userA] = &Link{Account: "a", Secret: secrets[0], KeyID: "ka", Created: time.Now()}
			m.Links[userB] = &Link{Account: "b", Secret: secrets[1], KeyID: "kb", Created: time.Now()}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return ce
}

func (e *contentEnv) search(token, query string) (SearchPage, int, string) {
	e.t.Helper()
	res, body := e.raw("GET", "/content/search?"+query, token, nil, "", nil)
	var page SearchPage
	_ = json.Unmarshal(body, &page)
	return page, res.StatusCode, string(body)
}

func titles(p SearchPage) []string {
	out := []string{}
	for _, it := range p.Items {
		out = append(out, it.Kind+":"+it.Title)
	}
	return out
}

func itemBy(p SearchPage, title string) *ContentItem {
	for i := range p.Items {
		if p.Items[i].Title == title {
			return &p.Items[i]
		}
	}
	return nil
}

func TestContentSearchFederatesWithEachProfilesRights(t *testing.T) {
	e := newContentEnv(t, "")
	a, code, rawA := e.search("token-a", "")
	if code != 200 || a.Completeness != "complete" || a.Next != nil {
		t.Fatalf("%d %s", code, rawA)
	}
	got := titles(a)
	for _, want := range []string{"movie:Nordlicht", "photo:Familienfest Sommer", "photo:" + secretPhotoA, "video:Sommer Clip", "document:" + secretTitleA, "document:Mietvertrag gemeinsam"} {
		if !slices.Contains(got, want) {
			t.Fatalf("A misses %s in %v", want, got)
		}
	}
	// Deterministic per-area groups: media, photos, documents.
	order := []string{}
	for _, it := range a.Items {
		if len(order) == 0 || order[len(order)-1] != it.Area {
			order = append(order, it.Area)
		}
	}
	if fmt.Sprint(order) != "[media photos documents]" {
		t.Fatalf("group order %v", order)
	}
	b, _, rawB := e.search("token-b", "")
	// No private titles, descriptions, snippets or counts of the other profile.
	for _, secret := range []string{secretTitleA, secretPhotoA, "128,40", photoPrivA} {
		if strings.Contains(rawB, secret) {
			t.Fatalf("B sees %q", secret)
		}
	}
	for _, secret := range []string{secretTitleB, "999,99", "Privater Film B", photoPrivB} {
		if strings.Contains(rawA, secret) {
			t.Fatalf("A sees %q", secret)
		}
	}
	// Shared content has one stable reference for both profiles.
	sharedA, sharedB := itemBy(a, "Familienfest Sommer"), itemBy(b, "Familienfest Sommer")
	if sharedA == nil || sharedB == nil || sharedA.Ref.ContentID != sharedB.Ref.ContentID || !validID(sharedA.Ref.ContentID) {
		t.Fatalf("shared refs %v %v", sharedA, sharedB)
	}
	doc := itemBy(a, secretTitleA)
	if doc.Ref.Revision == nil || doc.Ref.MuttiID == "" || doc.Preview != "content/items/"+doc.Ref.ContentID+"/thumbnail" || doc.Document == nil || doc.Document.Content != nil {
		t.Fatalf("document item %+v", doc)
	}
	movie := itemBy(a, "Nordlicht")
	if movie.Media == nil || movie.Media.ItemID != "11111111111111111111111111111111" || movie.Ref.Revision == nil || movie.Preview != "" {
		t.Fatalf("movie item %+v", movie)
	}
	// Repeated searches reuse IDs instead of minting new ones.
	again, _, _ := e.search("token-a", "")
	if itemBy(again, secretTitleA).Ref.ContentID != doc.Ref.ContentID {
		t.Fatal("content id not stable")
	}
}

func TestContentSearchFiltersAndPaging(t *testing.T) {
	e := newContentEnv(t, "")
	p, code, raw := e.search("token-a", "kinds=document&from=2026-01-01")
	if code != 200 || fmt.Sprint(titles(p)) != "[document:"+secretTitleA+"]" || len(p.Areas) != 1 {
		t.Fatalf("%d %s", code, raw)
	}
	p, _, _ = e.search("token-a", "kinds=video")
	if fmt.Sprint(titles(p)) != "[video:Sommer Clip]" {
		t.Fatalf("video filter %v", titles(p))
	}
	p, _, _ = e.search("token-a", "q=sommer&kinds=movie,photo,document")
	if fmt.Sprint(titles(p)) != "[movie:Sommer am See photo:Familienfest Sommer]" {
		t.Fatalf("text %v", titles(p))
	}
	for _, bad := range []string{"kinds=attachment", "from=2026-1-1", "q=" + strings.Repeat("x", 201)} {
		if _, code, _ := e.search("token-a", bad); code != 400 {
			t.Fatalf("%s: %d", bad, code)
		}
	}
	// Paging continues each area separately with a bound cursor.
	first, _, _ := e.search("token-a", "size=2")
	if first.Next == nil || len(first.Items) != 6 {
		t.Fatalf("first page %v", titles(first))
	}
	seen := map[string]bool{}
	for _, it := range first.Items {
		seen[it.Ref.ContentID] = true
	}
	next := url.QueryEscape(*first.Next)
	if _, code, _ := e.search("token-b", "cursor="+next); code != 409 {
		t.Fatalf("foreign cursor %d", code)
	}
	if _, code, _ := e.search("token-a", "cursor="+next[:len(next)-3]+"AAA"); code != 409 {
		t.Fatalf("forged cursor %d", code)
	}
	second, code, raw := e.search("token-a", "cursor="+next)
	if code != 200 || len(second.Items) == 0 {
		t.Fatalf("second %d %s", code, raw)
	}
	for _, it := range second.Items {
		if seen[it.Ref.ContentID] {
			t.Fatalf("repeated %s", it.Title)
		}
	}
	for second.Next != nil {
		second, _, _ = e.search("token-a", "cursor="+url.QueryEscape(*second.Next))
	}
	// A rights change invalidates outstanding cursors.
	first, _, _ = e.search("token-a", "size=1")
	e.mustAdmin("/admin/grants", map[string]any{"module": "documents", "userId": userA, "allowed": false})
	if _, code, _ := e.search("token-a", "cursor="+url.QueryEscape(*first.Next)); code != 409 {
		t.Fatalf("cursor after rights change %d", code)
	}
}

func TestContentSearchReportsOutagesAndMissingAccess(t *testing.T) {
	e := newContentEnv(t, "")
	e.mustAdmin("/admin/grants", map[string]any{"module": "photos", "userId": userB, "allowed": false})
	b, _, _ := e.search("token-b", "")
	if b.Completeness != "complete" || itemBy(b, "Familienfest Sommer") != nil {
		t.Fatalf("not granted area %v", b)
	}
	if i := slices.IndexFunc(b.Areas, func(a SearchArea) bool { return a.Area == ModulePhotos }); i < 0 || b.Areas[i].State != "not_available" {
		t.Fatalf("areas %+v", b.Areas)
	}
	e.docsSrv.Close()
	a, code, _ := e.search("token-a", "")
	if code != 200 || a.Completeness != "partial" || itemBy(a, "Nordlicht") == nil {
		t.Fatalf("outage %d %+v", code, a)
	}
	i := slices.IndexFunc(a.Areas, func(x SearchArea) bool { return x.Area == ModuleDocuments })
	if i < 0 || a.Areas[i].State != "unavailable" || a.Areas[i].Message == "" {
		t.Fatalf("outage area %+v", a.Areas)
	}
}

func TestContentOpenReauthorizesAndTracksRevision(t *testing.T) {
	state := t.TempDir()
	e := newContentEnv(t, state)
	a, _, _ := e.search("token-a", "")
	doc, private := itemBy(a, "Mietvertrag gemeinsam"), itemBy(a, secretPhotoA)
	var out struct {
		Item           ContentItem `json:"item"`
		RevisionStatus string      `json:"revisionStatus"`
	}
	if code := e.do("GET", "/content/items/"+doc.Ref.ContentID+"?revision="+*doc.Ref.Revision, "token-a", nil, &out); code != 200 || out.RevisionStatus != "current" {
		t.Fatalf("open %d %+v", code, out)
	}
	// B may open the shared document but not A's private photo or its bytes.
	if code := e.do("GET", "/content/items/"+doc.Ref.ContentID, "token-b", nil, nil); code != 200 {
		t.Fatalf("shared for B %d", code)
	}
	for _, path := range []string{"", "/thumbnail", "/original"} {
		if code := e.do("GET", "/content/items/"+private.Ref.ContentID+path, "token-b", nil, nil); code != 404 {
			t.Fatalf("B opens private%s: %d", path, code)
		}
	}
	res, body := e.raw("GET", "/content/items/"+private.Ref.ContentID+"/thumbnail", "token-a", nil, "", nil)
	if res.StatusCode != 200 || string(body) != "png-"+photoPrivA {
		t.Fatalf("thumbnail %d %s", res.StatusCode, body)
	}
	// A change in the backend is reported, not hidden.
	e.paperless.set("tok-a", []paperlessDocument{{ID: 1, Title: secretTitleA, Created: "2026-03-01", Modified: "m1"},
		{ID: 3, Title: "Mietvertrag gemeinsam", Created: "2025-01-15", Modified: "m2"}})
	e.do("GET", "/content/items/"+doc.Ref.ContentID+"?revision="+*doc.Ref.Revision, "token-a", nil, &out)
	if out.RevisionStatus != "changed" {
		t.Fatalf("revision %+v", out)
	}
	// Backend-side revocation: no metadata, no bytes.
	e.paperless.set("tok-a", nil)
	if code := e.do("GET", "/content/items/"+doc.Ref.ContentID+"/original", "token-a", nil, nil); code != 404 {
		t.Fatalf("revoked original %d", code)
	}
	if code := e.do("GET", "/content/items/notanid", "token-a", nil, nil); code != 404 {
		t.Fatalf("bad id %d", code)
	}
	// Pointing the module at another backend retires earlier IDs, also after
	// a restart with the persisted index.
	if err := e.hub.store.Update(func(c *Config) error { c.Modules[ModulePhotos].Instance = randomID(); return nil }); err != nil {
		t.Fatal(err)
	}
	if code := e.do("GET", "/content/items/"+private.Ref.ContentID, "token-a", nil, nil); code != 404 {
		t.Fatalf("replaced backend %d", code)
	}
	reopened, err := openContentIndex(state)
	if key, ok := reopened.lookup(doc.Ref.ContentID); err != nil || !ok || key.Object != "3" {
		t.Fatalf("persisted index %v %v %v", key, ok, err)
	}
}

func TestContentStreamEndsAfterBackendRevocation(t *testing.T) {
	e := newContentEnv(t, "")
	e.immich.slow[photoPrivA] = true
	a, _, _ := e.search("token-a", "kinds=photo")
	private := itemBy(a, secretPhotoA)
	req, _ := http.NewRequest("GET", e.server.URL+"/mutti/hub/v1/content/items/"+private.Ref.ContentID+"/original", nil)
	req.Header.Set("Authorization", "Bearer token-a")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("%v %v", err, res)
	}
	defer res.Body.Close()
	time.AfterFunc(500*time.Millisecond, func() { e.immich.revoke("key-a", photoPrivA) })
	start := time.Now()
	n, _ := io.Copy(io.Discard, res.Body)
	elapsed := time.Since(start)
	// Full transfer would take 20 s; the object check runs every 4 s.
	if elapsed > 6*time.Second || n >= 200*1024 {
		t.Fatalf("stream continued %v, %d bytes", elapsed, n)
	}
}

func TestRevokedSourcesLeaveModelHistory(t *testing.T) {
	e := newContentEnv(t, "")
	id, err := e.hub.jf.identify(context.Background(), "token-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	book := &sourceBook{Items: map[string]*Source{}}
	q1 := book.add(Source{Service: "documents", Kind: "document", ObjectID: "1", Title: secretTitleA, Revision: "m1"})
	q2 := book.add(Source{Service: "media", Kind: "movie", ObjectID: "11111111111111111111111111111111", Title: "Nordlicht"})
	e.hub.stampSources(book)
	if q1.Content == nil || q1.Locator == nil || q1.Retrieved == nil || q1.Content.Revision == nil || q2.Content == nil {
		t.Fatalf("provenance %+v %+v", q1, q2)
	}
	c := &Conversation{ID: randomID(), Owner: userA, Sources: *book, Messages: []*Message{
		{ID: "u1", Role: "user", Text: "Was kostet Strom?", Status: "completed"},
		{ID: "m1", Role: "assistant", Text: "128,40 Euro laut " + q1.Ref, Status: "completed", Used: []string{q1.Ref}, Memo: bookMemo(book, []string{q1.Ref})},
		{ID: "u2", Role: "user", Text: "Und Filme?", Status: "completed"},
		{ID: "m2", Role: "assistant", Text: "Nordlicht " + q2.Ref, Status: "completed", Sources: []string{q2.Ref}},
	}}
	if omit := e.hub.ai.revokedHistory(context.Background(), id, c, ""); len(omit) != 0 {
		t.Fatalf("nothing revoked yet %v", omit)
	}
	e.paperless.set("tok-a", nil) // the document is no longer shared with A
	omit := e.hub.ai.revokedHistory(context.Background(), id, c, "")
	if !omit["m1"] || omit["m2"] {
		t.Fatalf("omit %v", omit)
	}
	msgs := e.hub.ai.buildMessages(c, "", catalog[0], &profileTools{}, omit)
	all, _ := json.Marshal(msgs)
	if strings.Contains(string(all), "128,40") || strings.Contains(string(all), secretTitleA) || !strings.Contains(string(all), "Nordlicht") {
		t.Fatalf("history %s", all)
	}
	// Losing the module grant also removes the answer from reuse.
	e.paperless.set("tok-a", []paperlessDocument{{ID: 1, Title: secretTitleA, Created: "2026-03-01", Modified: "m1"}})
	e.mustAdmin("/admin/grants", map[string]any{"module": "documents", "userId": userA, "allowed": false})
	if omit := e.hub.ai.revokedHistory(context.Background(), id, c, ""); !omit["m1"] {
		t.Fatal("grant loss not applied to history")
	}
}

func TestChatSourcesCarryContentReferences(t *testing.T) {
	e := newContentEnv(t, "")
	e.configureAI()
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	first, _ := e.ask("token-a", cid, "Suche alle ungesehenen Filme unter 100 Sekunden.", "c1")
	e.events(runID(first), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "done" })
	var full struct {
		Sources map[string]*Source `json:"sources"`
	}
	e.do("GET", "/ai/conversations/"+cid, "token-a", nil, &full)
	s := full.Sources["Q1"]
	if s == nil || s.Content == nil || s.Content.Revision == nil || s.Retrieved == nil {
		t.Fatalf("source %+v", full.Sources)
	}
	var view map[string]any
	if code := e.do("GET", "/ai/sources/"+cid+"/Q1", "token-a", nil, &view); code != 200 || view["status"] != "current" || view["item"] == nil || view["ref"] != "Q1" {
		t.Fatalf("resolve %d %v", code, view)
	}
	// The same object found by manual search has the same reference.
	p, _, _ := e.search("token-a", "q=Nordlicht&kinds=movie")
	if len(p.Items) != 1 || p.Items[0].Ref.ContentID != s.Content.ContentID {
		t.Fatalf("manual %v vs %s", titles(p), s.Content.ContentID)
	}
	// Item changed in the library after the answer.
	e.jf.mu.Lock()
	e.jf.movies[userA][0].Etag = "etag-new"
	e.jf.mu.Unlock()
	e.do("GET", "/ai/sources/"+cid+"/Q1", "token-a", nil, &view)
	if view["status"] != "changed" {
		t.Fatalf("changed %v", view)
	}
}

func TestActionJournalPreventsRepeatAfterCrash(t *testing.T) {
	state := t.TempDir()
	e := newTestEnv(t, state)
	e.configureAI()
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	first, _ := e.ask("token-a", cid, "Suche alle ungesehenen Filme unter 100 Sekunden.", "j1")
	e.events(runID(first), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "done" })
	second, _ := e.ask("token-a", cid, "Markiere Nordlicht als Favorit.", "j2")
	events, _ := e.events(runID(second), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "done" })
	prop := events[len(events)-1].Data["proposals"].([]any)[0].(map[string]any)
	pid := prop["id"].(string)
	if prop["target"] == nil {
		t.Fatalf("proposal without target %v", prop)
	}
	// Simulate a crash after the intent was stored but before the outcome.
	if err := e.hub.journal.begin(ActionEntry{ID: pid, Profile: userA, Task: "media.favorite", Target: contentKey{Area: AreaMedia}}); err != nil {
		t.Fatal(err)
	}
	restarted, err := openJournal(state)
	if err != nil {
		t.Fatal(err)
	}
	if entry, _ := restarted.get(pid); entry.State != "outcome_unknown" {
		t.Fatalf("after restart %+v", entry)
	}
	e.hub.journal = restarted
	var p map[string]any
	if code := e.do("POST", "/ai/proposals/"+pid+"/confirm", "token-a", map[string]string{"conversation": cid}, &p); code != 200 || p["state"] != "outcome_unknown" {
		t.Fatalf("confirm after crash %d %v", code, p)
	}
	if e.jf.favorites[userA+"11111111111111111111111111111111"] {
		t.Fatal("action repeated after unknown outcome")
	}
	var jobs struct {
		Items []ContentJob `json:"items"`
	}
	e.do("GET", "/content/jobs", "token-a", nil, &jobs)
	if len(jobs.Items) != 1 || jobs.Items[0].Type != "action.favorite" || jobs.Items[0].State != "outcome_unknown" {
		t.Fatalf("jobs %+v", jobs.Items)
	}
	e.do("GET", "/content/jobs", "token-b", nil, &jobs)
	if len(jobs.Items) != 0 {
		t.Fatalf("B sees A's jobs %+v", jobs.Items)
	}
}

func TestActionJournalRecordsConfirmedAction(t *testing.T) {
	e := newTestEnv(t, "")
	e.configureAI()
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	first, _ := e.ask("token-a", cid, "Suche alle ungesehenen Filme unter 100 Sekunden.", "k1")
	e.events(runID(first), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "done" })
	second, _ := e.ask("token-a", cid, "Markiere Nordlicht als Favorit.", "k2")
	events, _ := e.events(runID(second), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "done" })
	pid := events[len(events)-1].Data["proposals"].([]any)[0].(map[string]any)["id"].(string)
	var p map[string]any
	if code := e.do("POST", "/ai/proposals/"+pid+"/confirm", "token-a", map[string]string{"conversation": cid}, &p); code != 200 || p["state"] != "confirmed" {
		t.Fatalf("confirm %d %v", code, p)
	}
	entry, ok := e.hub.journal.get(pid)
	if !ok || entry.State != "confirmed" || entry.Revision == "" || entry.ContentID == "" || entry.Args["favorite"] != true {
		t.Fatalf("journal %+v", entry)
	}
}

func TestContentJobsListImportsPerProfile(t *testing.T) {
	e := newContentEnv(t, "")
	for _, task := range []uploadTask{{ID: "11111111-1111-4111-8111-111111111111", User: userA, FileName: "rechnung.pdf", Created: time.Now()},
		{ID: "22222222-2222-4222-8222-222222222222", User: userB, FileName: "geheim-b.pdf", Created: time.Now()}} {
		if err := e.hub.docs.recordTask(task); err != nil {
			t.Fatal(err)
		}
	}
	e.paperless.tasks["11111111-1111-4111-8111-111111111111"] = "SUCCESS"
	_, body := e.raw("GET", "/content/jobs", "token-a", nil, "", nil)
	var jobs struct {
		Items []ContentJob `json:"items"`
	}
	_ = json.Unmarshal(body, &jobs)
	if len(jobs.Items) != 1 || jobs.Items[0].State != "completed" || jobs.Items[0].Content == nil || strings.Contains(string(body), "geheim-b") {
		t.Fatalf("jobs %s", body)
	}
}

func TestCapabilitiesAdvertiseContentWithoutModel(t *testing.T) {
	e := newTestEnv(t, "")
	var caps struct {
		Modules map[string]moduleCapability `json:"modules"`
	}
	e.do("GET", "/capabilities", "token-a", nil, &caps)
	c := caps.Modules["content"]
	if c.State != "ready" || !slices.Contains(c.Actions, "search") {
		t.Fatalf("content capability %+v", c)
	}
}
