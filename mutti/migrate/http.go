// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

//go:embed web/*
var assets embed.FS

func (m *Manager) Handler() (http.Handler, error) {
	origin, e := url.Parse(m.Options.Origin)
	if e != nil || origin.Host == "" || origin.Path != "" || origin.User != nil {
		return nil, errors.New("Ungültiger lokaler Ursprung.")
	}
	files, _ := fs.Sub(assets, "web")
	mux := http.NewServeMux()
	respond := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) { respond(w, m.State()) })
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) { respond(w, map[string]string{"csrf": m.token}) })
	mux.HandleFunc("GET /api/discover", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		found := Discover(ctx)
		var current PublicInfo
		_ = m.api.call(ctx, "GET", "/System/Info/Public", nil, &current)
		filtered := []map[string]string{}
		for _, s := range found {
			if s["id"] != current.Id {
				filtered = append(filtered, s)
			}
		}
		respond(w, filtered)
	})
	mux.HandleFunc("POST /api/new", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.jobCancel != nil || !m.state.Ready {
			http.Error(w, "Bitte den laufenden Vorgang abwarten.", 409)
			return
		}
		if e := privateWrite(filepath.Join(m.state.Active, "setup-new"), []byte("1")); e != nil {
			http.Error(w, "Die Auswahl konnte nicht gespeichert werden.", 500)
			return
		}
		m.state.NewSetup = true
		respond(w, map[string]string{"target": m.state.Target})
	})
	mux.HandleFunc("POST /api/import", func(w http.ResponseWriter, r *http.Request) {
		var input SourceInput
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		d.DisallowUnknownFields()
		if e := d.Decode(&input); e != nil {
			http.Error(w, "Bitte alle Felder prüfen.", 400)
			return
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			http.Error(w, "Ungültige Anfrage.", 400)
			return
		}
		if e := m.StartImport(r.Context(), input); e != nil {
			http.Error(w, e.Error(), 409)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("POST /api/cancel", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		if m.jobCancel != nil {
			m.jobCancel()
		}
		m.mu.Unlock()
		w.WriteHeader(204)
	})
	mux.Handle("GET /", http.FileServer(http.FS(files)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if r.Host != origin.Host {
			http.Error(w, "Unbekannter Host.", 403)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "Unbekannter Ursprung.", 403)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if o := r.Header.Get("Origin"); o != "" && o != m.Options.Origin {
				http.Error(w, "Unbekannter Ursprung.", 403)
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Mutti-CSRF")), []byte(m.token)) != 1 {
				http.Error(w, "Bitte die Seite neu öffnen.", 403)
				return
			}
		}
		mux.ServeHTTP(w, r)
	}), nil
}
