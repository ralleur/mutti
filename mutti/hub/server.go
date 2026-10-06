// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const Version = "0.2.0-dev"

type Options struct {
	State      string // private data directory
	Jellyfin   string // fixed loopback origin
	Host       string // Host header Jellyfin expects
	PeerSecret string // shared with the local Connect sidecar only
	Ollama     string // optional engine executable for the managed mode
	Sandbox    bool   // restrict the managed engine's network on macOS
	// Qualification overrides the signed evidence file location (tests and
	// measurement); by default it sits next to the hub executable.
	Qualification string
}

type Hub struct {
	opts    Options
	store   *Store
	jf      *jellyfin
	ai      *AI
	photos  *Photos
	docs    *Documents
	health  *healthCache
	streams *streamRegistry
	content *contentIndex
	journal *actionJournal
	// cursorKey signs search cursors; they expire with the process.
	cursorKey []byte
	mu        sync.Mutex
	log       *log.Logger
}

func New(opts Options) (*Hub, error) {
	store, err := OpenStore(opts.State)
	if err != nil {
		return nil, err
	}
	jf, err := newJellyfin(opts.Jellyfin, opts.Host)
	if err != nil {
		return nil, err
	}
	content, err := openContentIndex(opts.State)
	if err != nil {
		return nil, err
	}
	journal, err := openJournal(opts.State)
	if err != nil {
		return nil, err
	}
	h := &Hub{opts: opts, store: store, jf: jf, health: newHealthCache(), streams: newStreamRegistry(), content: content, journal: journal,
		cursorKey: newCursorKey(), log: log.New(io.Discard, "", 0)}
	// API calls are bounded; media relays and uploads only bound the wait for
	// response headers, so long videos and large originals are not cut off.
	h.photos = &Photos{hub: h, client: guardedClient(time.Minute), stream: guardedClient(0)}
	h.docs = &Documents{hub: h, client: guardedClient(time.Minute), stream: guardedClient(0)}
	if h.ai, err = newAI(h); err != nil {
		return nil, err
	}
	return h, nil
}

// Run starts background work and stops it with ctx.
func (h *Hub) Run(ctx context.Context) {
	go h.ai.run(ctx)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		h.refreshHealth(ctx)
		select {
		case <-ctx.Done():
			h.ai.shutdown()
			return
		case <-ticker.C:
		}
	}
}

// APIError carries a stable code for clients and a user message, rendered
// in the request's language when it is written.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string { return e.Message }

func apiErr(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

var (
	errNotFound  = apiErr(404, "not_found", "Nicht gefunden oder nicht freigegeben.")
	errInvalid   = apiErr(400, "invalid", "Ungültige Anfrage.")
	errForbidden = apiErr(403, "forbidden", "Für dieses Profil nicht freigegeben.")
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var api *APIError
	if !errors.As(err, &api) {
		if errors.Is(err, errUnauthorized) {
			api = apiErr(401, "unauthorized", errUnauthorized.Error())
		} else {
			api = apiErr(502, "unavailable", "Der Dienst ist gerade nicht erreichbar.")
		}
	}
	localized := *api
	localized.Message = responseLanguage(r).say(api.Message)
	writeJSON(w, api.Status, &localized)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, limit int64, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errInvalid
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return errInvalid
	}
	return nil
}

type handler func(w http.ResponseWriter, r *http.Request, id Identity) error

// Handler exposes all routes under /mutti/hub/v1. Every request is
// authenticated with the caller's own Jellyfin session.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	user := func(pattern string, fn handler) {
		mux.HandleFunc(pattern, h.wrap(fn, false))
	}
	admin := func(pattern string, fn handler) {
		mux.HandleFunc(pattern, h.wrap(fn, true))
	}
	user("GET /mutti/hub/v1/capabilities", h.capabilities)

	user("GET /mutti/hub/v1/content/search", h.contentSearch)
	user("GET /mutti/hub/v1/content/jobs", h.contentJobs)
	user("GET /mutti/hub/v1/content/items/{id}", h.contentItem)
	user("GET /mutti/hub/v1/content/items/{id}/{kind}", h.contentMedia)

	user("GET /mutti/hub/v1/ai/conversations", h.ai.listConversations)
	user("POST /mutti/hub/v1/ai/conversations", h.ai.createConversation)
	user("GET /mutti/hub/v1/ai/conversations/{id}", h.ai.getConversation)
	user("PATCH /mutti/hub/v1/ai/conversations/{id}", h.ai.renameConversation)
	user("DELETE /mutti/hub/v1/ai/conversations/{id}", h.ai.deleteConversation)
	user("POST /mutti/hub/v1/ai/conversations/{id}/messages", h.ai.postMessage)
	user("POST /mutti/hub/v1/ai/conversations/{id}/attachments", h.ai.postAttachment)
	user("GET /mutti/hub/v1/ai/conversations/{id}/attachments/{attachment}", h.ai.getAttachment)
	user("GET /mutti/hub/v1/ai/runs/{id}/events", h.ai.events)
	user("POST /mutti/hub/v1/ai/runs/{id}/cancel", h.ai.cancelRun)
	user("POST /mutti/hub/v1/ai/runs/{id}/retry", h.ai.retryRun)
	user("POST /mutti/hub/v1/ai/proposals/{id}/{decision}", h.ai.decideProposal)
	user("GET /mutti/hub/v1/ai/sources/{conversation}/{ref}", h.ai.resolveSource)

	user("GET /mutti/hub/v1/photos/assets", h.photos.list)
	user("GET /mutti/hub/v1/photos/search", h.photos.search)
	user("GET /mutti/hub/v1/photos/albums", h.photos.albums)
	user("GET /mutti/hub/v1/photos/albums/{id}", h.photos.album)
	user("GET /mutti/hub/v1/photos/assets/{id}", h.photos.asset)
	user("GET /mutti/hub/v1/photos/assets/{id}/{kind}", h.photos.media)
	user("POST /mutti/hub/v1/photos/assets", h.photos.upload)

	user("GET /mutti/hub/v1/documents", h.docs.list)
	user("GET /mutti/hub/v1/documents/tasks", h.docs.tasks)
	user("GET /mutti/hub/v1/documents/{id}", h.docs.document)
	user("GET /mutti/hub/v1/documents/{id}/{kind}", h.docs.media)
	user("POST /mutti/hub/v1/documents", h.docs.upload)

	admin("GET /mutti/hub/v1/admin/state", h.adminState)
	admin("POST /mutti/hub/v1/admin/enable/{module}", h.adminModule)
	admin("POST /mutti/hub/v1/admin/grants", h.adminGrant)
	admin("POST /mutti/hub/v1/admin/service/{module}", h.adminService)
	admin("POST /mutti/hub/v1/admin/link/{module}", h.adminLink)
	admin("POST /mutti/hub/v1/admin/unlink/{module}", h.adminUnlink)
	admin("POST /mutti/hub/v1/admin/ai/engine", h.ai.adminEngine)
	admin("POST /mutti/hub/v1/admin/ai/models/{action}", h.ai.adminModels)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Browsers never talk to the hub directly; the owner UI uses the
		// Jellyfin bridge and devices use the Connect tunnel.
		if r.Header.Get("Origin") != "" || r.URL.IsAbs() {
			writeError(w, r, errForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (h *Hub) wrap(fn handler, admin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := h.jf.identify(r.Context(), requestToken(r), 3*time.Second)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if admin != id.Admin {
			// Owner routes need the administrator; content routes are for
			// playback profiles. An administrator's private photos or chats are
			// out of scope for this package's profile model.
			if admin {
				writeError(w, r, errForbidden)
				return
			}
		}
		id.Device = peerDevice(r, h.opts.PeerSecret)
		if err = fn(w, r, id); err != nil {
			writeError(w, r, err)
		}
	}
}

// allowed returns the module configuration if the profile may use it now.
func (h *Hub) allowed(id Identity, module string) (*ModuleConfig, error) {
	cfg := h.store.Read()
	m := cfg.Modules[module]
	switch {
	case m == nil || !h.configured(module, m):
		return nil, apiErr(404, "not_configured", "Diese Funktion ist auf diesem Mutti noch nicht eingerichtet.")
	case !m.Enabled:
		return nil, apiErr(403, "disabled", "Diese Funktion ist derzeit ausgeschaltet.")
	case !m.Grants[id.UserID]:
		return nil, errForbidden
	}
	if module != ModuleAI && m.Links[id.UserID] == nil {
		return nil, apiErr(403, "not_linked", "Für dieses Profil ist noch kein Konto zugeordnet.")
	}
	return m, nil
}

func (h *Hub) configured(module string, m *ModuleConfig) bool {
	if module == ModuleAI {
		return m.Model != "" && m.Engine != nil
	}
	return m.ServiceURL != ""
}

type moduleCapability struct {
	Configured bool     `json:"configured"`
	Enabled    bool     `json:"enabled"`
	Allowed    bool     `json:"allowed"`
	State      string   `json:"state"`
	Health     string   `json:"health"`
	Message    string   `json:"message,omitempty"`
	Actions    []string `json:"actions"`
	Model      *string  `json:"model,omitempty"`
}

func (h *Hub) capabilities(w http.ResponseWriter, r *http.Request, id Identity) error {
	// AI readiness is reported for the client's answer language.
	lang, err := requestLanguage(r.URL.Query().Get("language"), r)
	if err != nil {
		return err
	}
	cfg := h.store.Read()
	modules := map[string]any{"media": moduleCapability{Configured: true, Enabled: true, Allowed: true, State: "ready", Health: "ok", Actions: []string{"browse", "play"}},
		// Shared content access works without any model: search, open, jobs.
		"content": moduleCapability{Configured: true, Enabled: true, Allowed: true, State: "ready", Health: "ok", Actions: []string{"search", "open", "jobs"}}}
	actions := map[string][]string{ModuleAI: {"chat", "sources", "favorite"}, ModulePhotos: {"view", "search", "albums", "upload"}, ModuleDocuments: {"view", "search", "upload"}}
	for _, name := range moduleIDs {
		m := cfg.Modules[name]
		c := moduleCapability{Configured: h.configured(name, m), Enabled: m.Enabled, Allowed: m.Grants[id.UserID], Actions: []string{}}
		health, message := h.health.get(name)
		message = lang.say(message)
		c.Health = health
		_, err := h.allowed(id, name)
		var api *APIError
		switch {
		case err == nil && health == "ok":
			c.State, c.Actions = "ready", actions[name]
		case err == nil:
			c.State, c.Message = "unavailable", message
		case errors.As(err, &api):
			c.State = api.Code
			if api.Code == "forbidden" {
				c.State = "not_allowed"
			}
		}
		if name == ModuleAI && c.State == "ready" {
			model := m.Model
			c.Model = &model
			if _, err := h.ai.qualify(assistantTask, lang); err != nil {
				c.State, c.Message, c.Actions = "qualification_required", lang.say(err.(*APIError).Message), []string{"sources"}
			} else if _, err := h.ai.qualify("media.favorite", lang); err != nil {
				c.Actions = []string{"chat", "sources"}
			}
		}
		modules[name] = c
	}
	return writeOK(w, map[string]any{"api": 1, "version": Version, "user": map[string]string{"id": id.UserID, "name": id.Name}, "modules": modules,
		"language": lang.Code})
}

func writeOK(w http.ResponseWriter, v any) error {
	writeJSON(w, 200, v)
	return nil
}

// healthCache keeps the last observed module health for capability queries.
type healthCache struct {
	mu    sync.Mutex
	state map[string][2]string
}

func newHealthCache() *healthCache { return &healthCache{state: map[string][2]string{}} }

func (c *healthCache) get(module string) (string, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.state[module]
	if !ok {
		return "unknown", ""
	}
	return s[0], s[1]
}

func (c *healthCache) set(module, state, message string) {
	c.mu.Lock()
	c.state[module] = [2]string{state, message}
	c.mu.Unlock()
}

func (h *Hub) refreshHealth(ctx context.Context) {
	cfg := h.store.Read()
	check := func(module string, fn func(context.Context, *ModuleConfig) error) {
		m := cfg.Modules[module]
		if !h.configured(module, m) {
			h.health.set(module, "unknown", "")
			return
		}
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := fn(c, m); err != nil {
			h.health.set(module, "unavailable", unavailableMessage(module))
			return
		}
		h.health.set(module, "ok", "")
	}
	check(ModulePhotos, h.photos.ping)
	check(ModuleDocuments, h.docs.ping)
	check(ModuleAI, h.ai.ping)
}

func unavailableMessage(module string) string {
	switch module {
	case ModulePhotos:
		return "Die Fotobibliothek ist gerade nicht erreichbar. Filme und Serien bleiben nutzbar."
	case ModuleDocuments:
		return "Das Dokumentenarchiv ist gerade nicht erreichbar. Filme und Serien bleiben nutzbar."
	default:
		return "Die lokale KI ist gerade nicht bereit. Suche und Wiedergabe bleiben nutzbar."
	}
}

// streamRegistry lets rights changes end long-running responses at once.
type streamRegistry struct {
	mu      sync.Mutex
	streams map[string]map[*registered]bool
}

type registered struct {
	module string
	cancel context.CancelFunc
}

func newStreamRegistry() *streamRegistry {
	return &streamRegistry{streams: map[string]map[*registered]bool{}}
}

func (s *streamRegistry) add(user, module string, cancel context.CancelFunc) func() {
	r := &registered{module: module, cancel: cancel}
	s.mu.Lock()
	if s.streams[user] == nil {
		s.streams[user] = map[*registered]bool{}
	}
	s.streams[user][r] = true
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.streams[user], r)
		s.mu.Unlock()
	}
}

// end cancels the user's streams of one module, or all if module is empty.
func (s *streamRegistry) end(user, module string) {
	s.mu.Lock()
	var cancel []context.CancelFunc
	for r := range s.streams[user] {
		if module == "" || r.module == module {
			cancel = append(cancel, r.cancel)
		}
	}
	s.mu.Unlock()
	for _, c := range cancel {
		c()
	}
}

func (s *streamRegistry) endModule(module string) {
	s.mu.Lock()
	users := []string{}
	for u := range s.streams {
		users = append(users, u)
	}
	s.mu.Unlock()
	for _, u := range users {
		s.end(u, module)
	}
}

// guard ends ctx when the session is revoked or the module grant disappears.
func (h *Hub) guard(ctx context.Context, id Identity, module string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	remove := h.streams.add(id.UserID, module, cancel)
	go func() {
		t := time.NewTicker(4 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			check, stop := context.WithTimeout(ctx, 3*time.Second)
			_, err := h.jf.identify(check, id.token, 3*time.Second)
			stop()
			if err == nil {
				_, err = h.allowed(id, module)
			}
			if err != nil {
				cancel()
				return
			}
		}
	}()
	return ctx, func() { remove(); cancel() }
}

func trimSlash(s string) string { return strings.TrimRight(s, "/") }
