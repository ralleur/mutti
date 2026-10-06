// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// This evidence exists only in the test binary and uses synthetic backends.
// It is not a deployment grant and cannot be installed through the product API.
func syntheticQualificationPolicy() qualificationPolicy {
	base := qualificationBinding{ModelDigest: catalog[0].Digest, EngineDigest: "synthetic-engine",
		HardwareProfile: "synthetic-hardware", OSBuild: "test-os",
		AdapterDigest: "fake-adapters", ContextTokens: catalog[0].ContextTokens, Temperature: catalog[0].Temperature, Thinking: "off"}
	bound := func(code string) qualificationBinding {
		b := base
		b.Language, b.ContractDigest = code, assistantContractDigest(languagePacks[code])
		return b
	}
	p := qualificationPolicy{runtime: func() qualificationBinding { return bound("en") }}
	for _, code := range []string{"en", "de"} {
		for _, task := range []string{assistantTask, "media.search", "media.read", "media.favorite", "documents.read", "photos.search"} {
			p.records = append(p.records, qualificationRecord{ID: "synthetic-" + code + "-" + task, Task: task, Status: "passed",
				EvidenceDigest: "test-evidence", SuiteDigest: "test-suite", Binding: bound(code), Expires: time.Now().Add(time.Hour)})
		}
	}
	return p
}

func TestQualificationRequiresExactEvidence(t *testing.T) {
	p := syntheticQualificationPolicy()
	b := p.runtime()
	if _, err := p.evaluate(assistantTask, b, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Every binding dimension is significant, including hardware and prompt.
	for i := 0; i < reflect.TypeOf(b).NumField(); i++ {
		t.Run(reflect.TypeOf(b).Field(i).Name, func(t *testing.T) {
			changed := b
			v := reflect.ValueOf(&changed).Elem().Field(i)
			switch v.Kind() {
			case reflect.String:
				v.SetString(v.String() + "-changed")
			case reflect.Int:
				v.SetInt(v.Int() + 1)
			case reflect.Float64:
				v.SetFloat(v.Float() + 1)
			}
			if _, err := p.evaluate(assistantTask, changed, time.Now()); err == nil {
				t.Fatal("accepted changed deployment")
			}
		})
	}
	for _, task := range []string{"", "unknown", "system.install"} {
		if _, err := p.evaluate(task, b, time.Now()); err == nil {
			t.Fatalf("accepted %q", task)
		}
	}
	for _, status := range []string{"not_tested", "failed", "recommended", ""} {
		q := syntheticQualificationPolicy()
		q.records[0].Status = status
		if _, err := q.evaluate(assistantTask, b, time.Now()); err == nil {
			t.Fatal("accepted non-pass")
		}
	}
	for _, change := range []func(*qualificationRecord){
		func(r *qualificationRecord) { r.EvidenceDigest = "" },
		func(r *qualificationRecord) { r.SuiteDigest = "" },
		func(r *qualificationRecord) { r.ID = "" },
		func(r *qualificationRecord) { r.Expires = time.Now().Add(-time.Second) },
	} {
		q := syntheticQualificationPolicy()
		change(&q.records[0])
		if _, err := q.evaluate(assistantTask, b, time.Now()); err == nil {
			t.Fatal("accepted incomplete/expired evidence")
		}
	}
	if _, err := p.evaluate(assistantTask, qualificationBinding{}, time.Now()); err == nil {
		t.Fatal("accepted unknown runtime")
	}
}

func TestUnqualifiedProductRejectsAPIAndTools(t *testing.T) {
	e := newTestEnvWithQualification(t, "", false)
	e.configureAI() // Installation/selection is allowed, inference is not.
	e.hub.health.set(ModuleAI, "ok", "")
	var caps map[string]any
	e.do("GET", "/capabilities", "token-a", nil, &caps)
	ai := caps["modules"].(map[string]any)["ai"].(map[string]any)
	if ai["state"] != "qualification_required" {
		t.Fatalf("false readiness: %v", ai)
	}
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	var failure map[string]any
	if code := e.do("POST", "/ai/conversations/"+cid+"/messages", "token-a", map[string]string{"text": "Find my documents"}, &failure); code != 409 || failure["code"] != "qualification_required" {
		t.Fatalf("message accepted: %d %v", code, failure)
	}
	if code := e.do("POST", "/ai/runs/old/retry", "token-a", map[string]string{"conversation": cid}, &failure); code != 409 {
		t.Fatalf("retry: %d", code)
	}
	c, keys, err := e.hub.ai.load(userA, cid)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources.Items["Q1"] = &Source{Ref: "Q1", Service: "media", ObjectID: "11111111111111111111111111111111"}
	c.Proposals["legacy"] = &Proposal{ID: "legacy", Kind: "favorite", Ref: "Q1", Favorite: true, State: "pending", Expires: time.Now().Add(time.Hour)}
	if err := e.hub.ai.save(c, keys); err != nil {
		t.Fatal(err)
	}
	if code := e.do("POST", "/ai/proposals/legacy/confirm", "token-a", map[string]string{"conversation": cid}, &failure); code != 409 {
		t.Fatalf("legacy confirmation: %d", code)
	}
	// Rejection must remain possible even without a qualification.
	if code := e.do("POST", "/ai/proposals/legacy/reject", "token-a", map[string]string{"conversation": cid}, nil); code != 200 {
		t.Fatalf("reject: %d", code)
	}
	tools := e.hub.ai.toolsFor(Identity{UserID: userA}, c, languagePacks["en"])
	if len(tools.Definitions()) != 0 {
		t.Fatal("unqualified tools advertised")
	}
	tools.defs = movieTools() // A forged model call cannot bypass dispatch.
	r := tools.Call(context.Background(), "propose_favorite", json.RawMessage(`{"source":"Q1","favorite":true}`))
	if r.Trace.Status != "failed" || len(tools.proposals) != 0 {
		t.Fatal("unqualified proposal created")
	}
	if e.engine.chats.Load() != 0 {
		t.Fatal("unqualified inference started")
	}
	// A job accepted by an older version/earlier qualification cannot bypass
	// the execution boundary by already being queued.
	c.Runs = append(c.Runs, &RunRecord{ID: "queued", Assistant: "answer", State: "queued"})
	c.Messages = append(c.Messages, &Message{ID: "answer", Role: "assistant", Status: "queued"})
	if err := e.hub.ai.save(c, keys); err != nil {
		t.Fatal(err)
	}
	live := &liveRun{id: "queued", conversation: cid, owner: userA,
		identity: Identity{UserID: userA, token: "token-a"}, notify: make(chan struct{})}
	e.hub.ai.process(context.Background(), live)
	stored, _, err := e.hub.ai.load(userA, cid)
	if err != nil {
		t.Fatal(err)
	}
	if findRun(stored, "queued").State != "failed" || e.engine.chats.Load() != 0 {
		t.Fatal("queued work bypassed qualification")
	}
	e.jf.mu.Lock()
	defer e.jf.mu.Unlock()
	if len(e.jf.favorites) != 0 {
		t.Fatal("unqualified write reached backend")
	}
}

func TestLegacyProposalDeniedEvenWithCurrentQualification(t *testing.T) {
	e := newTestEnv(t, "")
	e.configureAI()
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	c, keys, _ := e.hub.ai.load(userA, cid)
	c.Proposals["old"] = &Proposal{ID: "old", Kind: "favorite", State: "pending", Expires: time.Now().Add(time.Hour)}
	if err := e.hub.ai.save(c, keys); err != nil {
		t.Fatal(err)
	}
	if code := e.do("POST", "/ai/proposals/old/confirm", "token-a", map[string]string{"conversation": cid}, nil); code != 409 {
		t.Fatalf("legacy promoted: %d", code)
	}
}

func TestModelChangeInvalidatesPendingProposal(t *testing.T) {
	e := newTestEnv(t, "")
	e.configureAI()
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	c, keys, _ := e.hub.ai.load(userA, cid)
	grant, err := e.hub.ai.qualify("media.favorite", languagePacks["en"])
	if err != nil {
		t.Fatal(err)
	}
	c.Sources.Items["Q1"] = &Source{Ref: "Q1", Service: "media", ObjectID: "11111111111111111111111111111111"}
	c.Proposals["pending"] = &Proposal{ID: "pending", Kind: "favorite", Ref: "Q1", Qualification: grant, Favorite: true, State: "pending", Expires: time.Now().Add(time.Hour)}
	if err := e.hub.ai.save(c, keys); err != nil {
		t.Fatal(err)
	}
	if err := e.hub.store.Update(func(c *Config) error { c.Modules[ModuleAI].Model = catalog[1].ID; return nil }); err != nil {
		t.Fatal(err)
	}
	if code := e.do("POST", "/ai/proposals/pending/confirm", "token-a", map[string]string{"conversation": cid}, nil); code != 409 {
		t.Fatalf("changed model: %d", code)
	}
	e.jf.mu.Lock()
	defer e.jf.mu.Unlock()
	if len(e.jf.favorites) != 0 {
		t.Fatal("stale proposal executed")
	}
}
