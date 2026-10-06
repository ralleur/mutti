// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"bytes"
	"context"
	"net/url"
	"testing"
	"time"
)

// Findings of the P1 review (2026-10-06) as regression tests.

func TestHistoryCheckWithAttachmentsDoesNotRace(t *testing.T) {
	e := newContentEnv(t, "")
	id, _ := e.hub.jf.identify(context.Background(), "token-a", 0)
	book := &sourceBook{Items: map[string]*Source{}}
	doc := book.add(Source{Service: "documents", Kind: "document", ObjectID: "1"})
	movie := book.add(Source{Service: "media", Kind: "movie", ObjectID: "11111111111111111111111111111111"})
	att := book.add(Source{Service: "attachment", Kind: "attachment", ObjectID: "att1"})
	gone := book.add(Source{Service: "photos", Kind: "photo", ObjectID: "unmapped"})
	c := &Conversation{ID: randomID(), Owner: userA, Sources: *book, Attachments: []*Attachment{{ID: "att1"}},
		Messages: []*Message{{ID: "m1", Role: "assistant", Status: "completed", Used: []string{doc.Ref, movie.Ref, att.Ref, gone.Ref}}}}
	for i := 0; i < 20; i++ {
		if omit := e.hub.ai.revokedHistory(context.Background(), id, c, ""); !omit["m1"] {
			t.Fatal("unverifiable photo source not omitted")
		}
	}
}

func TestJournaledActionCannotBeRejectedOrExpire(t *testing.T) {
	e := newTestEnv(t, "")
	e.configureAI()
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	first, _ := e.ask("token-a", cid, "Suche alle ungesehenen Filme unter 100 Sekunden.", "r1")
	e.events(runID(first), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "done" })
	second, _ := e.ask("token-a", cid, "Markiere Nordlicht als Favorit.", "r2")
	events, _ := e.events(runID(second), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "done" })
	pid := events[len(events)-1].Data["proposals"].([]any)[0].(map[string]any)["id"].(string)
	if err := e.hub.journal.begin(ActionEntry{ID: pid, Profile: userA, Task: "media.favorite"}); err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if code := e.do("POST", "/ai/proposals/"+pid+"/reject", "token-a", map[string]string{"conversation": cid}, &p); code != 200 || p["state"] != "outcome_unknown" {
		t.Fatalf("reject after journaled action: %d %v", code, p)
	}
}

func TestSourcesOfAReplacedBackendStayApart(t *testing.T) {
	e := newContentEnv(t, "")
	e.configureAI() // tools need a qualified deployment (synthetic in tests)
	id, _ := e.hub.jf.identify(context.Background(), "token-a", 0)
	book := &sourceBook{Items: map[string]*Source{}}
	tools := &profileTools{hub: e.hub, id: id, sources: book, defs: documentTools(), docs: hubDocuments{e.hub.docs, id}, language: languagePacks["en"]}
	_ = tools.Call(context.Background(), "search_documents", []byte(`{"query":"Mietvertrag"}`))
	old := book.Items["Q1"]
	if old == nil || old.Instance == "" {
		t.Fatalf("no instance on source: %+v", book.Items)
	}
	e.hub.stampSources(book)
	// The owner points documents at another service; object 3 there is unrelated.
	if err := e.hub.store.Update(func(c *Config) error { c.Modules[ModuleDocuments].Instance = randomID(); return nil }); err != nil {
		t.Fatal(err)
	}
	book.touched = nil
	_ = tools.Call(context.Background(), "search_documents", []byte(`{"query":"Mietvertrag"}`))
	if book.Items["Q2"] == nil || book.Items["Q2"].ObjectID != old.ObjectID {
		t.Fatalf("same object number in the new backend reused the old marker: %+v", book.Items)
	}
	if _, ok := e.hub.sourceKey(old); ok {
		t.Fatal("old source still maps to the replaced backend")
	}
	e.hub.stampSources(book)
	if book.Items["Q1"].Content.ContentID == book.Items["Q2"].Content.ContentID {
		t.Fatal("old marker re-stamped with the new object")
	}
}

func TestReadingAnEarlierSourceCountsAsUsed(t *testing.T) {
	book := &sourceBook{Items: map[string]*Source{}}
	book.add(Source{Service: "documents", Kind: "document", ObjectID: "1", Title: "Mietvertrag"})
	book.touched = nil // from an earlier run
	text := "Kündigungsfrist drei Monate"
	tools := &profileTools{sources: book, defs: documentTools(), docs: readDocs{text}}
	_ = tools.Call(context.Background(), "read_document", []byte(`{"source":"Q1"}`))
	if !book.touched["Q1"] {
		t.Fatal("read source not recorded as used")
	}
}

type readDocs struct{ text string }

func (r readDocs) Search(context.Context, string) ([]Document, error) { return nil, nil }
func (r readDocs) Read(context.Context, int) (Document, error) {
	return Document{ID: 1, Title: "Mietvertrag", Content: &r.text}, nil
}

func TestNoAttachmentWhileAnAnswerRuns(t *testing.T) {
	e := newTestEnv(t, "")
	e.engine.delay = 200 * time.Millisecond
	e.configureAI()
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	started, _ := e.ask("token-a", cid, "Erzähl etwas lang", "b1")
	res, _ := e.raw("POST", "/ai/conversations/"+cid+"/attachments", "token-a", bytes.NewReader([]byte("Notiz")), "text/plain",
		map[string]string{"X-Filename": url.PathEscape("notiz.txt")})
	if res.StatusCode != 409 {
		t.Fatalf("attachment during run: %d", res.StatusCode)
	}
	e.do("POST", "/ai/runs/"+runID(started)+"/cancel", "token-a", map[string]string{}, nil)
}

func TestUnavailableAreaIsRetriedOnLaterPages(t *testing.T) {
	e := newContentEnv(t, "")
	e.docsSrv.Close()
	first, _, _ := e.search("token-a", "size=1")
	if first.Next == nil || first.Completeness != "partial" {
		t.Fatalf("first page %+v", first)
	}
	second, code, raw := e.search("token-a", "cursor="+url.QueryEscape(*first.Next))
	if code != 200 || second.Completeness != "partial" {
		t.Fatalf("later page lost the unavailable area: %d %s", code, raw)
	}
}
