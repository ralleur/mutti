// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

const assistantTask = "content.assist"

// qualificationBinding describes an exact deployment, not a model family or a
// RAM recommendation. Empty runtime attestations must never match a grant.
type qualificationBinding struct {
	ModelDigest, EngineDigest, HardwareProfile, OSBuild string
	ContractDigest, AdapterDigest                       string
	ContextTokens                                       int
	Temperature                                         float64
	Thinking                                            string
}

type qualificationRecord struct {
	ID, Task, Status, EvidenceDigest, SuiteDigest string
	Binding                                       qualificationBinding
	Expires                                       time.Time
}

// Immutable after construction. Neither hub.json, an HTTP request nor an
// environment variable can install grants or claim a runtime attestation.
// Production intentionally starts empty: no deployment has passed the product
// gates. A trusted runtime attestor and reviewed evidence loading are required
// before promoting the first deployment (see contracts.md).
type qualificationPolicy struct {
	runtime func() qualificationBinding
	records []qualificationRecord
}

func (b qualificationBinding) complete() bool {
	return b.ModelDigest != "" && b.EngineDigest != "" && b.HardwareProfile != "" &&
		b.OSBuild != "" && b.ContractDigest != "" && b.AdapterDigest != "" &&
		b.ContextTokens > 0 && (b.Thinking == "off" || b.Thinking == "on")
}

func qualificationError() error {
	return apiErr(409, "qualification_required", "Diese KI-Aufgabe ist für die gewählte Konfiguration noch nicht freigegeben.")
}

func (p qualificationPolicy) evaluate(task string, binding qualificationBinding, now time.Time) (string, error) {
	if task == "" || !binding.complete() {
		return "", qualificationError()
	}
	for _, record := range p.records {
		if record.Task == task && record.Status == "passed" && record.ID != "" &&
			record.EvidenceDigest != "" && record.SuiteDigest != "" && now.Before(record.Expires) &&
			record.Binding == binding {
			encoded, _ := json.Marshal(record.Binding)
			digest := sha256.Sum256(encoded)
			return record.ID + ":" + hex.EncodeToString(digest[:]), nil
		}
	}
	return "", qualificationError()
}

func assistantContractDigest() string {
	defs := append(append(movieTools(), documentTools()...), photoTools()...)
	// Freeze variable prompt inputs so text/schema edits invalidate evidence.
	b, _ := json.Marshal([]any{PromptVersion, SystemPrompt("qualification", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), defs), defs})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (a *AI) qualify(task string) (string, error) {
	model, ok := catalogModel(a.hub.store.Read().Modules[ModuleAI].Model)
	if !ok || a.qualification.runtime == nil {
		return "", qualificationError()
	}
	binding := a.qualification.runtime()
	binding.ModelDigest = model.Digest
	binding.ContextTokens = model.ContextTokens
	binding.Temperature = model.Temperature
	binding.Thinking = "off"
	binding.ContractDigest = assistantContractDigest()
	return a.qualification.evaluate(task, binding, time.Now())
}

func toolTask(name string) string {
	switch name {
	case "search_movies":
		return "media.search"
	case "get_movie":
		return "media.read"
	case "propose_favorite":
		return "media.favorite"
	case "search_documents", "read_document":
		return "documents.read"
	case "search_photos":
		return "photos.search"
	default:
		return "" // Unknown tools are denied, never covered by an existing grant.
	}
}
