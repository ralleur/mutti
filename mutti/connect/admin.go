// SPDX-License-Identifier: MPL-2.0
package connect

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/hashicorp/yamux"
	"github.com/skip2/go-qrcode"
	"net/http"
	"net/url"
	"strings"
	"time"
)

//go:embed admin.html
var adminHTML []byte

//go:embed sora.woff2
var adminFont []byte

//go:embed sora-semibold.woff2
var adminSemibold []byte

//go:embed sora-bold.woff2
var adminBold []byte

//go:embed mark-light.svg
var adminMark []byte

//go:embed wordmark-light.svg
var adminWordmark []byte

type jellyUser struct {
	policySnapshot string
	ID             string `json:"Id"`
	Name           string `json:"Name"`
	Policy         struct {
		IsAdministrator bool `json:"IsAdministrator"`
		IsDisabled      bool `json:"IsDisabled"`
	} `json:"Policy"`
}

// Keep the complete policy for stream invalidation, including future upstream
// permission fields. It never becomes a client-supplied authorization context.
func (u *jellyUser) UnmarshalJSON(data []byte) error {
	type plain jellyUser
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var raw struct{ Policy map[string]any }
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	policy, err := json.Marshal(raw.Policy)
	if err != nil {
		return err
	}
	*u = jellyUser(value)
	u.policySnapshot = string(policy)
	return nil
}

func (s *Server) Admin(origin string) http.Handler {
	parsed, _ := url.Parse(origin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; font-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if r.Host != parsed.Host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) {
			http.Error(w, "forbidden", 403)
			return
		}
		if r.Method == "GET" {
			var data []byte
			contentType := "font/woff2"
			switch r.URL.Path {
			case "/sora.woff2":
				data = adminFont
			case "/sora-semibold.woff2":
				data = adminSemibold
			case "/sora-bold.woff2":
				data = adminBold
			case "/mark-light.svg":
				data = adminMark
				contentType = "image/svg+xml"
			case "/wordmark-light.svg":
				data = adminWordmark
				contentType = "image/svg+xml"
			}
			if data != nil {
				w.Header().Set("Content-Type", contentType)
				_, _ = w.Write(data)
				return
			}
		}
		if r.URL.Path == "/" && r.Method == "GET" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(adminHTML)
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			w.WriteHeader(415)
			return
		}
		if r.URL.Path == "/login" {
			var input struct {
				Username string `json:"Username"`
				Password string `json:"Pw"`
			}
			if decodeBody(w, r, &input) != nil || len(input.Username) > 128 || len(input.Password) > 1024 {
				w.WriteHeader(400)
				return
			}
			b, _ := json.Marshal(input)
			var result struct {
				AccessToken string    `json:"AccessToken"`
				User        jellyUser `json:"User"`
			}
			if e := s.jf(r.Context(), "POST", "/Users/AuthenticateByName", "", bytes.NewReader(b), &result); e != nil || !result.User.Policy.IsAdministrator {
				http.Error(w, "Bitte mit dem lokalen Besitzerzugang anmelden.", 401)
				return
			}
			jsonReply(w, 200, map[string]string{"token": result.AccessToken})
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		var owner jellyUser
		if len(token) < 20 || s.jf(r.Context(), "GET", "/Users/Me", token, nil, &owner) != nil || !owner.Policy.IsAdministrator || owner.Policy.IsDisabled {
			http.Error(w, "Besitzeranmeldung erforderlich.", 401)
			return
		}
		switch r.URL.Path {
		case "/state":
			var users []jellyUser
			if s.jf(r.Context(), "GET", "/Users", token, nil, &users) != nil {
				http.Error(w, "Profile konnten nicht geladen werden.", 502)
				return
			}
			profiles := []jellyUser{}
			for _, u := range users {
				if !u.Policy.IsAdministrator && !u.Policy.IsDisabled {
					profiles = append(profiles, u)
				}
			}
			s.mu.Lock()
			s.prune(time.Now())
			pendingList := []pending{}
			for _, p := range s.pending {
				pendingList = append(pendingList, p)
			}
			devices := []map[string]any{}
			for _, d := range s.state.Devices {
				devices = append(devices, map[string]any{"pin": d.Pin, "name": d.Name, "created": d.Created, "userId": d.UserID})
			}
			s.mu.Unlock()
			jsonReply(w, 200, map[string]any{"pending": pendingList, "devices": devices, "profiles": profiles, "broker": s.Broker, "remote": strings.HasPrefix(s.Broker, "https://"), "relay": false})
		case "/invite":
			i, e := s.NewInvitation()
			if e != nil {
				http.Error(w, e.Error(), 429)
				return
			}
			jsonReply(w, 200, map[string]any{"url": i.URL(), "expires": i.Expires})
		case "/qr":
			var req struct {
				URL string `json:"url"`
			}
			if decodeBody(w, r, &req) != nil {
				w.WriteHeader(400)
				return
			}
			i, e := ParseInvitation(req.URL)
			if e != nil || i.Pin != s.state.Identity.Pin() {
				w.WriteHeader(400)
				return
			}
			png, e := qrcode.Encode(req.URL, qrcode.Medium, 512)
			if e != nil {
				w.WriteHeader(500)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		case "/approve":
			var req struct {
				Pin    string `json:"pin"`
				UserID string `json:"userId"`
			}
			if decodeBody(w, r, &req) != nil || !validID(req.Pin) {
				w.WriteHeader(400)
				return
			}
			if e := s.approve(r.Context(), req.Pin, req.UserID, token); e != nil {
				http.Error(w, e.Error(), 409)
				return
			}
			w.WriteHeader(204)
		case "/revoke":
			var req struct {
				Pin string `json:"pin"`
			}
			if decodeBody(w, r, &req) != nil || !validID(req.Pin) {
				w.WriteHeader(400)
				return
			}
			if e := s.Revoke(req.Pin); e != nil {
				http.Error(w, "Sperren fehlgeschlagen.", 500)
				return
			}
			w.WriteHeader(204)
		case "/profile":
			var req struct {
				Name string `json:"Name"`
			}
			if decodeBody(w, r, &req) != nil || len(strings.TrimSpace(req.Name)) == 0 || len(req.Name) > 80 {
				w.WriteHeader(400)
				return
			}
			b, _ := json.Marshal(map[string]string{"Name": req.Name, "Password": randomID()})
			var profile jellyUser
			if e := s.jf(r.Context(), "POST", "/Users/New", token, bytes.NewReader(b), &profile); e != nil {
				http.Error(w, "Profil konnte nicht erstellt werden.", 409)
				return
			}
			jsonReply(w, 200, profile)
		default:
			http.NotFound(w, r)
		}
	})
}
func (s *Server) approve(ctx context.Context, pin, userID, ownerToken string) error {
	s.mu.Lock()
	s.prune(time.Now())
	p, ok := s.pending[pin]
	s.mu.Unlock()
	if !ok {
		return errors.New("Anfrage abgelaufen. Bitte neu koppeln.")
	}
	var user jellyUser
	if e := s.jf(ctx, "GET", "/Users/"+url.PathEscape(userID), ownerToken, nil, &user); e != nil || user.Policy.IsAdministrator || user.Policy.IsDisabled {
		return errors.New("Bitte ein aktives Wiedergabeprofil ohne Administratorrechte wählen.")
	}
	var qc struct {
		Code   string `json:"Code"`
		Secret string `json:"Secret"`
	}
	if e := s.jfDevice(ctx, "POST", "/QuickConnect/Initiate", "", pin, nil, &qc); e != nil {
		return errors.New("Quick Connect muss in Mutti aktiviert sein, damit das freigegebene Profil angemeldet werden kann.")
	}
	if e := s.jf(ctx, "POST", "/QuickConnect/Authorize?code="+url.QueryEscape(qc.Code)+"&userId="+url.QueryEscape(userID), ownerToken, nil, nil); e != nil {
		return e
	}
	body, _ := json.Marshal(map[string]string{"Secret": qc.Secret})
	var auth struct {
		AccessToken string          `json:"AccessToken"`
		User        json.RawMessage `json:"User"`
	}
	if e := s.jfDevice(ctx, "POST", "/Users/AuthenticateWithQuickConnect", "", pin, bytes.NewReader(body), &auth); e != nil || auth.AccessToken == "" {
		return errors.New("Profilanmeldung fehlgeschlagen.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(time.Now())
	if _, ok := s.pending[pin]; !ok {
		return errors.New("Anfrage inzwischen abgelaufen.")
	}
	d := Device{pin, p.Name, userID, auth.AccessToken, auth.User, time.Now()}
	old := s.state.Devices[pin]
	s.state.Devices[pin] = d
	if e := savePrivate(s.path, s.state); e != nil {
		if old.Pin == "" {
			delete(s.state.Devices, pin)
		} else {
			s.state.Devices[pin] = old
		}
		return e
	}
	delete(s.pending, pin)
	for k, inv := range s.invites {
		if inv.Pin == pin {
			delete(s.invites, k)
		}
	}
	return nil
}
func (s *Server) Revoke(pin string) error {
	s.mu.Lock()
	old, exists := s.state.Devices[pin]
	delete(s.state.Devices, pin)
	if e := savePrivate(s.path, s.state); e != nil {
		if exists {
			s.state.Devices[pin] = old
		}
		s.mu.Unlock()
		return e
	}
	delete(s.policies, pin)
	delete(s.pending, pin)
	for k, inv := range s.invites {
		if inv.Pin == pin {
			delete(s.invites, k)
		}
	}
	var closing []*yamux.Session
	for session := range s.sessions[pin] {
		closing = append(closing, session)
	}
	s.mu.Unlock()
	for _, session := range closing {
		_ = session.Close()
	}
	return nil
}
