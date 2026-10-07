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
	mux.HandleFunc("POST /api/maintenance/{operation}", m.maintenanceHandler)
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) { respond(w, m.State()) })
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]any{"csrf": m.token, "nativeOwner": m.isNativeOwner(r)})
	})
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
		if r.Header.Get("X-Mutti-Native-Owner") != "" && !m.isNativeOwner(r) {
			http.Error(w, "Die App-Freigabe ist abgelaufen. Bitte Mutti erneut öffnen.", 403)
			return
		}
		input.nativeOwner = m.isNativeOwner(r)
		if e := m.StartImport(r.Context(), input); e != nil {
			http.Error(w, e.Error(), 409)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
	// Restoring the pre-update data stage needs the native Mac app: in this
	// state the server does not run, so there is no server owner sign-in.
	mux.HandleFunc("POST /api/update/rollback", func(w http.ResponseWriter, r *http.Request) {
		if !m.isNativeOwner(r) {
			http.Error(w, "Nur die Mutti-App auf diesem Mac darf den Datenstand zurücksetzen.", 403)
			return
		}
		var input struct{ Snapshot string }
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		d.DisallowUnknownFields()
		if d.Decode(&input) != nil {
			http.Error(w, "Ungültige Anfrage.", 400)
			return
		}
		// Check and claim in one step, so two requests never restore at once.
		m.mu.Lock()
		blocked := m.state.Phase == "update_blocked"
		if blocked {
			m.state.Phase, m.state.Message = "update", "Die Sicherung vor dem Update wird wiederhergestellt …"
		}
		m.mu.Unlock()
		if !blocked {
			http.Error(w, "Eine Wiederherstellung vor dem Update ist nur bei gesperrtem Start möglich.", 409)
			return
		}
		if err := m.rollbackUpdate(input.Snapshot); err != nil {
			// Still blocked: the owner can retry; an interrupted rollback resumes.
			m.setPhase("update_blocked", err.Error())
			http.Error(w, err.Error(), 409)
			return
		}
		m.setPhase("idle", "")
		select {
		case m.unblock <- struct{}{}:
		default:
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
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || (subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Mutti-CSRF")), []byte(m.token)) != 1 && !(strings.HasPrefix(r.URL.Path, "/api/maintenance/") && r.Header.Get("Origin") == "")) {
				http.Error(w, "Bitte die Seite neu öffnen.", 403)
				return
			}
		}
		mux.ServeHTTP(w, r)
	}), nil
}

func (m *Manager) isNativeOwner(r *http.Request) bool {
	return m.nativeOwnerToken != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Mutti-Native-Owner")), []byte(m.nativeOwnerToken)) == 1
}
