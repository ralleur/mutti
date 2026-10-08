// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Real Immich 2.7.5 and Paperless-ngx 2.20.15 from mutti/tests/module-testenv.py.
// Opt-in: MUTTI_MODULE_TESTENV=/path/to/build/module-testenv/private.json
type moduleTestenv struct {
	PhotosURL    string `json:"photos_url"`
	DocumentsURL string `json:"documents_url"`
	Accounts     struct {
		Photos    map[string]struct{ Email, Password string } `json:"photos"`
		Documents map[string]struct {
			Username, Password string
			ID                 int
		} `json:"documents"`
	} `json:"accounts"`
	PhotosFixture    map[string]string `json:"photos_fixture"`
	DocumentsFixture map[string]int    `json:"documents_fixture"`
}

func loadTestenv(t *testing.T) moduleTestenv {
	path := os.Getenv("MUTTI_MODULE_TESTENV")
	if path == "" {
		t.Skip("set MUTTI_MODULE_TESTENV to the isolated module test environment")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var env moduleTestenv
	if err = json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	return env
}

func (e *testEnv) raw(method, path, token string, body io.Reader, contentType string, headers map[string]string) (*http.Response, []byte) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.server.URL+"/mutti/hub/v1"+path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

func (e *testEnv) mustAdmin(path string, body any) {
	e.t.Helper()
	var out map[string]any
	if code := e.do("POST", path, "token-owner", body, &out); code != 200 {
		e.t.Fatalf("%s: %d %v", path, code, out)
	}
}

func TestRealPhotosModule(t *testing.T) {
	env := loadTestenv(t)
	e := newTestEnv(t, "")
	var out map[string]any
	if code := e.do("POST", "/admin/link/photos", "token-owner", map[string]string{"userId": userA, "account": "x", "password": "y"}, &out); code != 409 {
		t.Fatalf("link before service %d", code)
	}
	e.mustAdmin("/admin/service/photos", map[string]string{"url": env.PhotosURL})
	// Wrong password and the instance administrator are both rejected.
	if code := e.do("POST", "/admin/link/photos", "token-owner", map[string]string{"userId": userA, "account": env.Accounts.Photos["alpha"].Email, "password": "wrong"}, &out); code != 400 {
		t.Fatalf("wrong password %d", code)
	}
	e.mustAdmin("/admin/link/photos", map[string]string{"userId": userA, "account": env.Accounts.Photos["alpha"].Email, "password": env.Accounts.Photos["alpha"].Password})
	if code := e.do("POST", "/admin/link/photos", "token-owner", map[string]string{"userId": userB, "account": env.Accounts.Photos["alpha"].Email, "password": env.Accounts.Photos["alpha"].Password}, &out); code != 409 {
		t.Fatalf("same account for two profiles %d", code)
	}
	e.mustAdmin("/admin/link/photos", map[string]string{"userId": userB, "account": env.Accounts.Photos["beta"].Email, "password": env.Accounts.Photos["beta"].Password})
	var state map[string]any
	e.do("GET", "/admin/state", "token-owner", nil, &state)
	if b, _ := json.Marshal(state); bytes.Contains(b, []byte(env.Accounts.Photos["alpha"].Password)) || bytes.Contains(b, []byte(`"secret"`)) {
		t.Fatal("admin state exposes credentials")
	}
	if code := e.do("GET", "/photos/assets", "token-a", nil, nil); code != 403 {
		t.Fatalf("photos before grant %d", code)
	}
	e.mustAdmin("/admin/enable/photos", map[string]bool{"enabled": true})
	e.mustAdmin("/admin/grants", map[string]any{"module": "photos", "userId": userA, "allowed": true})
	e.mustAdmin("/admin/grants", map[string]any{"module": "photos", "userId": userB, "allowed": true})
	e.hub.refreshHealth(context.Background())

	var page struct {
		Items []PhotoAsset `json:"items"`
	}
	if code := e.do("GET", "/photos/assets?size=50", "token-a", nil, &page); code != 200 {
		t.Fatalf("list %d", code)
	}
	ids := map[string]PhotoAsset{}
	for _, a := range page.Items {
		ids[a.ID] = a
	}
	for _, key := range []string{"see", "garten", "herbst", "clip"} {
		if _, ok := ids[env.PhotosFixture[key]]; !ok {
			t.Fatalf("own asset %s missing", key)
		}
	}
	if _, ok := ids[env.PhotosFixture["private_b"]]; ok {
		t.Fatal("foreign asset listed")
	}
	if ids[env.PhotosFixture["clip"]].Type != "video" || !strings.Contains(ids[env.PhotosFixture["herbst"]].Mime, "heic") {
		t.Fatalf("types %v / %v", ids[env.PhotosFixture["clip"]], ids[env.PhotosFixture["herbst"]])
	}
	var search struct {
		Items []PhotoAsset `json:"items"`
		Mode  string       `json:"mode"`
	}
	e.do("GET", "/photos/search?q=See", "token-a", nil, &search)
	if len(search.Items) == 0 || search.Items[0].ID != env.PhotosFixture["see"] {
		t.Fatalf("search %v", search)
	}
	e.do("GET", "/photos/search?q=Privat", "token-a", nil, &search)
	if len(search.Items) != 0 {
		t.Fatal("search revealed another profile's photo")
	}
	e.do("GET", "/photos/search?from=2026-08-01&to=2026-08-31", "token-a", nil, &search)
	if len(search.Items) != 1 || search.Items[0].ID != env.PhotosFixture["clip"] {
		t.Fatalf("date search %v", search.Items)
	}
	var albums struct {
		Items []map[string]any `json:"items"`
	}
	e.do("GET", "/photos/albums", "token-a", nil, &albums)
	if len(albums.Items) != 1 || albums.Items[0]["name"] != "Urlaub 2026" {
		t.Fatalf("albums %v", albums)
	}
	var album struct {
		Items []PhotoAsset `json:"items"`
	}
	e.do("GET", "/photos/albums/"+env.PhotosFixture["album"], "token-a", nil, &album)
	if len(album.Items) != 2 {
		t.Fatalf("album assets %d", len(album.Items))
	}
	if code := e.do("GET", "/photos/albums/"+env.PhotosFixture["album"], "token-b", nil, nil); code != 404 {
		t.Fatalf("foreign album %d", code)
	}
	for _, kind := range []string{"thumbnail", "preview", "original"} {
		res, body := e.raw("GET", "/photos/assets/"+env.PhotosFixture["see"]+"/"+kind, "token-a", nil, "", nil)
		if res.StatusCode != 200 || len(body) == 0 || res.Header.Get("Set-Cookie") != "" {
			t.Fatalf("%s %d", kind, res.StatusCode)
		}
		if res2, _ := e.raw("GET", "/photos/assets/"+env.PhotosFixture["see"]+"/"+kind, "token-b", nil, "", nil); res2.StatusCode == 200 {
			t.Fatalf("foreign %s readable", kind)
		}
	}
	res, body := e.raw("GET", "/photos/assets/"+env.PhotosFixture["clip"]+"/video", "token-a", nil, "", map[string]string{"Range": "bytes=0-99"})
	if res.StatusCode != 206 || len(body) != 100 {
		t.Fatalf("video range %d %d", res.StatusCode, len(body))
	}
	// Upload, duplicate detection and the profile's device id.
	upload := func(token, name string, content []byte) (int, map[string]string) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("deviceAssetId", "test-"+name)
		_ = mw.WriteField("fileCreatedAt", "2026-10-06T10:00:00Z")
		_ = mw.WriteField("fileModifiedAt", "2026-10-06T10:00:00Z")
		part, _ := mw.CreateFormFile("assetData", name)
		_, _ = part.Write(content)
		_ = mw.Close()
		res, body := e.raw("POST", "/photos/assets", token, &buf, mw.FormDataContentType(), nil)
		var out map[string]string
		_ = json.Unmarshal(body, &out)
		return res.StatusCode, out
	}
	content := testPNG(byte(time.Now().UnixNano()))
	code, first := upload("token-a", fmt.Sprintf("upload-%d.png", time.Now().UnixNano()), content)
	if code != 200 || first["status"] != "created" {
		t.Fatalf("upload %d %v", code, first)
	}
	code, second := upload("token-a", "again.png", content)
	if code != 200 || second["status"] != "duplicate" || second["id"] != first["id"] {
		t.Fatalf("duplicate %d %v", code, second)
	}
	res, body = e.raw("GET", "/photos/assets/"+first["id"]+"/original", "token-a", nil, "", nil)
	if res.StatusCode != 200 || sha256.Sum256(body) != sha256.Sum256(content) {
		t.Fatal("uploaded original not byte-identical")
	}
	// Revocation of the grant applies immediately.
	e.mustAdmin("/admin/grants", map[string]any{"module": "photos", "userId": userA, "allowed": false})
	if res, _ := e.raw("GET", "/photos/assets/"+env.PhotosFixture["see"]+"/original", "token-a", nil, "", nil); res.StatusCode != 403 {
		t.Fatalf("after revoke %d", res.StatusCode)
	}
	var caps map[string]any
	e.do("GET", "/capabilities", "token-a", nil, &caps)
	if s := caps["modules"].(map[string]any)["photos"].(map[string]any)["state"]; s != "not_allowed" {
		t.Fatalf("capability after revoke %v", s)
	}
}

func TestRealDocumentsModule(t *testing.T) {
	env := loadTestenv(t)
	e := newTestEnv(t, "")
	e.mustAdmin("/admin/service/documents", map[string]string{"url": env.DocumentsURL})
	var out map[string]any
	if code := e.do("POST", "/admin/link/documents", "token-owner", map[string]string{"userId": userA, "account": "testowner", "password": "nope"}, &out); code != 400 {
		t.Fatalf("bad login %d", code)
	}
	for user, name := range map[string]string{userA: "alpha", userB: "beta"} {
		e.mustAdmin("/admin/link/documents", map[string]string{"userId": user, "account": env.Accounts.Documents[name].Username, "password": env.Accounts.Documents[name].Password})
		e.mustAdmin("/admin/grants", map[string]any{"module": "documents", "userId": user, "allowed": true})
	}
	e.mustAdmin("/admin/enable/documents", map[string]bool{"enabled": true})
	e.hub.refreshHealth(context.Background())
	var page documentPage
	if code := e.do("GET", "/documents?q=Rechnung", "token-a", nil, &page); code != 200 || page.Count == 0 {
		t.Fatalf("search %d %v", code, page)
	}
	if page.Items[0].ID != env.DocumentsFixture["invoice"] || !strings.Contains(page.Items[0].Snippet, "128,40") || strings.Contains(page.Items[0].Snippet, "<span") {
		t.Fatalf("hit %v", page.Items[0])
	}
	e.do("GET", "/documents?q=Arztrechnung", "token-a", nil, &page)
	if page.Count != 0 {
		t.Fatal("foreign document found")
	}
	e.do("GET", "/documents?q=312", "token-a", nil, &page)
	if page.Count == 0 || page.Items[0].ID != env.DocumentsFixture["scan_ocr"] {
		t.Fatalf("OCR scan not searchable %v", page)
	}
	var doc Document
	if code := e.do("GET", fmt.Sprintf("/documents/%d", env.DocumentsFixture["contract"]), "token-a", nil, &doc); code != 200 || doc.Pages == nil || *doc.Pages != 3 || !strings.Contains(*doc.Content, "840,00") {
		t.Fatalf("contract %d %v", code, doc)
	}
	if code := e.do("GET", fmt.Sprintf("/documents/%d", env.DocumentsFixture["private_b"]), "token-a", nil, nil); code != 404 {
		t.Fatalf("foreign document %d", code)
	}
	for _, kind := range []string{"thumbnail", "preview", "original"} {
		res, body := e.raw("GET", fmt.Sprintf("/documents/%d/%s", env.DocumentsFixture["invoice"], kind), "token-a", nil, "", nil)
		if res.StatusCode != 200 || len(body) == 0 {
			t.Fatalf("%s %d", kind, res.StatusCode)
		}
		if res, _ := e.raw("GET", fmt.Sprintf("/documents/%d/%s", env.DocumentsFixture["invoice"], kind), "token-b", nil, "", nil); res.StatusCode == 200 {
			t.Fatalf("foreign %s", kind)
		}
	}
	// Upload is tracked per profile until processing finishes.
	marker := fmt.Sprintf("Testupload %d", time.Now().UnixNano())
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("title", marker)
	part, _ := mw.CreateFormFile("document", "notiz.txt")
	_, _ = part.Write([]byte("Notiz " + marker + " Betrag 42,00 EUR"))
	_ = mw.Close()
	res, body := e.raw("POST", "/documents", "token-a", &buf, mw.FormDataContentType(), nil)
	if res.StatusCode != 202 {
		t.Fatalf("upload %d %s", res.StatusCode, body)
	}
	var task map[string]string
	_ = json.Unmarshal(body, &task)
	var tasks struct {
		Items []map[string]any `json:"items"`
	}
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		e.do("GET", "/documents/tasks", "token-a", nil, &tasks)
		if len(tasks.Items) > 0 && tasks.Items[0]["id"] == task["task"] && tasks.Items[0]["state"] != "processing" && tasks.Items[0]["state"] != "unknown" {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if tasks.Items[0]["state"] != "done" || tasks.Items[0]["document"] == nil {
		t.Fatalf("task %v", tasks.Items[0])
	}
	e.do("GET", "/documents/tasks", "token-b", nil, &tasks)
	for _, it := range tasks.Items {
		if it["id"] == task["task"] {
			t.Fatal("foreign profile sees upload task")
		}
	}
	// The AI tool uses the same profile-bound search and never sees B's data.
	// Since P0 a tool runs only for a qualified deployed model (synthetic here).
	e.configureAI()
	tools := &profileTools{hub: e.hub, id: Identity{UserID: userA}, sources: &sourceBook{Items: map[string]*Source{}}, defs: documentTools(),
		docs: hubDocuments{e.hub.docs, Identity{UserID: userA}}}
	result := tools.Call(context.Background(), "search_documents", json.RawMessage(`{"query":"Rechnung"}`))
	if !strings.Contains(result.Content, "RE-2026-0815") || strings.Contains(result.Content, "Arzt") || !strings.Contains(result.Content, `"source":"Q1"`) {
		t.Fatalf("tool %s", result.Content)
	}
	read := tools.Call(context.Background(), "read_document", json.RawMessage(`{"source":"Q1"}`))
	if !strings.Contains(read.Content, "15.11.2026") {
		t.Fatalf("read %s", read.Content)
	}
	if fake := tools.Call(context.Background(), "read_document", json.RawMessage(`{"source":"RE-2026-0815"}`)); !strings.Contains(fake.Content, `"error"`) {
		t.Fatal("invented source accepted")
	}
}

func testPNG(seed byte) []byte {
	// Minimal valid 1x1 PNG; the same call returns the same bytes only once.
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	chunk := func(kind string, data []byte) {
		var b bytes.Buffer
		_ = writeU32(&b, uint32(len(data)))
		b.WriteString(kind)
		b.Write(data)
		_ = writeU32(&b, crc32IEEE(append([]byte(kind), data...)))
		png = append(png, b.Bytes()...)
	}
	chunk("IHDR", []byte{0, 0, 0, 1, 0, 0, 0, 1, 8, 2, 0, 0, 0})
	// The persistent test library keeps every upload; a timestamp text chunk
	// keeps the bytes unique beyond the 256 pixel variants.
	chunk("tEXt", []byte(fmt.Sprintf("Comment\x00mutti-test-%d", time.Now().UnixNano())))
	chunk("IDAT", zlibBytes([]byte{0, seed, 0x80, 0x40}))
	chunk("IEND", nil)
	return png
}
