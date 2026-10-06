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
	ModelDigest     string  `json:"modelDigest"`
	EngineDigest    string  `json:"engineDigest"`
	HardwareProfile string  `json:"hardwareProfile"`
	OSBuild         string  `json:"osBuild"`
	ContractDigest  string  `json:"contractDigest"`
	AdapterDigest   string  `json:"adapterDigest"`
	ContextTokens   int     `json:"contextTokens"`
	Temperature     float64 `json:"temperature"`
	Thinking        string  `json:"thinking"`
	// Language is the answer language; quality is measured per language.
	Language string `json:"language"`
}

type qualificationRecord struct {
	ID             string               `json:"id"`
	Task           string               `json:"task"`
	Status         string               `json:"status"`
	EvidenceDigest string               `json:"evidenceDigest"`
	SuiteDigest    string               `json:"suiteDigest"`
	Binding        qualificationBinding `json:"binding"`
	Expires        time.Time            `json:"expires"`
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
		b.ContextTokens > 0 && (b.Thinking == "off" || b.Thinking == "on") && b.Language != ""
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

func assistantContractDigest(lang *languagePack) string {
	defs := append(append(movieTools(), documentTools()...), photoTools()...)
	// Freeze variable prompt inputs so text/schema edits invalidate evidence.
	b, _ := json.Marshal([]any{PromptVersion, lang.Code, SystemPrompt("qualification", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), defs, lang), defs})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (a *AI) qualify(task string, lang *languagePack) (string, error) {
	if lang == nil {
		return "", qualificationError()
	}
	model, ok := catalogModel(a.hub.store.Read().Modules[ModuleAI].Model)
	if !ok || a.qualification.runtime == nil {
		return "", qualificationError()
	}
	binding := a.qualification.runtime()
	binding.ModelDigest = model.Digest
	binding.ContextTokens = model.ContextTokens
	binding.Temperature = model.Temperature
	binding.Thinking = "off"
	binding.ContractDigest = assistantContractDigest(lang)
	binding.Language = lang.Code
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
