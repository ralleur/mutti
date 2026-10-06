// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Documents adapts Paperless-ngx with each profile's own token; Paperless
// enforces document ownership and permissions itself.
type Documents struct {
	hub    *Hub
	client *http.Client
	stream *http.Client
	mu     sync.Mutex
}

const paperlessAccept = "application/json; version=9"

type Document struct {
	ID            int      `json:"id"`
	Title         string   `json:"title"`
	Created       string   `json:"created"`
	Added         string   `json:"added"`
	Pages         *int     `json:"pages"`
	Mime          string   `json:"mime"`
	FileName      string   `json:"fileName"`
	Snippet       string   `json:"snippet,omitempty"`
	Correspondent string   `json:"correspondent,omitempty"`
	DocumentType  string   `json:"documentType,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Content       *string  `json:"content,omitempty"`
}

type paperlessDocument struct {
	ID               int    `json:"id"`
	Title            string `json:"title"`
	Content          string `json:"content"`
	Created          string `json:"created"`
	Added            string `json:"added"`
	PageCount        *int   `json:"page_count"`
	MimeType         string `json:"mime_type"`
	OriginalFileName string `json:"original_file_name"`
	Correspondent    *int   `json:"correspondent"`
	DocumentType     *int   `json:"document_type"`
	Tags             []int  `json:"tags"`
	SearchHit        *struct {
		Highlights string `json:"highlights"`
	} `json:"__search_hit__"`
}

var tagPattern = regexp.MustCompile(`<[^>]*>`)

func (d paperlessDocument) public() Document {
	doc := Document{ID: d.ID, Title: d.Title, Created: d.Created, Added: d.Added, Pages: d.PageCount, Mime: d.MimeType, FileName: d.OriginalFileName}
	if d.SearchHit != nil {
		// Highlights are upstream HTML; clients receive plain text only.
		doc.Snippet = html.UnescapeString(tagPattern.ReplaceAllString(d.SearchHit.Highlights, ""))
	}
	return doc
}

func (d *Documents) headers(secret string) map[string]string {
	return map[string]string{"Authorization": "Token " + secret, "Accept": paperlessAccept}
}

func (d *Documents) ping(ctx context.Context, m *ModuleConfig) error {
	req, err := http.NewRequestWithContext(ctx, "GET", m.ServiceURL+"/api/documents/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", paperlessAccept)
	res, err := d.client.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	// Without credentials the document API answers with a token challenge.
	if res.StatusCode != 401 || res.Header.Get("WWW-Authenticate") != "Token" {
		return errors.New("not paperless")
	}
	return nil
}

func (d *Documents) link(ctx context.Context, service, username, password string) (*Link, error) {
	var token struct {
		Token string `json:"token"`
	}
	if err := serviceCall(ctx, d.client, "POST", service+"/api/token/", map[string]string{"Accept": paperlessAccept}, map[string]string{"username": username, "password": password}, &token); err != nil || token.Token == "" {
		return nil, apiErr(400, "login_failed", "Anmeldung beim Dokumentenarchiv fehlgeschlagen. Bitte Benutzername und Passwort prüfen.")
	}
	var settings struct {
		User struct {
			Username    string `json:"username"`
			IsSuperuser bool   `json:"is_superuser"`
		} `json:"user"`
	}
	err := serviceCall(ctx, d.client, "GET", service+"/api/ui_settings/", d.headers(token.Token), nil, &settings)
	var api *APIError
	switch {
	case err == nil && settings.User.IsSuperuser:
		// A superuser token would expose every document to this profile.
		return nil, apiErr(400, "admin_account", "Bitte ein normales Paperless-Konto verwenden, kein Superuser-Konto.")
	case err == nil:
	case errors.As(err, &api) && api.Code == "service_denied":
		// Ordinary accounts may lack the UI-settings permission; a superuser
		// never does, so this account is not one.
		settings.User.Username = username
	default:
		return nil, err
	}
	return &Link{Account: settings.User.Username, Secret: token.Token, Remote: settings.User.Username, Created: time.Now().UTC()}, nil
}

func (d *Documents) session(id Identity) (string, map[string]string, error) {
	m, err := d.hub.allowed(id, ModuleDocuments)
	if err != nil {
		return "", nil, err
	}
	return m.ServiceURL, d.headers(m.Links[id.UserID].Secret), nil
}

type documentPage struct {
	Items []Document `json:"items"`
	Count int        `json:"count"`
	Next  *int       `json:"nextPage"`
}

func (d *Documents) search(ctx context.Context, id Identity, query string, page, size int) (documentPage, error) {
	base, headers, err := d.session(id)
	if err != nil {
		return documentPage{}, err
	}
	params := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(size)}, "truncate_content": {"true"}}
	if query != "" {
		params.Set("query", query)
	} else {
		params.Set("ordering", "-created")
	}
	var out struct {
		Count   int                 `json:"count"`
		Next    *string             `json:"next"`
		Results []paperlessDocument `json:"results"`
	}
	if err = serviceCall(ctx, d.client, "GET", base+"/api/documents/?"+params.Encode(), headers, nil, &out); err != nil {
		return documentPage{}, err
	}
	result := documentPage{Items: []Document{}, Count: out.Count}
	for _, doc := range out.Results {
		result.Items = append(result.Items, doc.public())
	}
	if out.Next != nil {
		n := page + 1
		result.Next = &n
	}
	return result, nil
}

func (d *Documents) list(w http.ResponseWriter, r *http.Request, id Identity) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 200 {
		return errInvalid
	}
	page, err := d.search(r.Context(), id, q, queryInt(r, "page", 1, 1, 100000), queryInt(r, "size", 25, 1, 100))
	if err != nil {
		return err
	}
	return writeOK(w, page)
}

func documentID(r *http.Request) (int, error) {
	n, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || n <= 0 || n > 1<<30 {
		return 0, errNotFound
	}
	return n, nil
}

// fetch returns one document with names for its metadata and its full text.
func (d *Documents) fetch(ctx context.Context, id Identity, doc int, withContent bool) (Document, error) {
	base, headers, err := d.session(id)
	if err != nil {
		return Document{}, err
	}
	var raw paperlessDocument
	if err = serviceCall(ctx, d.client, "GET", base+"/api/documents/"+strconv.Itoa(doc)+"/", headers, nil, &raw); err != nil {
		return Document{}, err
	}
	out := raw.public()
	name := func(kind string, ref int) string {
		var named struct {
			Name string `json:"name"`
		}
		if serviceCall(ctx, d.client, "GET", base+"/api/"+kind+"/"+strconv.Itoa(ref)+"/", headers, nil, &named) != nil {
			return ""
		}
		return named.Name
	}
	if raw.Correspondent != nil {
		out.Correspondent = name("correspondents", *raw.Correspondent)
	}
	if raw.DocumentType != nil {
		out.DocumentType = name("document_types", *raw.DocumentType)
	}
	for _, t := range raw.Tags {
		if n := name("tags", t); n != "" {
			out.Tags = append(out.Tags, n)
		}
	}
	if withContent {
		content := raw.Content
		if len(content) > 200_000 {
			content = content[:200_000]
		}
		out.Content = &content
	}
	return out, nil
}

func (d *Documents) document(w http.ResponseWriter, r *http.Request, id Identity) error {
	doc, err := documentID(r)
	if err != nil {
		return err
	}
	out, err := d.fetch(r.Context(), id, doc, true)
	if err != nil {
		return err
	}
	return writeOK(w, out)
}

func (d *Documents) media(w http.ResponseWriter, r *http.Request, id Identity) error {
	doc, err := documentID(r)
	if err != nil {
		return err
	}
	base, headers, err := d.session(id)
	if err != nil {
		return err
	}
	ctx, done := d.hub.guard(r.Context(), id, ModuleDocuments)
	defer done()
	r = r.WithContext(ctx)
	path := base + "/api/documents/" + strconv.Itoa(doc)
	switch r.PathValue("kind") {
	case "thumbnail":
		return relay(w, r, d.stream, path+"/thumb/", headers, "")
	case "preview":
		return relay(w, r, d.stream, path+"/preview/", headers, "")
	case "original":
		meta, err := d.fetch(ctx, id, doc, false)
		if err != nil {
			return err
		}
		return relay(w, r, d.stream, path+"/download/?original=true", headers, meta.FileName)
	}
	return errNotFound
}

// Upload tasks are recorded per profile because the Paperless task API can
// reveal metadata of other users' imports to anyone with task permission.
type uploadTask struct {
	ID       string    `json:"id"`
	User     string    `json:"user"`
	FileName string    `json:"fileName"`
	Created  time.Time `json:"created"`
}

func (d *Documents) taskPath() string { return filepath.Join(d.hub.opts.State, "document-tasks.json") }

func (d *Documents) loadTasks() []uploadTask {
	var tasks []uploadTask
	b, err := os.ReadFile(d.taskPath())
	if err == nil {
		_ = json.Unmarshal(b, &tasks)
	}
	return tasks
}

func (d *Documents) recordTask(t uploadTask) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	keep := []uploadTask{}
	for _, old := range d.loadTasks() {
		if time.Since(old.Created) < 14*24*time.Hour {
			keep = append(keep, old)
		}
	}
	return writePrivateJSON(d.taskPath(), append(keep, t))
}

var taskPattern = regexp.MustCompile(`^[0-9a-f-]{36}$`)

func (d *Documents) upload(w http.ResponseWriter, r *http.Request, id Identity) error {
	base, headers, err := d.session(id)
	if err != nil {
		return err
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512<<20)
	reader, err := r.MultipartReader()
	if err != nil {
		return errInvalid
	}
	title := ""
	for {
		part, err := reader.NextPart()
		if err != nil {
			return errInvalid
		}
		switch part.FormName() {
		case "title":
			b, _ := io.ReadAll(io.LimitReader(part, 256))
			title = strings.TrimSpace(string(b))
			continue
		case "document":
		default:
			return errInvalid
		}
		filename := part.FileName()
		if filename == "" || len(filename) > 255 || strings.ContainsAny(filename, "/\\") {
			return errInvalid
		}
		task, err := d.forward(r.Context(), base, headers, title, filename, part)
		if err != nil {
			return err
		}
		if err = d.recordTask(uploadTask{ID: task, User: id.UserID, FileName: filename, Created: time.Now().UTC()}); err != nil {
			return err
		}
		writeJSON(w, 202, map[string]string{"task": task, "state": "processing"})
		return nil
	}
}

func (d *Documents) forward(ctx context.Context, base string, headers map[string]string, title, filename string, file io.Reader) (string, error) {
	// Django needs a known Content-Length for multipart bodies, so the upload is
	// spooled to a private temporary file instead of being streamed chunked.
	dir := filepath.Join(d.hub.opts.State, "tmp")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	spool, err := os.CreateTemp(dir, "upload-")
	if err != nil {
		return "", err
	}
	defer os.Remove(spool.Name())
	defer spool.Close()
	mw := multipart.NewWriter(spool)
	if title != "" {
		err = mw.WriteField("title", title)
	}
	if err == nil {
		var part io.Writer
		if part, err = mw.CreateFormFile("document", filename); err == nil {
			_, err = io.Copy(part, file)
		}
	}
	if err == nil {
		err = mw.Close()
	}
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			return "", apiErr(413, "too_large", "Die Datei ist zu groß.")
		}
		return "", apiErr(400, "upload_interrupted", "Der Upload wurde unterbrochen. Bitte erneut versuchen.")
	}
	size, err := spool.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", err
	}
	if _, err = spool.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/api/documents/post_document/", spool)
	if err != nil {
		return "", err
	}
	req.ContentLength = size
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := d.stream.Do(req)
	if err != nil {
		return "", apiErr(502, "unavailable", "Das Dokumentenarchiv ist gerade nicht erreichbar.")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		if res.StatusCode == 400 {
			return "", apiErr(415, "unsupported", "Dieses Dateiformat kann das Dokumentenarchiv nicht übernehmen.")
		}
		return "", serviceError(res.StatusCode)
	}
	var task string
	if err = decodeLimited(res.Body, &task); err != nil || !taskPattern.MatchString(task) {
		return "", apiErr(502, "unavailable", "Unerwartete Antwort des Dokumentenarchivs.")
	}
	return task, nil
}

func (d *Documents) tasks(w http.ResponseWriter, r *http.Request, id Identity) error {
	base, headers, err := d.session(id)
	if err != nil {
		return err
	}
	d.mu.Lock()
	all := d.loadTasks()
	d.mu.Unlock()
	out := []map[string]any{}
	for i := len(all) - 1; i >= 0 && len(out) < 20; i-- {
		t := all[i]
		if t.User != id.UserID {
			continue
		}
		entry := map[string]any{"id": t.ID, "fileName": t.FileName, "created": t.Created, "state": "unknown"}
		var upstream []struct {
			Status          string  `json:"status"`
			Result          *string `json:"result"`
			RelatedDocument *string `json:"related_document"`
		}
		if serviceCall(r.Context(), d.client, "GET", base+"/api/tasks/?task_id="+t.ID, headers, nil, &upstream) == nil && len(upstream) == 1 {
			switch upstream[0].Status {
			case "PENDING", "STARTED", "RETRY":
				entry["state"] = "processing"
			case "SUCCESS":
				entry["state"] = "done"
				if upstream[0].RelatedDocument != nil {
					if n, err := strconv.Atoi(*upstream[0].RelatedDocument); err == nil {
						entry["document"] = n
					}
				}
			case "FAILURE":
				entry["state"] = "failed"
				entry["message"] = "Der Import ist fehlgeschlagen."
				if upstream[0].Result != nil && strings.Contains(strings.ToLower(*upstream[0].Result), "duplicate") {
					entry["state"], entry["message"] = "duplicate", "Dieses Dokument ist bereits im Archiv."
				}
			}
		}
		out = append(out, entry)
	}
	return writeOK(w, map[string]any{"items": out})
}

func decodeLimited(r io.Reader, out any) error {
	if err := json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(out); err != nil {
		return apiErr(502, "unavailable", "Unerwartete Antwort des Dienstes.")
	}
	return nil
}
