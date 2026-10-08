// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestManagerMessagesTranslate(t *testing.T) {
	for _, c := range []struct{ german, english string }{
		{"Bitte zuerst den Server beenden.", "Please stop the server first."},
		// A German reason inside a composed message is translated too.
		{"Das Update konnte nicht abgeschlossen werden (der neue Server beendet sich wiederholt). Der Datenstand vor dem Update ist gesichert; die vorherige Mutti-Version bietet die Wiederherstellung an.",
			"The update could not be completed (the new server keeps stopping). The pre-update data is backed up; the previous Mutti version offers to restore it."},
		{"Der Server hat die Anfrage abgelehnt (HTTP 503).", "The server rejected the request (HTTP 503)."},
		{fmt.Sprintf("Der Name der Bibliothek %q hat sich geändert.", "Filme \"alt\""), `The name of the library "Filme \"alt\"" has changed.`},
		{fmt.Errorf("Jellyfins Importvorbereitung konnte nicht geprüft werden: %w", errors.New("Ungültige Anfrage.")).Error(),
			"Jellyfin's import preparation could not be checked: Invalid request."},
		// Technical details of other programs stay as they are.
		{"Das Mutti-Paket ist beschädigt oder verändert (files differ from the component list: web/index.html). Bitte Mutti neu installieren. Es wurde nichts verändert.",
			"The Mutti package is damaged or modified (files differ from the component list: web/index.html). Please reinstall Mutti. Nothing was changed."},
		{"open /x: no such file", "open /x: no such file"},
	} {
		if got := translateMessage(c.german); got != c.english {
			t.Errorf("%q\n got %q\nwant %q", c.german, got, c.english)
		}
	}
}

func TestManagerLanguageSelection(t *testing.T) {
	for _, c := range []struct {
		query, header string
		english       bool
	}{
		{"", "", false}, {"", "en-US,en;q=0.9", true}, {"", "de-DE,de;q=0.9,en;q=0.8", false},
		{"", "fr-FR,en;q=0.5", true}, {"?language=en", "de-DE", true}, {"?language=de", "en-US", false},
	} {
		r := httptest.NewRequest("GET", "/api/state"+c.query, nil)
		if c.header != "" {
			r.Header.Set("Accept-Language", c.header)
		}
		if got := englishRequest(r); got != c.english {
			t.Errorf("%q %q: english %v", c.query, c.header, got)
		}
	}
}

// The setup page is German at its source; i18n.js must cover every visible
// text of index.html and every German literal app.js shows.
func TestSetupPageHasEnglishTexts(t *testing.T) {
	read := func(name string) string {
		b, err := assets.ReadFile("web/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	dict := map[string]bool{}
	for _, m := range regexp.MustCompile(`'((?:[^'\\]|\\.)*)':'`).FindAllStringSubmatch(read("i18n.js"), -1) {
		dict[m[1]] = true
	}
	page := read("index.html")
	var texts []string
	for _, m := range regexp.MustCompile(`>([^<>]+)<`).FindAllStringSubmatch(page, -1) {
		texts = append(texts, strings.TrimSpace(m[1]))
	}
	for _, m := range regexp.MustCompile(`(?:placeholder|aria-label|title)="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		texts = append(texts, m[1])
	}
	german := regexp.MustCompile(`[äöüßÄÖÜ]|\b(der|die|das|und|nicht|bitte|deine?|Mutti|Jellyfin|Benutzer|Bibliotheken|Pfad|Ordner)\b`)
	for _, m := range regexp.MustCompile(`'((?:[^'\\]|\\.)*)'`).FindAllStringSubmatch(read("app.js"), -1) {
		if german.MatchString(m[1]) && !strings.HasPrefix(m[1], "/") && strings.ContainsAny(m[1], " äöü") {
			texts = append(texts, m[1])
		}
	}
	for _, text := range texts {
		if text == "" || !regexp.MustCompile(`\pL`).MatchString(text) || text == "Mutti" || strings.HasPrefix(text, "http") {
			continue
		}
		if !dict[text] {
			t.Errorf("no English text for %q", text)
		}
	}
}

// Every German text the manager can show has an English text. German is
// the message ID; composed messages are fmt formats in the table.
func TestEveryManagerMessageHasAnEnglishText(t *testing.T) {
	german := regexp.MustCompile(`[äöüßÄÖÜ…]|\b(der|die|das|ist|nicht|bitte|Bitte|wird|wurde|kann|keine?|und|oder|mit|für|vor|nach|Ungültige?r?|Unbekannte?r?|Sicherung|Daten|Server|Datei|fehlgeschlagen|unbekannt)\b`)
	notShown := map[string]bool{"Mutti Import": true, "… (%d in total)": true} // client name; English detail
	files, _ := filepath.Glob("*.go")
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "messages.go" {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil || !strings.Contains(text, " ") || !german.MatchString(text) || notShown[text] {
				return true
			}
			checked++
			if _, ok := englishMessages[text]; !ok {
				t.Errorf("%s: no English text for %q", f, text)
			}
			return true
		})
	}
	if checked < 150 {
		t.Fatalf("only %d texts checked", checked)
	}
}

func TestStateFollowsTheRequestLanguage(t *testing.T) {
	f := newUpdateFixture(t)
	f.m.Options.Origin = "http://127.0.0.1:18594"
	f.m.token = randomID()
	f.m.setPhase("update_blocked", "Bitte zuerst den Server beenden.")
	f.m.setUpdate(&UpdateState{State: "failed", Message: fmt.Sprintf("Das Update konnte nicht abgeschlossen werden (%s). Der Datenstand vor dem Update ist gesichert; die vorherige Mutti-Version bietet die Wiederherstellung an.", "der neue Server beendet sich wiederholt")})
	h, err := f.m.Handler()
	if err != nil {
		t.Fatal(err)
	}
	get := func(language string) State {
		r := httptest.NewRequest("GET", "/api/state", nil)
		r.Host = "127.0.0.1:18594"
		if language != "" {
			r.Header.Set("Accept-Language", language)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var s State
		_ = json.Unmarshal(w.Body.Bytes(), &s)
		return s
	}
	if s := get("en-US"); s.Message != "Please stop the server first." || !strings.HasPrefix(s.Update.Message, "The update could not be completed (the new server keeps stopping).") {
		t.Fatalf("en %q / %q", s.Message, s.Update.Message)
	}
	if s := get("de-DE"); s.Message != "Bitte zuerst den Server beenden." {
		t.Fatalf("de %q", s.Message)
	}
	if f.m.State().Message != "Bitte zuerst den Server beenden." {
		t.Fatal("stored state was translated")
	}
}
