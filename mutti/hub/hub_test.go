// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	userA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	userB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	owner = "cccccccccccccccccccccccccccccccc"
)

type fakeJellyfin struct {
	mu        sync.Mutex
	tokens    map[string]string
	disabled  map[string]bool
	favorites map[string]bool
	movies    map[string][]jellyItem
}

func newFakeJellyfin() *fakeJellyfin {
	item := func(id, name string, seconds int, played bool) jellyItem {
		it := jellyItem{ID: id, Name: name, ProductionYear: 2024, RunTimeTicks: int64(seconds) * 10_000_000, Genres: []string{"Drama"}, Overview: "Synthetischer Testfilm."}
		it.UserData.Played = played
		return it
	}
	common := []jellyItem{item("11111111111111111111111111111111", "Nordlicht", 84, false), item("22222222222222222222222222222222", "Sommer am See", 95, false),
		item("33333333333333333333333333333333", "Lange Reise", 125, false), item("44444444444444444444444444444444", "Schon gesehen", 88, true)}
	return &fakeJellyfin{
		tokens:    map[string]string{"token-a": userA, "token-b": userB, "token-owner": owner},
		disabled:  map[string]bool{},
		favorites: map[string]bool{},
		movies:    map[string][]jellyItem{userA: common, userB: append(append([]jellyItem{}, common...), item("55555555555555555555555555555555", "Privater Film B", 92, false))},
	}
}

func (f *fakeJellyfin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	token := requestToken(r)
	user, ok := f.tokens[token]
	if !ok || f.disabled[user] {
		w.WriteHeader(401)
		return
	}
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	path := r.URL.Path
	switch {
	case path == "/Users/Me":
		reply(map[string]any{"Id": user, "Name": map[string]string{userA: "Alpha", userB: "Beta", owner: "Owner"}[user],
			"Policy": map[string]any{"IsAdministrator": user == owner, "IsDisabled": false}})
	case path == "/Users":
		if user != owner {
			w.WriteHeader(403)
			return
		}
		reply([]map[string]any{{"Id": userA, "Name": "Alpha", "Policy": map[string]bool{}}, {"Id": userB, "Name": "Beta", "Policy": map[string]bool{}},
			{"Id": owner, "Name": "Owner", "Policy": map[string]bool{"IsAdministrator": true}}})
	case strings.HasPrefix(path, "/Users/"+user+"/FavoriteItems/"):
		id := strings.TrimPrefix(path, "/Users/"+user+"/FavoriteItems/")
		f.favorites[user+id] = r.Method == "POST"
		w.WriteHeader(200)
		reply(map[string]any{})
	case path == "/Users/"+user+"/Items":
		reply(map[string]any{"Items": f.movies[user]})
	case strings.HasPrefix(path, "/Users/"+user+"/Items/"):
		id := strings.TrimPrefix(path, "/Users/"+user+"/Items/")
		for _, m := range f.movies[user] {
			if m.ID == id {
				m.UserData.IsFavorite = f.favorites[user+id]
				reply(m)
				return
			}
		}
		w.WriteHeader(404)
	default:
		w.WriteHeader(404)
	}
}

// fakeEngine scripts the tool loop of a model so harness, rights and source
// handling are tested deterministically. Real models run in the casting.
type fakeEngine struct {
	delay  time.Duration
	chats  atomic.Int32
	aborts atomic.Int32
}

func (e *fakeEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/version":
		_ = json.NewEncoder(w).Encode(map[string]string{"version": "0.0-test"})
	case "/api/tags":
		_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "qwen3.5:4b", "digest": catalog[0].Digest, "size": catalog[0].Bytes}}})
	case "/api/show":
		_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{"completion", "tools", "thinking"}})
	case "/api/chat":
		e.chats.Add(1)
		var req struct {
			Messages []chatMessage `json:"messages"`
			Tools    []toolDef     `json:"tools"`
			Think    *bool         `json:"think"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		last := req.Messages[len(req.Messages)-1]
		flusher := w.(http.Flusher)
		send := func(v any) { _ = json.NewEncoder(w).Encode(v); flusher.Flush() }
		call := func(name, args string) {
			send(map[string]any{"message": map[string]any{"role": "assistant", "content": "", "tool_calls": []map[string]any{{"function": map[string]any{"name": name, "arguments": json.RawMessage(args)}}}}})
			send(map[string]any{"done": true, "done_reason": "stop"})
		}
		words := func(text string) {
			for _, word := range strings.SplitAfter(text, " ") {
				select {
				case <-r.Context().Done():
					e.aborts.Add(1)
					return
				case <-time.After(e.delay):
				}
				send(map[string]any{"message": map[string]any{"role": "assistant", "content": word}})
			}
			send(map[string]any{"done": true, "done_reason": "stop", "eval_count": 10, "eval_duration": 1000})
		}
		switch {
		case last.Role == "user" && strings.Contains(last.Content, "ungesehenen Filme"):
			call("search_movies", `{"runtime_below_seconds":100,"unwatched":true,"sort":"runtime"}`)
		case last.Role == "user" && strings.Contains(last.Content, "Favorit"):
			call("propose_favorite", `{"quelle":"Q1","favorite":true}`)
		case last.Role == "tool" && last.ToolName == "search_movies":
			words("Gefunden: Nordlicht [Q1] und Sommer am See [Q2]. Erfunden [Q9].")
		case last.Role == "tool":
			words("Ich habe den Vorschlag angelegt; bitte bestätige ihn.")
		case last.Role == "user" && strings.Contains(last.Content, "lang"):
			words(strings.Repeat("lang ", 200))
		default:
			words("Hallo, ich bin lokal.")
		}
	default:
		w.WriteHeader(404)
	}
}

type testEnv struct {
	t      *testing.T
	hub    *Hub
	server *httptest.Server
	jf     *fakeJellyfin
	engine *fakeEngine
	state  string
}

func newTestEnv(t *testing.T, state string) *testEnv {
	t.Helper()
	jf := newFakeJellyfin()
	jfServer := httptest.NewServer(jf)
	t.Cleanup(jfServer.Close)
	if state == "" {
		state = t.TempDir()
	}
	h, err := New(Options{State: state, Jellyfin: jfServer.URL, PeerSecret: "peer-secret"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Handler())
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	t.Cleanup(func() { cancel(); srv.Close() })
	return &testEnv{t: t, hub: h, server: srv, jf: jf, engine: &fakeEngine{}, state: state}
}

func (e *testEnv) do(method, path, token string, body any, out any) int {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.server.URL+"/mutti/hub/v1"+path, reader)
	req.Header.Set("Authorization", `MediaBrowser Token="`+token+`"`)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func (e *testEnv) configureAI() {
	e.t.Helper()
	engine := httptest.NewServer(e.engine)
	e.t.Cleanup(engine.Close)
	if code := e.do("POST", "/admin/ai/engine", "token-owner", map[string]string{"mode": "external", "url": engine.URL}, nil); code != 200 {
		e.t.Fatalf("engine config %d", code)
	}
	if code := e.do("POST", "/admin/ai/models/select", "token-owner", map[string]string{"model": "qwen3.5:4b"}, nil); code != 200 {
		e.t.Fatalf("select %d", code)
	}
	for _, step := range []struct {
		path string
		body any
	}{{"/admin/enable/ai", map[string]bool{"enabled": true}}, {"/admin/grants", map[string]any{"module": "ai", "userId": userA, "allowed": true}},
		{"/admin/grants", map[string]any{"module": "ai", "userId": userB, "allowed": true}}} {
		if code := e.do("POST", step.path, "token-owner", step.body, nil); code != 200 {
			e.t.Fatalf("%s %d", step.path, code)
		}
	}
}

type sseEvent struct {
	ID   string
	Type string
	Data map[string]any
}

func (e *testEnv) events(run, conversation, token string, stop func(sseEvent) bool) ([]sseEvent, int) {
	e.t.Helper()
	req, _ := http.NewRequest("GET", e.server.URL+"/mutti/hub/v1/ai/runs/"+run+"/events?conversation="+conversation, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, res.StatusCode
	}
	var out []sseEvent
	scanner := bufio.NewScanner(res.Body)
	var cur sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "id: "):
			cur.ID = line[4:]
		case strings.HasPrefix(line, "event: "):
			cur.Type = line[7:]
		case strings.HasPrefix(line, "data: "):
			_ = json.Unmarshal([]byte(line[6:]), &cur.Data)
		case line == "" && cur.Type != "":
			out = append(out, cur)
			if stop != nil && stop(cur) {
				return out, 200
			}
			cur = sseEvent{}
		}
	}
	return out, 200
}

func (e *testEnv) ask(token, conversation, text, key string) (map[string]any, int) {
	var out map[string]any
	code := e.do("POST", "/ai/conversations/"+conversation+"/messages", token, map[string]string{"text": text, "idempotencyKey": key}, &out)
	return out, code
}

func runID(v map[string]any) string { return v["run"].(map[string]any)["id"].(string) }

func TestAdminBoundariesAndCapabilities(t *testing.T) {
	e := newTestEnv(t, "")
	var caps map[string]any
	if code := e.do("GET", "/capabilities", "token-a", nil, &caps); code != 200 {
		t.Fatal(code)
	}
	modules := caps["modules"].(map[string]any)
	if modules["ai"].(map[string]any)["state"] != "not_configured" || modules["media"].(map[string]any)["state"] != "ready" {
		t.Fatalf("unexpected capabilities %v", modules)
	}
	if code := e.do("GET", "/admin/state", "token-a", nil, nil); code != 403 {
		t.Fatalf("profile reached owner route: %d", code)
	}
	if code := e.do("GET", "/capabilities", "stolen", nil, nil); code != 401 {
		t.Fatalf("unknown token %d", code)
	}
	if code := e.do("POST", "/admin/service/photos", "token-owner", map[string]string{"url": "https://photos.example.com"}, nil); code != 400 {
		t.Fatalf("public service URL accepted: %d", code)
	}
	if code := e.do("POST", "/admin/service/photos", "token-owner", map[string]string{"url": "http://8.8.8.8:2283"}, nil); code != 400 {
		t.Fatalf("public IP accepted: %d", code)
	}
	if code := e.do("POST", "/admin/grants", "token-owner", map[string]any{"module": "ai", "userId": owner, "allowed": true}, nil); code != 400 {
		t.Fatalf("administrator granted as playback profile: %d", code)
	}
	req, _ := http.NewRequest("GET", e.server.URL+"/mutti/hub/v1/capabilities", nil)
	req.Header.Set("Authorization", "Bearer token-a")
	req.Header.Set("Origin", "http://evil.example")
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != 403 {
		t.Fatalf("browser origin accepted: %d", res.StatusCode)
	}
}

func TestChatToolsSourcesAndIsolation(t *testing.T) {
	e := newTestEnv(t, "")
	e.configureAI()
	var conv map[string]any
	if code := e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv); code != 201 {
		t.Fatal(code)
	}
	cid := conv["id"].(string)
	started, code := e.ask("token-a", cid, "Suche alle ungesehenen Filme unter 100 Sekunden und sortiere nach Laufzeit.", "k1")
	if code != 202 {
		t.Fatal(code)
	}
	again, code := e.ask("token-a", cid, "Suche alle ungesehenen Filme unter 100 Sekunden und sortiere nach Laufzeit.", "k1")
	if code != 200 || runID(again) != runID(started) {
		t.Fatalf("idempotent resend created another run: %d", code)
	}
	if _, code := e.events(runID(started), cid, "token-b", nil); code != 404 {
		t.Fatalf("foreign profile reached run events: %d", code)
	}
	events, _ := e.events(runID(started), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "done" })
	done := events[len(events)-1]
	if done.Data["state"] != "completed" {
		t.Fatalf("run state %v", done.Data)
	}
	text := done.Data["message"].(map[string]any)["text"].(string)
	if strings.Contains(text, "[Q9]") || !strings.Contains(text, "[Q1]") || !strings.Contains(text, "[Q2]") {
		t.Fatalf("citations not cleaned: %q", text)
	}
	titles := []string{}
	for _, s := range done.Data["sources"].([]any) {
		titles = append(titles, s.(map[string]any)["title"].(string))
	}
	if strings.Join(titles, ",") != "Nordlicht,Sommer am See" {
		t.Fatalf("sources %v", titles)
	}
	tools := 0
	for _, ev := range events {
		if ev.Type == "tool" {
			tools++
		}
	}
	if tools != 2 {
		t.Fatalf("tool events %d", tools)
	}
	if code := e.do("GET", "/ai/conversations/"+cid, "token-b", nil, nil); code != 404 {
		t.Fatalf("foreign conversation visible: %d", code)
	}
	if code := e.do("GET", "/ai/sources/"+cid+"/Q1", "token-b", nil, nil); code != 404 {
		t.Fatalf("foreign source resolvable: %d", code)
	}
	var source map[string]any
	if code := e.do("GET", "/ai/sources/"+cid+"/Q1", "token-a", nil, &source); code != 200 || source["title"] != "Nordlicht" {
		t.Fatalf("own source %d %v", code, source)
	}
	var list map[string]any
	e.do("GET", "/ai/conversations", "token-b", nil, &list)
	if len(list["items"].([]any)) != 0 {
		t.Fatal("conversation list leaked")
	}
	// The replayed stream after completion is a snapshot, not a new run.
	snapshot, _ := e.events(runID(started), cid, "token-a", nil)
	if len(snapshot) != 1 || snapshot[0].Data["snapshot"] != true {
		t.Fatalf("snapshot %v", snapshot)
	}
	if e.engine.chats.Load() != 2 {
		t.Fatalf("engine calls %d", e.engine.chats.Load())
	}
}

func TestProposalConfirmationIsServerSide(t *testing.T) {
	e := newTestEnv(t, "")
	e.configureAI()
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	first, _ := e.ask("token-a", cid, "Suche alle ungesehenen Filme unter 100 Sekunden.", "s1")
	e.events(runID(first), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "done" })
	second, _ := e.ask("token-a", cid, "Markiere Nordlicht als Favorit.", "s2")
	events, _ := e.events(runID(second), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "done" })
	props := events[len(events)-1].Data["proposals"].([]any)
	if len(props) != 1 {
		t.Fatalf("proposals %v", events[len(events)-1].Data)
	}
	pid := props[0].(map[string]any)["id"].(string)
	if e.jf.favorites[userA+"11111111111111111111111111111111"] {
		t.Fatal("favorite changed before confirmation")
	}
	if code := e.do("POST", "/ai/proposals/"+pid+"/confirm", "token-b", map[string]string{"conversation": cid}, nil); code != 404 {
		t.Fatalf("foreign confirmation %d", code)
	}
	var p map[string]any
	if code := e.do("POST", "/ai/proposals/"+pid+"/confirm", "token-a", map[string]string{"conversation": cid}, &p); code != 200 || p["state"] != "confirmed" {
		t.Fatalf("confirm %d %v", code, p)
	}
	if !e.jf.favorites[userA+"11111111111111111111111111111111"] {
		t.Fatal("favorite not applied")
	}
	e.jf.favorites[userA+"11111111111111111111111111111111"] = false
	e.do("POST", "/ai/proposals/"+pid+"/confirm", "token-a", map[string]string{"conversation": cid}, &p)
	if p["state"] != "confirmed" || e.jf.favorites[userA+"11111111111111111111111111111111"] {
		t.Fatal("repeated confirmation executed again")
	}
	if code := e.do("POST", "/ai/proposals/"+pid+"/reject", "token-a", map[string]string{"conversation": cid}, &p); code != 200 || p["state"] != "confirmed" {
		t.Fatal("decided proposal changed state")
	}
}

func TestCancelRetryRevokeAndRestart(t *testing.T) {
	state := t.TempDir()
	e := newTestEnv(t, state)
	e.engine.delay = 30 * time.Millisecond
	e.configureAI()
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	run, _ := e.ask("token-a", cid, "Erzähl etwas lang", "c1")
	if _, code := e.ask("token-a", cid, "Noch eine Frage", "c2"); code != 409 {
		t.Fatalf("parallel run accepted: %d", code)
	}
	var cancelled time.Time
	events, _ := e.events(runID(run), cid, "token-a", func(ev sseEvent) bool {
		if ev.Type == "delta" && cancelled.IsZero() {
			cancelled = time.Now()
			if code := e.do("POST", "/ai/runs/"+runID(run)+"/cancel", "token-a", nil, nil); code != 202 {
				t.Errorf("cancel %d", code)
			}
		}
		return ev.Type == "done"
	})
	done := events[len(events)-1]
	if done.Data["state"] != "cancelled" || time.Since(cancelled) > time.Second {
		t.Fatalf("cancel took %v state %v", time.Since(cancelled), done.Data["state"])
	}
	time.Sleep(100 * time.Millisecond)
	if e.engine.aborts.Load() == 0 {
		t.Fatal("engine generation not aborted")
	}
	var retry map[string]any
	if code := e.do("POST", "/ai/runs/"+runID(run)+"/retry", "token-a", map[string]string{"conversation": cid, "idempotencyKey": "r1"}, &retry); code != 202 {
		t.Fatalf("retry %d", code)
	}
	var again map[string]any
	if code := e.do("POST", "/ai/runs/"+runID(run)+"/retry", "token-a", map[string]string{"conversation": cid, "idempotencyKey": "r1"}, &again); code != 200 || runID(again) != runID(retry) {
		t.Fatalf("repeated retry created another run: %d", code)
	}
	// Revoking the grant ends the live stream and further access.
	revoked := make(chan []sseEvent, 1)
	go func() {
		evs, _ := e.events(runID(retry), cid, "token-a", nil)
		revoked <- evs
	}()
	time.Sleep(200 * time.Millisecond)
	e.do("POST", "/admin/grants", "token-owner", map[string]any{"module": "ai", "userId": userA, "allowed": false}, nil)
	select {
	case <-revoked:
	case <-time.After(3 * time.Second):
		t.Fatal("stream continued after revocation")
	}
	if code := e.do("GET", "/ai/conversations/"+cid, "token-a", nil, nil); code != 403 {
		t.Fatalf("access after revocation %d", code)
	}
	var c map[string]any
	e.do("POST", "/admin/grants", "token-owner", map[string]any{"module": "ai", "userId": userA, "allowed": true}, nil)
	e.do("GET", "/ai/conversations/"+cid, "token-a", nil, &c)
	states := []string{}
	for _, r := range c["runs"].([]any) {
		states = append(states, r.(map[string]any)["state"].(string))
	}
	if fmt.Sprint(states) != "[cancelled cancelled]" {
		t.Fatalf("revocation did not stop the generation: %v", states)
	}
}

func TestRestartMarksRunInterrupted(t *testing.T) {
	state := t.TempDir()
	e := newTestEnv(t, state)
	e.engine.delay = 50 * time.Millisecond
	e.configureAI()
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	run, _ := e.ask("token-a", cid, "Erzähl etwas lang", "x1")
	e.events(runID(run), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "delta" })
	// A second process start on the same data (after a crash) must not
	// resume or report the unfinished answer as successful.
	e2 := newTestEnv(t, state)
	var c map[string]any
	e2.do("GET", "/ai/conversations/"+cid, "token-a", nil, &c)
	r := c["runs"].([]any)[0].(map[string]any)
	m := c["messages"].([]any)[1].(map[string]any)
	if r["state"] != "interrupted" || m["status"] != "interrupted" || m["error"] == "" {
		t.Fatalf("after restart: run %v message %v", r["state"], m["status"])
	}
	var retry map[string]any
	if code := e2.do("POST", "/ai/runs/"+runID(run)+"/retry", "token-a", map[string]string{"conversation": cid}, &retry); code != 202 {
		t.Fatalf("explicit retry after restart %d", code)
	}
}

func TestCitationCleanup(t *testing.T) {
	text, cited, invalid := CleanCitations("A [Q1] B [Q7] C [Q1] [Q2].", func(r string) bool { return r == "Q1" || r == "Q2" })
	if text != "A [Q1] B  C [Q1] [Q2]." || fmt.Sprint(cited) != "[Q1 Q2]" || invalid != 1 {
		t.Fatalf("%q %v %d", text, cited, invalid)
	}
	text, cited, _ = CleanCitations("Betrag (Q2), Nummer (Q8).", func(r string) bool { return r == "Q2" })
	if text != "Betrag [Q2], Nummer (Q8)." || fmt.Sprint(cited) != "[Q2]" {
		t.Fatalf("parenthesised %q %v", text, cited)
	}
}

func TestFilterMoviesUsesExclusiveRuntime(t *testing.T) {
	yes := true
	movies := []Movie{{ID: "1", Title: "A", Seconds: 99}, {ID: "2", Title: "B", Seconds: 100}, {ID: "3", Title: "C", Seconds: 50, Watched: true}}
	got := filterMovies(movies, "", 100, nil, "", "runtime", 10)
	if len(got) != 2 || got[0].ID != "3" {
		t.Fatalf("%v", got)
	}
	got = filterMovies(movies, "", 100, &yes, "", "runtime", 10)
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("unwatched %v", got)
	}
}

func TestPDFTextExtraction(t *testing.T) {
	pdf := []byte("%PDF-1.4\n1 0 obj\n<< /Length 60 >>\nstream\nBT /F1 12 Tf (Rechnungsbetrag: 128,40 EUR) Tj T* (F\\344llig 15.11.2026) Tj ET\nendstream\nendobj\n")
	text := extractPDFText(pdf)
	if !strings.Contains(text, "128,40 EUR") || !strings.Contains(text, "Fällig 15.11.2026") {
		t.Fatalf("%q", text)
	}
}

func TestBareMarkersAndQuarters(t *testing.T) {
	valid := func(r string) bool { return r == "Q1" }
	text, cited, _ := CleanCitations("Die Quelle ist Q1. Umsatz im Q1 2026 stieg.", valid)
	if text != "Die Quelle ist [Q1]. Umsatz im Q1 2026 stieg." || fmt.Sprint(cited) != "[Q1]" {
		t.Fatalf("%q %v", text, cited)
	}
}

type guardTools struct{ calls int }

func (g *guardTools) Definitions() []toolDef { return movieTools() }
func (g *guardTools) Call(context.Context, string, json.RawMessage) ToolResult {
	g.calls++
	return ToolResult{Content: `{"anzahl":0,"treffer":[]}`, Trace: ToolTrace{Name: "search_movies", Status: "done"}}
}

func TestGuardWithdrawsClaimWithoutTool(t *testing.T) {
	step := 0
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step++
		var msg map[string]any
		switch step {
		case 1:
			msg = map[string]any{"role": "assistant", "content": "Ich habe in deiner Bibliothek nichts gefunden."}
		case 2:
			msg = map[string]any{"role": "assistant", "content": "", "tool_calls": []map[string]any{{"function": map[string]any{"name": "search_movies", "arguments": map[string]any{"query": "Mondmann"}}}}}
		default:
			msg = map[string]any{"role": "assistant", "content": "Es gibt keinen Film namens Mondmann."}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"message": msg})
		_ = json.NewEncoder(w).Encode(map[string]any{"done": true})
	}))
	defer engine.Close()
	tools := &guardTools{}
	resets := 0
	h := &Harness{Client: http.DefaultClient, Base: engine.URL, Model: "m"}
	res, err := h.Run(context.Background(), []chatMessage{{Role: "user", Content: "Gibt es den Mondmann?"}}, tools, func(ev HarnessEvent) {
		if ev.Type == "reset" {
			resets++
		}
	})
	if err != nil || resets != 1 || tools.calls != 1 || strings.Contains(res.Text, "Bibliothek nichts") || !strings.Contains(res.Text, "keinen Film") {
		t.Fatalf("err=%v resets=%d calls=%d text=%q", err, resets, tools.calls, res.Text)
	}
}
