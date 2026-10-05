// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNativeReplacementAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, key, origin, extra string
		replace                  bool
		status                   int
		sourceRequests           int32
	}{
		{name: "browser still needs target admin", replace: true, status: 202},
		{name: "CSRF is not owner proof", key: "csrf", replace: true, status: 403},
		{name: "wrong native key", key: "stale", replace: true, status: 403},
		{name: "JSON cannot grant ownership", extra: `,"nativeOwner":true`, replace: true, status: 400},
		{name: "native still needs confirmation", key: "native", status: 202},
		{name: "native still rejects cross origin", key: "native", origin: "https://attacker.example", replace: true, status: 403},
		{name: "native requires source login only", key: "native", replace: true, status: 202, sourceRequests: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sourceLogins atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/System/Info/Public" {
					_ = json.NewEncoder(w).Encode(PublicInfo{Id: "target", StartupWizardCompleted: true})
					return
				}
				w.WriteHeader(401)
			}))
			defer target.Close()
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Users/AuthenticateByName" {
					sourceLogins.Add(1)
				}
				w.WriteHeader(401) // Never migrate without a valid source admin.
			}))
			defer source.Close()
			api, _ := NewAPI(target.URL)
			m := &Manager{Options: Options{Origin: "http://127.0.0.1:18594", Backend: target.URL}, api: api, token: randomID(), nativeOwnerToken: randomID(), state: State{Active: "previous"}}
			h, err := m.Handler()
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(SourceInput{Address: source.URL, Replace: tc.replace})
			if tc.extra != "" {
				body = []byte(strings.TrimSuffix(string(body), "}") + tc.extra + "}")
			}
			r := httptest.NewRequest("POST", "/api/import", strings.NewReader(string(body)))
			r.Host = "127.0.0.1:18594"
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Mutti-CSRF", m.token)
			r.Header.Set("Origin", tc.origin)
			key := tc.key
			if key == "native" {
				key = m.nativeOwnerToken
			}
			if key == "csrf" {
				key = m.token
			}
			r.Header.Set("X-Mutti-Native-Owner", key)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			m.jobs.Wait()
			if got := sourceLogins.Load(); got != tc.sourceRequests {
				t.Fatalf("source login requests: %d, want %d", got, tc.sourceRequests)
			}
			if m.State().Active != "previous" {
				t.Fatal("failed import changed active instance")
			}
			if tc.status == 202 && m.State().Phase != "error" {
				t.Fatal("unauthenticated source accepted")
			}
			for _, k := range []string{"", m.token, m.nativeOwnerToken} {
				r = httptest.NewRequest("GET", "/api/session", nil)
				r.Host = "127.0.0.1:18594"
				r.Header.Set("X-Mutti-Native-Owner", k)
				w = httptest.NewRecorder()
				h.ServeHTTP(w, r)
				var session struct {
					NativeOwner bool `json:"nativeOwner"`
				}
				_ = json.Unmarshal(w.Body.Bytes(), &session)
				if session.NativeOwner != (k == m.nativeOwnerToken) {
					t.Fatal("native availability not tied to private capability")
				}
				if strings.Contains(w.Body.String(), m.nativeOwnerToken) {
					t.Fatal("private capability leaked to page")
				}
			}
		})
	}
}

func TestBrowserReplacementWithExistingAdmin(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/System/Info/Public":
			_ = json.NewEncoder(w).Encode(PublicInfo{Id: "target", StartupWizardCompleted: true})
		case "/Users/AuthenticateByName":
			var credentials map[string]string
			_ = json.NewDecoder(r.Body).Decode(&credentials)
			if credentials["Username"] != "existing-admin" || credentials["Pw"] != "fixture-password" {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{"AccessToken":"fixture-token","User":{"Policy":{"IsAdministrator":true}}}`))
		default:
			w.WriteHeader(204)
		}
	}))
	defer target.Close()
	api, _ := NewAPI(target.URL)
	m := &Manager{api: api, Options: Options{Backend: target.URL}}
	err := m.importSource(context.Background(), SourceInput{Address: "invalid-source", Replace: true, TargetUsername: "existing-admin", TargetPassword: "fixture-password"})
	if err == nil || !strings.Contains(err.Error(), "gültige Serveradresse") {
		t.Fatal("existing admin no longer authorizes browser import", err)
	}
}

func TestNativeCapabilityCannotEnableOtherListeners(t *testing.T) {
	for _, o := range []Options{
		{Bind: "0.0.0.0", Listen: "127.0.0.1:18594", NativeOwnerToken: strings.Repeat("a", 64)},
		{Bind: "127.0.0.1", Listen: "0.0.0.0:18594", NativeOwnerToken: strings.Repeat("a", 64)},
		{Bind: "127.0.0.1", Listen: "127.0.0.1:18594", Container: true, NativeOwnerToken: strings.Repeat("a", 64)},
		{Bind: "127.0.0.1", Listen: "127.0.0.1:18594", NativeOwnerToken: "weak"},
	} {
		if _, err := NewManager(o); err == nil {
			t.Fatal("invalid native capability setup accepted")
		}
	}
}
