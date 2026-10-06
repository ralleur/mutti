// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Photos adapts Immich. Every call uses the profile's own scoped API key, so
// Immich itself enforces ownership and album sharing.
type Photos struct {
	hub    *Hub
	client *http.Client
	stream *http.Client
}

// Immich permissions requested for a profile key: read, view, download and
// upload its own library. No delete, share, admin or key management rights.
var photoPermissions = []string{"asset.read", "asset.view", "asset.download", "asset.upload", "album.read", "user.read"}

type PhotoAsset struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Taken       string  `json:"taken"`
	FileName    string  `json:"fileName"`
	Mime        string  `json:"mime"`
	Description string  `json:"description,omitempty"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	Duration    string  `json:"duration,omitempty"`
	Favorite    bool    `json:"favorite"`
	City        string  `json:"city,omitempty"`
	Country     string  `json:"country,omitempty"`
	Camera      string  `json:"camera,omitempty"`
	Live        bool    `json:"livePhoto,omitempty"`
	Size        float64 `json:"size,omitempty"`
}

type immichAsset struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	OriginalFileName string `json:"originalFileName"`
	OriginalMimeType string `json:"originalMimeType"`
	LocalDateTime    string `json:"localDateTime"`
	FileCreatedAt    string `json:"fileCreatedAt"`
	Duration         string `json:"duration"`
	IsFavorite       bool   `json:"isFavorite"`
	Width            *int   `json:"width"`
	Height           *int   `json:"height"`
	LivePhotoVideoID string `json:"livePhotoVideoId"`
	ExifInfo         *struct {
		Description    string  `json:"description"`
		City           string  `json:"city"`
		Country        string  `json:"country"`
		Make           string  `json:"make"`
		Model          string  `json:"model"`
		FileSizeInByte float64 `json:"fileSizeInByte"`
		ExifImageWidth *int    `json:"exifImageWidth"`
		ExifImageH     *int    `json:"exifImageHeight"`
	} `json:"exifInfo"`
}

func (a immichAsset) public() PhotoAsset {
	p := PhotoAsset{ID: a.ID, Type: strings.ToLower(a.Type), Taken: a.LocalDateTime, FileName: a.OriginalFileName, Mime: a.OriginalMimeType,
		Favorite: a.IsFavorite, Live: a.LivePhotoVideoID != ""}
	if p.Taken == "" {
		p.Taken = a.FileCreatedAt
	}
	if p.Type == "video" && a.Duration != "" && a.Duration != "0:00:00.00000" {
		p.Duration = a.Duration
	}
	if a.Width != nil && a.Height != nil {
		p.Width, p.Height = *a.Width, *a.Height
	}
	if e := a.ExifInfo; e != nil {
		p.Description, p.City, p.Country, p.Size = e.Description, e.City, e.Country, e.FileSizeInByte
		p.Camera = strings.TrimSpace(e.Make + " " + e.Model)
		if p.Width == 0 && e.ExifImageWidth != nil && e.ExifImageH != nil {
			p.Width, p.Height = *e.ExifImageWidth, *e.ExifImageH
		}
	}
	return p
}

func (p *Photos) ping(ctx context.Context, m *ModuleConfig) error {
	var out struct {
		Res string `json:"res"`
	}
	if err := serviceCall(ctx, p.client, "GET", m.ServiceURL+"/api/server/ping", nil, nil, &out); err != nil {
		return err
	}
	if out.Res != "pong" {
		return errors.New("not immich")
	}
	return nil
}

func (p *Photos) link(ctx context.Context, service, email, password, profileName string) (*Link, error) {
	var login struct {
		AccessToken string `json:"accessToken"`
		UserID      string `json:"userId"`
		IsAdmin     bool   `json:"isAdmin"`
	}
	if err := serviceCall(ctx, p.client, "POST", service+"/api/auth/login", nil, map[string]string{"email": email, "password": password}, &login); err != nil {
		return nil, apiErr(400, "login_failed", "Anmeldung bei der Fotobibliothek fehlgeschlagen. Bitte E-Mail und Passwort prüfen.")
	}
	session := map[string]string{"Authorization": "Bearer " + login.AccessToken}
	defer serviceCall(context.WithoutCancel(ctx), p.client, "POST", service+"/api/auth/logout", session, nil, nil)
	if login.IsAdmin {
		// One shared administrator key would see every user's library.
		return nil, apiErr(400, "admin_account", "Bitte ein normales Immich-Konto verwenden, nicht das Administratorkonto.")
	}
	var key struct {
		Secret string `json:"secret"`
		APIKey struct {
			ID string `json:"id"`
		} `json:"apiKey"`
	}
	name := "Mutti – " + strings.TrimSpace(profileName)
	if err := serviceCall(ctx, p.client, "POST", service+"/api/api-keys", session, map[string]any{"name": name, "permissions": photoPermissions}, &key); err != nil || key.Secret == "" {
		return nil, apiErr(502, "link_failed", "Die Fotobibliothek hat keinen Zugriffsschlüssel ausgestellt.")
	}
	return &Link{Account: email, Secret: key.Secret, KeyID: key.APIKey.ID, Remote: login.UserID, Created: time.Now().UTC()}, nil
}

func (p *Photos) revokeKey(ctx context.Context, service string, l *Link) error {
	if l.KeyID == "" {
		return errors.New("no key id")
	}
	return serviceCall(ctx, p.client, "DELETE", service+"/api/api-keys/"+url.PathEscape(l.KeyID), map[string]string{"x-api-key": l.Secret}, nil, nil)
}

func (p *Photos) session(id Identity) (string, map[string]string, error) {
	m, err := p.hub.allowed(id, ModulePhotos)
	if err != nil {
		return "", nil, err
	}
	return m.ServiceURL, map[string]string{"x-api-key": m.Links[id.UserID].Secret}, nil
}

type searchPage struct {
	Items    []PhotoAsset `json:"items"`
	NextPage *int         `json:"nextPage"`
	Total    int          `json:"total"`
}

func (p *Photos) metadata(ctx context.Context, id Identity, filter map[string]any) (searchPage, error) {
	base, headers, err := p.session(id)
	if err != nil {
		return searchPage{}, err
	}
	var out struct {
		Assets struct {
			Items    []immichAsset `json:"items"`
			NextPage *string       `json:"nextPage"`
			Total    int           `json:"total"`
		} `json:"assets"`
	}
	filter["withExif"] = true
	if _, ok := filter["visibility"]; !ok {
		filter["visibility"] = "timeline"
	}
	if err = serviceCall(ctx, p.client, "POST", base+"/api/search/metadata", headers, filter, &out); err != nil {
		return searchPage{}, err
	}
	page := searchPage{Items: []PhotoAsset{}, Total: out.Assets.Total}
	for _, a := range out.Assets.Items {
		page.Items = append(page.Items, a.public())
	}
	if out.Assets.NextPage != nil {
		var n int
		if _, err := fmt.Sscan(*out.Assets.NextPage, &n); err == nil {
			page.NextPage = &n
		}
	}
	return page, nil
}

func (p *Photos) list(w http.ResponseWriter, r *http.Request, id Identity) error {
	filter := map[string]any{"page": queryInt(r, "page", 1, 1, 100000), "size": queryInt(r, "size", 60, 1, 200), "order": "desc"}
	switch r.URL.Query().Get("type") {
	case "image":
		filter["type"] = "IMAGE"
	case "video":
		filter["type"] = "VIDEO"
	}
	if r.URL.Query().Get("favorite") == "true" {
		filter["isFavorite"] = true
	}
	page, err := p.metadata(r.Context(), id, filter)
	if err != nil {
		return err
	}
	return writeOK(w, page)
}

var dateParam = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Search uses Immich's own search: smart search when its ML is available,
// otherwise description and file-name matches. No second photo index.
func (p *Photos) search(w http.ResponseWriter, r *http.Request, id Identity) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if len(q) > 200 || (from != "" && !dateParam.MatchString(from)) || (to != "" && !dateParam.MatchString(to)) {
		return errInvalid
	}
	results, mode, err := p.find(r.Context(), id, q, from, to, queryInt(r, "size", 60, 1, 200))
	if err != nil {
		return err
	}
	return writeOK(w, map[string]any{"items": results, "mode": mode})
}

func (p *Photos) find(ctx context.Context, id Identity, q, from, to string, size int) ([]PhotoAsset, string, error) {
	base, headers, err := p.session(id)
	if err != nil {
		return nil, "", err
	}
	dates := map[string]any{}
	if from != "" {
		dates["takenAfter"] = from + "T00:00:00.000Z"
	}
	if to != "" {
		dates["takenBefore"] = to + "T23:59:59.999Z"
	}
	var features struct {
		SmartSearch bool `json:"smartSearch"`
	}
	_ = serviceCall(ctx, p.client, "GET", base+"/api/server/features", headers, nil, &features)
	if q != "" && features.SmartSearch {
		body := map[string]any{"query": q, "size": size, "withExif": true}
		for k, v := range dates {
			body[k] = v
		}
		var out struct {
			Assets struct {
				Items []immichAsset `json:"items"`
			} `json:"assets"`
		}
		if err := serviceCall(ctx, p.client, "POST", base+"/api/search/smart", headers, body, &out); err == nil {
			items := []PhotoAsset{}
			for _, a := range out.Assets.Items {
				items = append(items, a.public())
			}
			return items, "smart", nil
		}
	}
	seen := map[string]bool{}
	items := []PhotoAsset{}
	filters := []map[string]any{{}}
	if q != "" {
		filters = []map[string]any{{"description": q}, {"originalFileName": q}, {"city": q}}
	}
	for _, f := range filters {
		f["size"], f["order"] = size, "desc"
		for k, v := range dates {
			f[k] = v
		}
		page, err := p.metadata(ctx, id, f)
		if err != nil {
			var api *APIError
			if errors.As(err, &api) && api.Code == "not_found" {
				continue // e.g. an unknown city is no error for a text query
			}
			return nil, "", err
		}
		for _, a := range page.Items {
			if !seen[a.ID] {
				seen[a.ID] = true
				items = append(items, a)
			}
		}
	}
	return items, "metadata", nil
}

func (p *Photos) albums(w http.ResponseWriter, r *http.Request, id Identity) error {
	base, headers, err := p.session(id)
	if err != nil {
		return err
	}
	var raw []struct {
		ID         string `json:"id"`
		Name       string `json:"albumName"`
		Count      int    `json:"assetCount"`
		Cover      string `json:"albumThumbnailAssetId"`
		Start      string `json:"startDate"`
		End        string `json:"endDate"`
		Shared     bool   `json:"shared"`
		OwnerID    string `json:"ownerId"`
		Descriptor string `json:"description"`
	}
	if err = serviceCall(r.Context(), p.client, "GET", base+"/api/albums", headers, nil, &raw); err != nil {
		return err
	}
	out := []map[string]any{}
	for _, a := range raw {
		out = append(out, map[string]any{"id": a.ID, "name": a.Name, "count": a.Count, "cover": a.Cover, "start": a.Start, "end": a.End,
			"shared": a.Shared, "description": a.Descriptor})
	}
	return writeOK(w, map[string]any{"items": out})
}

func (p *Photos) album(w http.ResponseWriter, r *http.Request, id Identity) error {
	album := r.PathValue("id")
	if !uuidPattern.MatchString(album) {
		return errNotFound
	}
	base, headers, err := p.session(id)
	if err != nil {
		return err
	}
	var raw struct {
		ID     string        `json:"id"`
		Name   string        `json:"albumName"`
		Shared bool          `json:"shared"`
		Assets []immichAsset `json:"assets"`
	}
	if err = serviceCall(r.Context(), p.client, "GET", base+"/api/albums/"+album, headers, nil, &raw); err != nil {
		return err
	}
	items := []PhotoAsset{}
	for _, a := range raw.Assets {
		items = append(items, a.public())
	}
	return writeOK(w, map[string]any{"id": raw.ID, "name": raw.Name, "shared": raw.Shared, "items": items})
}

func (p *Photos) asset(w http.ResponseWriter, r *http.Request, id Identity) error {
	asset := r.PathValue("id")
	if !uuidPattern.MatchString(asset) {
		return errNotFound
	}
	base, headers, err := p.session(id)
	if err != nil {
		return err
	}
	var raw immichAsset
	if err = serviceCall(r.Context(), p.client, "GET", base+"/api/assets/"+asset, headers, nil, &raw); err != nil {
		return err
	}
	return writeOK(w, raw.public())
}

func (p *Photos) media(w http.ResponseWriter, r *http.Request, id Identity) error {
	asset, kind := r.PathValue("id"), r.PathValue("kind")
	if !uuidPattern.MatchString(asset) {
		return errNotFound
	}
	base, headers, err := p.session(id)
	if err != nil {
		return err
	}
	ctx, done := p.hub.guard(r.Context(), id, ModulePhotos)
	defer done()
	r = r.WithContext(ctx)
	switch kind {
	case "thumbnail", "preview":
		return relay(w, r, p.stream, base+"/api/assets/"+asset+"/thumbnail?size="+kind, headers, "")
	case "video":
		return relay(w, r, p.stream, base+"/api/assets/"+asset+"/video/playback", headers, "")
	case "original":
		var meta immichAsset
		if err = serviceCall(ctx, p.client, "GET", base+"/api/assets/"+asset, headers, nil, &meta); err != nil {
			return err
		}
		return relay(w, r, p.stream, base+"/api/assets/"+asset+"/original", headers, meta.OriginalFileName)
	}
	return errNotFound
}

var deviceAssetPattern = regexp.MustCompile(`^[A-Za-z0-9._:/+=-]{1,200}$`)

// upload streams one file into the profile's own Immich library. Text fields
// must precede the file part; the device id is set by Mutti, not the client.
func (p *Photos) upload(w http.ResponseWriter, r *http.Request, id Identity) error {
	base, headers, err := p.session(id)
	if err != nil {
		return err
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<30)
	reader, err := r.MultipartReader()
	if err != nil {
		return errInvalid
	}
	fields := map[string]string{}
	for {
		part, err := reader.NextPart()
		if err != nil {
			return errInvalid
		}
		name := part.FormName()
		if name != "assetData" {
			if name != "deviceAssetId" && name != "fileCreatedAt" && name != "fileModifiedAt" && name != "duration" {
				return errInvalid
			}
			b, err := io.ReadAll(io.LimitReader(part, 256))
			if err != nil {
				return errInvalid
			}
			fields[name] = string(b)
			continue
		}
		if !deviceAssetPattern.MatchString(fields["deviceAssetId"]) || !validTimestamp(fields["fileCreatedAt"]) || !validTimestamp(fields["fileModifiedAt"]) {
			return apiErr(400, "invalid", "Für den Upload fehlen Dateikennung oder Aufnahmedatum.")
		}
		fields["deviceId"] = "mutti-" + safeDevice(id.Device)
		filename := part.FileName()
		if filename == "" || len(filename) > 255 || strings.ContainsAny(filename, "/\\") {
			return errInvalid
		}
		return p.forwardUpload(w, r.Context(), base, headers, fields, filename, part)
	}
}

func safeDevice(device string) string {
	if device == "" {
		return "local"
	}
	if len(device) > 16 {
		device = device[:16]
	}
	return device
}

func validTimestamp(s string) bool {
	_, err := time.Parse(time.RFC3339Nano, s)
	return err == nil
}

func (p *Photos) forwardUpload(w http.ResponseWriter, ctx context.Context, base string, headers map[string]string, fields map[string]string, filename string, file io.Reader) error {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		for _, k := range []string{"deviceAssetId", "deviceId", "fileCreatedAt", "fileModifiedAt", "duration"} {
			if v, ok := fields[k]; ok {
				_ = mw.WriteField(k, v)
			}
		}
		part, err := mw.CreateFormFile("assetData", filename)
		if err == nil {
			_, err = io.Copy(part, file)
		}
		if err == nil {
			err = mw.Close()
		}
		pw.CloseWithError(err)
	}()
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/api/assets", pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := p.stream.Do(req)
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			return apiErr(413, "too_large", "Die Datei ist zu groß.")
		}
		return apiErr(502, "upload_interrupted", "Der Upload wurde unterbrochen. Bitte erneut versuchen; doppelte Fotos werden erkannt.")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 201 {
		return serviceError(res.StatusCode)
	}
	var out struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := decodeLimited(res.Body, &out); err != nil {
		return err
	}
	return writeOK(w, out)
}
