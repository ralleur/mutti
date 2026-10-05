// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceTransportBoundary(t *testing.T) {
	for _, address := range []string{"http://192.168.1.10:8096", "https://user:secret@example.org", "https://example.org/?api_key=secret", "file:///tmp/data", "https://example.org/../private", "http://localhost.attacker.example:8096"} {
		if _, e := NewAPI(address); e == nil {
			t.Fatalf("accepted %s", address)
		}
	}
	for _, address := range []string{"http://127.0.0.1:8096", "http://[::1]:8096", "https://jellyfin.example.org/base"} {
		if _, e := NewAPI(address); e != nil {
			t.Fatal(e)
		}
	}
	reached := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer redirect.Close()
	a, _ := NewAPI(redirect.URL)
	if e := a.call(context.Background(), "POST", "/Users/AuthenticateByName", map[string]string{"Pw": "fixture-secret"}, nil); e == nil {
		t.Fatal("redirect accepted")
	}
	if reached {
		t.Fatal("credentials followed redirect")
	}
}
func TestArchiveBoundaries(t *testing.T) {
	for _, name := range []string{"../outside", "Config/../../outside", "/etc/passwd", "C:/outside", "Config\\outside", "Config/system.xml/../network.xml", "Config//file", "a\x00b"} {
		if safeEntry(name) {
			t.Fatal("unsafe path accepted", name)
		}
	}
	for _, test := range []struct {
		name    string
		entries []string
		symlink bool
	}{{"case collision", []string{"Config/system.xml", "config/SYSTEM.xml"}, false}, {"symlink", []string{"Config/system.xml"}, true}, {"incomplete", []string{"manifest.json"}, false}} {
		t.Run(test.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "test.zip")
			f, _ := os.Create(p)
			w := zip.NewWriter(f)
			for _, name := range test.entries {
				h := &zip.FileHeader{Name: name}
				if test.symlink {
					h.SetMode(os.ModeSymlink | 0777)
				}
				entry, _ := w.CreateHeader(h)
				_, _ = io.WriteString(entry, `{"ServerVersion":"12.1.0","BackupEngineVersion":"0.2.0","Options":{"Database":true}}`)
			}
			w.Close()
			f.Close()
			z, e := zip.OpenReader(p)
			if e != nil {
				t.Fatal(e)
			}
			defer z.Close()
			if validateArchive(z) == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}
func TestSemanticAuditPreservesRightsAndPlayback(t *testing.T) {
	audit := func(table, rows string) Audit {
		t.Helper()
		a := newAudit()
		if e := transformJSON(strings.NewReader(rows), io.Discard, nil, table, &a, false); e != nil {
			t.Fatal(e)
		}
		return a
	}
	a := audit("Permissions", `[{"Id":1,"UserId":"viewer","Kind":0,"Value":false}]`)
	b := audit("Permissions", `[{"Id":40,"UserId":"viewer","Kind":0,"Value":false}]`)
	if e := CompareAudit(a, b); e != nil {
		t.Fatal("generated row IDs are not semantic", e)
	}
	b = audit("Permissions", `[{"Id":40,"UserId":"viewer","Kind":0,"Value":true}]`)
	if CompareAudit(a, b) == nil {
		t.Fatal("privilege escalation accepted")
	}
	for _, rows := range []string{`[{"UserId":"viewer","ItemId":"film","PlaybackPositionTicks":20,"IsFavorite":true}]`, `[]`} {
		a = audit("UserData", `[{"UserId":"viewer","ItemId":"film","PlaybackPositionTicks":10,"IsFavorite":true}]`)
		b = audit("UserData", rows)
		if CompareAudit(a, b) == nil {
			t.Fatal("lost or changed playback accepted")
		}
	}
	a = audit("UserData", `[]`)
	b = audit("UserData", `[{"ItemId":"new"}]`)
	if CompareAudit(a, b) == nil {
		t.Fatal("empty source table not checked")
	}
	if CompareAudit(a, newAudit()) == nil {
		t.Fatal("missing empty table accepted")
	}
}
func TestImportClearsOnlySessions(t *testing.T) {
	for _, table := range []string{"Devices", "ApiKeys", "DeviceOptions", "ActivityLogs", "Users"} {
		var out bytes.Buffer
		a := newAudit()
		if e := transformJSON(strings.NewReader(`[{"Id":"fixture"}]`), &out, nil, table, &a, true); e != nil {
			t.Fatal(e)
		}
		empty := out.String() == "[]"
		if empty != (table == "Devices" || table == "ApiKeys" || table == "DeviceOptions") {
			t.Fatal(table, out.String())
		}
	}
	a := newAudit()
	if transformJSON(strings.NewReader(`[{"AuthenticationProviderId":"LDAP.Provider"}]`), io.Discard, nil, "Users", &a, true) == nil {
		t.Fatal("external login imported without provider")
	}
}
func TestHTTPProtectionAndOnboarding(t *testing.T) {
	m := &Manager{Options: Options{Origin: "http://127.0.0.1:18594"}, token: randomID(), state: State{Ready: true, Active: t.TempDir(), Target: "http://127.0.0.1:18596/web/"}}
	h, e := m.Handler()
	if e != nil {
		t.Fatal(e)
	}
	for _, tt := range []struct {
		name, host, origin, token string
		want                      int
	}{{"wrong host", "attacker.example", "", m.token, 403}, {"cross origin", "127.0.0.1:18594", "https://attacker.example", m.token, 403}, {"no token", "127.0.0.1:18594", "", "", 403}, {"valid", "127.0.0.1:18594", "http://127.0.0.1:18594", m.token, 200}} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/new", strings.NewReader("{}"))
			r.Host = tt.host
			r.Header.Set("Origin", tt.origin)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Mutti-CSRF", tt.token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	if !m.State().NewSetup {
		t.Fatal("choice not retained")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "127.0.0.1:18594"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Aus Jellyfin übernehmen") || w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("onboarding missing")
	}
}
func TestFailedImportKeepsPreviousInstance(t *testing.T) {
	m := &Manager{Options: Options{Root: t.TempDir()}, state: State{Active: "previous"}}
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/System/Info/Public" {
			_ = json.NewEncoder(w).Encode(PublicInfo{Id: "existing", StartupWizardCompleted: true})
		} else {
			w.WriteHeader(401)
		}
	}))
	defer source.Close()
	m.api, _ = NewAPI(source.URL)
	m.Options.Backend = source.URL
	e := m.importSource(context.Background(), SourceInput{Address: source.URL, Replace: false})
	if e == nil {
		t.Fatal("replacement without confirmation")
	}
	if m.State().Active != "previous" {
		t.Fatal("active changed")
	}
	if _, e = os.Stat(filepath.Join(m.Options.Root, "active-instance.json")); !os.IsNotExist(e) {
		t.Fatal("pointer written")
	}
}
func TestRestoredFilesAndMapping(t *testing.T) {
	if p := replacePath("/media2/film", map[string]string{"/media": "/mnt/media"}); p != "/media2/film" {
		t.Fatal("prefix collision")
	}
	data, e := rewriteXML(append([]byte{0xef, 0xbb, 0xbf}, []byte(`<?xml version="1.0"?><Options><Path>/media/film</Path></Options>`)...), map[string]string{"/media": "/mnt/media"}, nil)
	if e != nil || !bytes.Contains(data, []byte("/mnt/media/film")) {
		t.Fatal(e, string(data))
	}
	if _, e = rewriteXML([]byte(`<!DOCTYPE x><Options/>`), nil, nil); e == nil {
		t.Fatal("XML directive allowed")
	}
	a := newAudit()
	a.Files["Data/playlists/list.xml"] = "wrong"
	instance := t.TempDir()
	if VerifyFiles(a, instance) == nil {
		t.Fatal("missing file accepted")
	}
}
