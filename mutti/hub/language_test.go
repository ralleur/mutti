// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLanguagePacksRecogniseRequestsAndDrafts(t *testing.T) {
	en, de := languagePacks["en"], languagePacks["de"]
	for _, c := range []struct {
		lang *languagePack
		got  bool
		want bool
		name string
	}{
		{en, en.favoriteRequest.MatchString("Please mark Nordlicht as a favourite."), true, "en favourite request"},
		{en, en.favoriteRequest.MatchString("Remove Lange Reise from my favorites"), true, "en remove favourite"},
		{en, en.favoriteRequest.MatchString("Which movies are short?"), false, "en no request"},
		{en, en.claimed.MatchString("Nordlicht has been marked as a favourite."), true, "en claim"},
		{en, en.claimed.MatchString("I've prepared a suggestion. Please confirm it."), false, "en honest proposal"},
		{en, en.nothingFound.MatchString("I couldn't find a movie with that name."), true, "en nothing found"},
		{en, en.unverifiedClaim.MatchString("I have searched your documents but found nothing."), true, "en unverified"},
		{de, de.favoriteRequest.MatchString("Markiere Nordlicht als Favorit."), true, "de favourite request"},
		{de, de.claimed.MatchString("Der Film wurde als Favorit markiert."), true, "de claim"},
		{en, en.wrongLanguage("Die Rechnung ist noch nicht bezahlt und der Betrag ist hoch."), true, "German answer for English user"},
		{en, en.wrongLanguage("The invoice is not paid yet; the amount is 128,40 EUR."), false, "English answer"},
		{de, de.wrongLanguage("The invoice is not paid yet and it is due in November."), true, "English answer for German user"},
		{de, de.wrongLanguage("Die Rechnung lautet „Invoice 4711“ und ist offen."), false, "German answer with English title"},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %v", c.name, c.got)
		}
	}
	if got := en.searchTerms("How much does my home insurance cost per year?"); got != "home OR insurance OR cost OR per OR year" {
		t.Errorf("en terms %q", got)
	}
}

func TestRequestLanguageSelection(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Language", "fr-FR, de-DE;q=0.8, en;q=0.5")
	for _, c := range []struct{ explicit, want string }{{"", "de"}, {"en-GB", "en"}, {"DE", "de"}} {
		l, err := requestLanguage(c.explicit, r)
		if err != nil || l.Code != c.want {
			t.Errorf("%q: %v %v", c.explicit, l, err)
		}
	}
	if _, err := requestLanguage("tlh", r); err == nil {
		t.Error("unsupported explicit language accepted")
	}
	if l, _ := requestLanguage("", httptest.NewRequest("GET", "/", nil)); l.Code != defaultLanguage {
		t.Errorf("default %s", l.Code)
	}
}

func TestQualificationIsBoundToLanguage(t *testing.T) {
	p := syntheticQualificationPolicy()
	var kept []qualificationRecord
	for _, r := range p.records {
		if r.Binding.Language == "de" {
			kept = append(kept, r)
		}
	}
	p.records = kept
	e := newTestEnvWithQualification(t, "", false)
	e.configureAI()
	e.hub.ai.qualification = p
	if _, err := e.hub.ai.qualify(assistantTask, languagePacks["de"]); err != nil {
		t.Fatalf("de: %v", err)
	}
	if _, err := e.hub.ai.qualify(assistantTask, languagePacks["en"]); err == nil {
		t.Fatal("German evidence unlocked English answers")
	}
	// The contract differs per language, so evidence cannot be relabelled.
	if assistantContractDigest(languagePacks["de"]) == assistantContractDigest(languagePacks["en"]) {
		t.Fatal("language not part of the contract digest")
	}
	var caps map[string]any
	e.do("GET", "/capabilities?language=en", "token-a", nil, &caps)
	if caps["modules"].(map[string]any)["ai"].(map[string]any)["state"] == "ready" {
		t.Fatal("English reported ready")
	}
}

func TestEnglishHarnessCorrectsClaimInEnglish(t *testing.T) {
	h, seen := scriptedModel(t, `call:propose_favorite:{"source":"Q1","favorite":true}`, "Nordlicht has been marked as a favourite.", "Done, it is now marked as a favourite.")
	h.Lang = languagePacks["en"]
	fav := &stubTools{results: map[string]string{"propose_favorite": `{"proposal":"created","source":"Q1"}`}}
	res, err := h.Run(context.Background(), []chatMessage{{Role: "system", Content: SystemPrompt("m", time.Now(), nil, h.Lang)}, {Role: "user", Content: "Mark Nordlicht as my favourite."}}, fav, func(HarnessEvent) {})
	if err != nil || fmt.Sprint(res.Interventions) != "[claim claim_fallback]" || res.Text != languagePacks["en"].proposalNotice {
		t.Fatalf("%v %v %q", err, res.Interventions, res.Text)
	}
	notes := 0
	for _, m := range (*seen)[len(*seen)-1] {
		if strings.HasPrefix(m.Content, serverNoteMarker) {
			notes++
			if !strings.Contains(m.Content, "proposal is waiting") {
				t.Fatalf("note %q", m.Content)
			}
		}
	}
	if notes != 1 {
		t.Fatalf("notes %d", notes)
	}
	if !strings.Contains(SystemPrompt("m", time.Now(), nil, languagePacks["de"]), "Answer in German") {
		t.Fatal("German answer language not requested")
	}
}
