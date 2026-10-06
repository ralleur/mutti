// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every message created with apiErr (and the shared health texts) must have
// an English rendering, so English users never see German.
func TestEveryUserMessageHasAnEnglishText(t *testing.T) {
	literal := regexp.MustCompile(`apiErr\(\d+, "[a-z_]+", "([^"]+)"\)`)
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, m := range literal.FindAllStringSubmatch(string(b), -1) {
			if _, ok := englishMessages[m[1]]; !ok {
				t.Errorf("%s: no English text for %q", f, m[1])
			}
		}
	}
	for _, text := range []string{errUnauthorized.Error(), unavailableMessage(ModulePhotos), unavailableMessage(ModuleDocuments), unavailableMessage(ModuleAI),
		areaUnavailable(AreaMedia), qualificationError().Error()} {
		if _, ok := englishMessages[text]; !ok {
			t.Errorf("no English text for %q", text)
		}
	}
}

func TestErrorsFollowTheRequestLanguage(t *testing.T) {
	e := newTestEnv(t, "")
	get := func(header, query string) map[string]any {
		req, _ := http.NewRequest("GET", e.server.URL+"/mutti/hub/v1/content/items/notanid"+query, nil)
		req.Header.Set("Authorization", "Bearer token-a")
		if header != "" {
			req.Header.Set("Accept-Language", header)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return out
	}
	if out := get("de-DE,de;q=0.9", ""); out["message"] != "Nicht gefunden oder nicht freigegeben." || out["code"] != "not_found" {
		t.Fatalf("de %v", out)
	}
	if out := get("en-GB", ""); out["message"] != "Not found or not shared." || out["code"] != "not_found" {
		t.Fatalf("en %v", out)
	}
	if out := get("de", "?language=en"); out["message"] != "Not found or not shared." {
		t.Fatalf("explicit %v", out)
	}
	// The shared error value itself stays unchanged for other requests.
	if errNotFound.Message != "Nicht gefunden oder nicht freigegeben." {
		t.Fatal("shared error mutated")
	}
}
