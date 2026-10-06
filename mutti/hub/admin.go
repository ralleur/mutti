// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"net/http"
	"slices"
	"time"
)

type profile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// profiles lists active, non-administrative Jellyfin users with the owner's token.
func (h *Hub) profiles(ctx context.Context, id Identity) ([]profile, error) {
	var users []struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Policy struct {
			IsAdministrator bool `json:"IsAdministrator"`
			IsDisabled      bool `json:"IsDisabled"`
		} `json:"Policy"`
	}
	if err := h.jf.call(ctx, "GET", "/Users", id.token, nil, &users); err != nil {
		return nil, err
	}
	out := []profile{}
	for _, u := range users {
		if !u.Policy.IsAdministrator && !u.Policy.IsDisabled {
			out = append(out, profile{normalizeID(u.ID), u.Name})
		}
	}
	return out, nil
}

func (h *Hub) adminState(w http.ResponseWriter, r *http.Request, id Identity) error {
	profiles, err := h.profiles(r.Context(), id)
	if err != nil {
		return err
	}
	cfg := h.store.Read()
	modules := map[string]any{}
	for _, name := range moduleIDs {
		m := cfg.Modules[name]
		links := map[string]any{}
		for user, l := range m.Links {
			links[user] = map[string]any{"account": l.Account, "created": l.Created}
		}
		grants := []string{}
		for user, ok := range m.Grants {
			if ok {
				grants = append(grants, user)
			}
		}
		slices.Sort(grants)
		health, message := h.health.get(name)
		entry := map[string]any{"configured": h.configured(name, m), "enabled": m.Enabled, "grants": grants, "links": links,
			"serviceUrl": m.ServiceURL, "health": health, "message": message}
		if name == ModuleAI {
			entry["engine"] = m.Engine
			entry["model"] = m.Model
			entry["ai"] = h.ai.adminSnapshot(r.Context())
		}
		modules[name] = entry
	}
	return writeOK(w, map[string]any{"version": Version, "profiles": profiles, "modules": modules})
}

func validModule(name string) bool { return slices.Contains(moduleIDs, name) }

func (h *Hub) adminModule(w http.ResponseWriter, r *http.Request, id Identity) error {
	name := r.PathValue("module")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !validModule(name) {
		return errNotFound
	}
	if err := decodeJSON(w, r, 4096, &req); err != nil {
		return err
	}
	err := h.store.Update(func(c *Config) error {
		m := c.Modules[name]
		if req.Enabled && !h.configured(name, m) {
			return apiErr(409, "not_configured", "Bitte die Funktion zuerst einrichten.")
		}
		m.Enabled = req.Enabled
		return nil
	})
	if err != nil {
		return err
	}
	if !req.Enabled {
		h.streams.endModule(name)
		if name == ModuleAI {
			h.ai.cancelUser("")
		}
	}
	h.refreshHealth(r.Context())
	return h.adminState(w, r, id)
}

func (h *Hub) adminGrant(w http.ResponseWriter, r *http.Request, id Identity) error {
	var req struct {
		Module  string `json:"module"`
		UserID  string `json:"userId"`
		Allowed bool   `json:"allowed"`
	}
	if err := decodeJSON(w, r, 4096, &req); err != nil || !validModule(req.Module) {
		return errInvalid
	}
	user, err := h.profileByID(r.Context(), id, req.UserID)
	if err != nil {
		return err
	}
	if err = h.store.Update(func(c *Config) error {
		if req.Allowed {
			c.Modules[req.Module].Grants[user.ID] = true
		} else {
			delete(c.Modules[req.Module].Grants, user.ID)
		}
		return nil
	}); err != nil {
		return err
	}
	if !req.Allowed {
		h.streams.end(user.ID, req.Module)
		if req.Module == ModuleAI {
			h.ai.cancelUser(user.ID)
		}
	}
	return h.adminState(w, r, id)
}

func (h *Hub) profileByID(ctx context.Context, id Identity, user string) (profile, error) {
	profiles, err := h.profiles(ctx, id)
	if err != nil {
		return profile{}, err
	}
	for _, p := range profiles {
		if p.ID == normalizeID(user) {
			return p, nil
		}
	}
	return profile{}, apiErr(400, "invalid_profile", "Bitte ein aktives Wiedergabeprofil ohne Administratorrechte wählen.")
}

func (h *Hub) adminService(w http.ResponseWriter, r *http.Request, id Identity) error {
	name := r.PathValue("module")
	if name != ModulePhotos && name != ModuleDocuments {
		return errNotFound
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(w, r, 4096, &req); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	normalized, err := ValidateServiceURL(ctx, req.URL)
	if err != nil {
		return apiErr(400, "invalid_url", err.Error())
	}
	probe := &ModuleConfig{ServiceURL: normalized}
	if name == ModulePhotos {
		err = h.photos.ping(ctx, probe)
	} else {
		err = h.docs.ping(ctx, probe)
	}
	if err != nil {
		return apiErr(409, "unreachable", "Unter dieser Adresse antwortet kein passender Dienst.")
	}
	if err = h.store.Update(func(c *Config) error {
		m := c.Modules[name]
		if m.ServiceURL != normalized {
			// Accounts belong to one service instance; never reuse keys elsewhere.
			m.Links = map[string]*Link{}
			m.Instance = randomID()
		}
		m.ServiceURL = normalized
		return nil
	}); err != nil {
		return err
	}
	h.streams.endModule(name)
	h.refreshHealth(r.Context())
	return h.adminState(w, r, id)
}

func (h *Hub) adminLink(w http.ResponseWriter, r *http.Request, id Identity) error {
	name := r.PathValue("module")
	if name != ModulePhotos && name != ModuleDocuments {
		return errNotFound
	}
	var req struct {
		UserID   string `json:"userId"`
		Account  string `json:"account"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, 8192, &req); err != nil || req.Account == "" || req.Password == "" || len(req.Account) > 200 || len(req.Password) > 512 {
		return errInvalid
	}
	user, err := h.profileByID(r.Context(), id, req.UserID)
	if err != nil {
		return err
	}
	cfg := h.store.Read().Modules[name]
	if cfg.ServiceURL == "" {
		return apiErr(409, "not_configured", "Bitte zuerst die Dienstadresse festlegen.")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var link *Link
	// The password is used once to obtain a profile-scoped credential and is
	// never stored or logged.
	if name == ModulePhotos {
		link, err = h.photos.link(ctx, cfg.ServiceURL, req.Account, req.Password, user.Name)
	} else {
		link, err = h.docs.link(ctx, cfg.ServiceURL, req.Account, req.Password)
	}
	if err != nil {
		return err
	}
	for other, existing := range cfg.Links {
		if other != user.ID && existing.Remote == link.Remote {
			return apiErr(409, "account_in_use", "Dieses Konto ist bereits einem anderen Profil zugeordnet. Jedes Profil braucht ein eigenes Konto.")
		}
	}
	if err = h.store.Update(func(c *Config) error {
		c.Modules[name].Links[user.ID] = link
		return nil
	}); err != nil {
		return err
	}
	h.streams.end(user.ID, name)
	return h.adminState(w, r, id)
}

func (h *Hub) adminUnlink(w http.ResponseWriter, r *http.Request, id Identity) error {
	name := r.PathValue("module")
	if name != ModulePhotos && name != ModuleDocuments {
		return errNotFound
	}
	var req struct {
		UserID string `json:"userId"`
	}
	if err := decodeJSON(w, r, 4096, &req); err != nil {
		return err
	}
	user := normalizeID(req.UserID)
	cfg := h.store.Read().Modules[name]
	old := cfg.Links[user]
	if old == nil {
		return errNotFound
	}
	if err := h.store.Update(func(c *Config) error {
		delete(c.Modules[name].Links, user)
		return nil
	}); err != nil {
		return err
	}
	h.streams.end(user, name)
	revoked := false
	if name == ModulePhotos {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		revoked = h.photos.revokeKey(ctx, cfg.ServiceURL, old) == nil
		cancel()
	}
	return writeOK(w, map[string]any{"unlinked": true, "remoteRevoked": revoked})
}
