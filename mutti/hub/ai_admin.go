// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type catalogEntry struct {
	CatalogModel
	Installed bool `json:"installed"`
	Suitable  bool `json:"suitable"`
}

func (a *AI) adminSnapshot(ctx context.Context) map[string]any {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	memory := systemMemoryGB()
	status := a.engine.status()
	installed := map[string]string{}
	if status.State == "ready" || status.Mode == "external" {
		if models, err := a.engine.installed(ctx); err == nil {
			for _, m := range models {
				installed[m.Name] = m.Digest
			}
		}
	}
	entries := []catalogEntry{}
	for _, m := range catalog {
		entries = append(entries, catalogEntry{CatalogModel: m, Installed: installed[m.ID] == m.Digest, Suitable: memory == 0 || memory >= m.MinMemoryGB})
	}
	adoptable := false
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			_, err = os.Stat(filepath.Join(home, ".ollama", "models", "manifests"))
			adoptable = err == nil
		}
	}
	return map[string]any{"engine": status, "catalog": entries, "memoryGB": memory, "promptVersion": PromptVersion, "adoptAvailable": adoptable,
		"qualification": a.qualificationSnapshot()}
}

var qualificationTasks = []string{assistantTask, "media.search", "media.read", "media.favorite", "documents.read", "photos.search"}

// qualificationSnapshot shows the owner what this deployment measured and
// which tasks the shipped evidence grants per answer language. It cannot
// change either.
func (a *AI) qualificationSnapshot() map[string]any {
	measured := qualificationBinding{}
	if a.qualification.runtime != nil {
		measured = a.qualification.runtime()
	}
	granted := map[string]map[string]bool{}
	for code, lang := range languagePacks {
		granted[code] = map[string]bool{}
		for _, task := range qualificationTasks {
			_, err := a.qualify(task, lang)
			granted[code][task] = err == nil
		}
	}
	records := []map[string]any{}
	for _, r := range a.qualification.records {
		records = append(records, map[string]any{"id": r.ID, "task": r.Task, "language": r.Binding.Language, "status": r.Status, "expires": r.Expires})
	}
	return map[string]any{"attestation": map[string]any{"engineDigest": measured.EngineDigest, "hardwareProfile": measured.HardwareProfile,
		"osBuild": measured.OSBuild, "adapterDigest": measured.AdapterDigest}, "granted": granted, "records": records, "loadError": a.qualificationLoad}
}

func (a *AI) adminEngine(w http.ResponseWriter, r *http.Request, id Identity) error {
	var req Engine
	if err := decodeJSON(w, r, 4096, &req); err != nil {
		return err
	}
	switch req.Mode {
	case "managed":
		req.URL = ""
	case "external":
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		normalized, err := ValidateServiceURL(ctx, req.URL)
		cancel()
		if err != nil {
			return apiErr(400, "invalid_url", err.Error())
		}
		req.URL = normalized
		var version struct {
			Version string `json:"version"`
		}
		ctx, cancel = context.WithTimeout(r.Context(), 8*time.Second)
		err = serviceCall(ctx, a.engine.client, "GET", req.URL+"/api/version", nil, nil, &version)
		cancel()
		if err != nil || version.Version == "" {
			return apiErr(409, "unreachable", "Unter dieser Adresse antwortet keine lokale KI-Engine.")
		}
	default:
		return errInvalid
	}
	if err := a.hub.store.Update(func(c *Config) error {
		c.Modules[ModuleAI].Engine = &req
		return nil
	}); err != nil {
		return err
	}
	a.engine.configure(&req)
	a.hub.streams.endModule(ModuleAI)
	return a.hub.adminState(w, r, id)
}

func (a *AI) adminModels(w http.ResponseWriter, r *http.Request, id Identity) error {
	var req struct {
		Model string `json:"model"`
		// Source optionally names another local engine store to adopt from;
		// every layer is verified against the pinned digest before use.
		Source string `json:"source,omitempty"`
	}
	if err := decodeJSON(w, r, 4096, &req); err != nil {
		return err
	}
	model, ok := catalogModel(req.Model)
	if !ok {
		return apiErr(400, "unknown_model", "Dieses Modell gehört nicht zum geprüften Katalog.")
	}
	memory := systemMemoryGB()
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	switch r.PathValue("action") {
	case "pull":
		if memory > 0 && memory < model.MinMemoryGB {
			return apiErr(409, "unsuitable", "Für dieses Modell hat der Server zu wenig Arbeitsspeicher.")
		}
		if err := a.engine.pull(model); err != nil {
			return err
		}
	case "adopt":
		home, err := os.UserHomeDir()
		if runtime.GOOS != "darwin" || err != nil {
			return apiErr(409, "unsupported", "Die Übernahme ist nur aus einer lokalen Mac-Installation möglich.")
		}
		source := filepath.Join(home, ".ollama", "models")
		if req.Source != "" {
			if !filepath.IsAbs(req.Source) || filepath.Clean(req.Source) != req.Source {
				return errInvalid
			}
			if info, err := os.Stat(filepath.Join(req.Source, "manifests")); err != nil || !info.IsDir() {
				return apiErr(404, "not_found", "Unter diesem Pfad liegt kein lokaler Modellspeicher.")
			}
			source = req.Source
		}
		if err = a.engine.adopt(model, source); err != nil {
			return err
		}
	case "select":
		if err := a.engine.ensure(ctx); err != nil {
			return err
		}
		if !a.engine.verified(ctx, model) {
			return apiErr(409, "not_installed", "Bitte das Modell zuerst laden.")
		}
		if err := a.hub.store.Update(func(c *Config) error {
			c.Modules[ModuleAI].Model = model.ID
			return nil
		}); err != nil {
			return err
		}
	case "remove":
		if err := a.engine.ensure(ctx); err != nil {
			return err
		}
		if err := a.hub.store.Update(func(c *Config) error {
			if c.Modules[ModuleAI].Model == model.ID {
				c.Modules[ModuleAI].Model = ""
				c.Modules[ModuleAI].Enabled = false
			}
			return nil
		}); err != nil {
			return err
		}
		if err := a.engine.remove(ctx, model.ID); err != nil {
			return err
		}
	default:
		return errNotFound
	}
	a.hub.refreshHealth(r.Context())
	return a.hub.adminState(w, r, id)
}
