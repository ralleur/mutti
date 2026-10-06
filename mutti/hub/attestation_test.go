// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDirectoryDigestCoversContentModesAndLinks(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "lib"), 0700)
	_ = os.WriteFile(filepath.Join(dir, "ollama"), []byte("engine"), 0700)
	_ = os.WriteFile(filepath.Join(dir, "lib", "mlx.metallib"), []byte("kernels"), 0600)
	_ = os.Symlink("lib/mlx.metallib", filepath.Join(dir, "current"))
	first, err := directoryDigest(dir)
	if err != nil || len(first) != 64 {
		t.Fatal(err)
	}
	if again, _ := directoryDigest(dir); again != first {
		t.Fatal("digest not stable")
	}
	for _, change := range []func(){
		func() { _ = os.WriteFile(filepath.Join(dir, "lib", "mlx.metallib"), []byte("kernelz"), 0600) },
		func() { _ = os.Chmod(filepath.Join(dir, "lib", "mlx.metallib"), 0700) },
		func() {
			_ = os.Remove(filepath.Join(dir, "current"))
			_ = os.Symlink("ollama", filepath.Join(dir, "current"))
		},
		func() { _ = os.WriteFile(filepath.Join(dir, "lib", "extra.dylib"), nil, 0600) },
	} {
		before, _ := directoryDigest(dir)
		change()
		if after, _ := directoryDigest(dir); after == before {
			t.Fatal("change not reflected")
		}
	}
	if _, err := directoryDigest(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing directory measured")
	}
}

func signedTestFile(t *testing.T, doc recordsPayload) (string, map[string]string, ed25519.PrivateKey) {
	t.Helper()
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	keys := map[string]string{"test-key": base64.StdEncoding.EncodeToString(public)}
	b, err := SignRecords(doc, "test-key", private)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "records.json")
	_ = os.WriteFile(path, b, 0600)
	return path, keys, private
}

func TestSignedRecordsAreVerifiedAndRevocable(t *testing.T) {
	record := qualificationRecord{ID: "q1", Task: assistantTask, Status: "passed", EvidenceDigest: "e", SuiteDigest: "s",
		Binding: qualificationBinding{Language: "de"}, Expires: time.Now().Add(time.Hour)}
	doc := recordsPayload{Version: 1, Issued: time.Now().UTC(), Records: []qualificationRecord{record, {ID: "q2", Task: "media.search"}}, Revoked: []string{"q2"}}
	path, keys, _ := signedTestFile(t, doc)
	records, err := loadSignedRecords(path, keys)
	if err != nil || len(records) != 1 || records[0].ID != "q1" || records[0].Binding.Language != "de" {
		t.Fatalf("%v %v", records, err)
	}
	if _, err := loadSignedRecords(path, map[string]string{"other": keys["test-key"]}); err == nil {
		t.Fatal("unknown key id accepted")
	}
	otherPublic, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := loadSignedRecords(path, map[string]string{"test-key": base64.StdEncoding.EncodeToString(otherPublic)}); err == nil {
		t.Fatal("wrong key accepted")
	}
	if _, err := loadSignedRecords(path, map[string]string{"test-key": ""}); err == nil {
		t.Fatal("empty trusted key accepted")
	}
	// Tampering with the payload breaks the signature.
	var file signedRecords
	raw, _ := os.ReadFile(path)
	_ = json.Unmarshal(raw, &file)
	payload, _ := base64.StdEncoding.DecodeString(file.Payload)
	var changed recordsPayload
	_ = json.Unmarshal(payload, &changed)
	changed.Revoked = nil
	b, _ := json.Marshal(changed)
	file.Payload = base64.StdEncoding.EncodeToString(b)
	b, _ = json.Marshal(file)
	_ = os.WriteFile(path, b, 0600)
	if _, err := loadSignedRecords(path, keys); err == nil {
		t.Fatal("tampered payload accepted")
	}
}

func TestAttestationDeniesExternalOrUnconfinedEngine(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "ollama")
	_ = os.WriteFile(binary, []byte("engine"), 0700)
	e := newEngine(t.TempDir(), binary, true)
	digest, _ := directoryDigest(dir)
	a := &attestation{engine: e, dir: dir, digest: digest, sandbox: true, hardware: "hw", osBuild: "os", adapter: "hub"}
	if a.bindingFor(CatalogModel{}).EngineDigest != digest {
		t.Fatal("managed confined engine not attested")
	}
	a.sandbox = false
	if a.bindingFor(CatalogModel{}).EngineDigest != "" {
		t.Fatal("unconfined engine attested")
	}
	a.sandbox = true
	e.configure(&Engine{Mode: "external", URL: "http://127.0.0.1:11434"})
	if a.bindingFor(CatalogModel{}).EngineDigest != "" {
		t.Fatal("external engine attested")
	}
	e.configure(&Engine{Mode: "managed"})
	// After a new engine start, swapped files are noticed by the re-check.
	e.mu.Lock()
	e.starts++
	e.mu.Unlock()
	_ = os.WriteFile(binary, []byte("swapped"), 0700)
	a.recheck()
	if got := a.bindingFor(CatalogModel{}).EngineDigest; got == digest || got == "" {
		t.Fatalf("recheck %q", got)
	}
}

func TestProductionPolicyWithoutEvidenceDeniesEverything(t *testing.T) {
	e := newTestEnvWithQualification(t, "", false)
	e.configureAI()
	if e.hub.ai.qualificationLoad == "" {
		t.Fatal("missing evidence not reported")
	}
	for code, lang := range languagePacks {
		for _, task := range qualificationTasks {
			if _, err := e.hub.ai.qualify(task, lang); err == nil {
				t.Fatalf("%s %s granted without evidence", code, task)
			}
		}
	}
	snap := e.hub.ai.qualificationSnapshot()
	if snap["granted"].(map[string]map[string]bool)["de"][assistantTask] {
		t.Fatal("snapshot claims a grant")
	}
}

func TestSignedEvidenceGrantsOnlyTheMeasuredDeployment(t *testing.T) {
	measured := qualificationBinding{EngineDigest: "engine", HardwareProfile: "hw", OSBuild: "os", AdapterDigest: "hub"}
	full := measured
	full.ModelDigest, full.ContextTokens, full.Temperature, full.Thinking = catalog[0].Digest, catalog[0].ContextTokens, catalog[0].Temperature, "off"
	full.Language, full.ContractDigest = "de", assistantContractDigest(languagePacks["de"])
	doc := recordsPayload{Version: 1, Issued: time.Now().UTC(), Records: []qualificationRecord{{ID: "q-de-assist", Task: assistantTask, Status: "passed",
		EvidenceDigest: "e", SuiteDigest: "s", Binding: full, Expires: time.Now().Add(time.Hour)}}}
	path, keys, _ := signedTestFile(t, doc)
	records, err := loadSignedRecords(path, keys)
	if err != nil {
		t.Fatal(err)
	}
	e := newTestEnvWithQualification(t, "", false)
	e.configureAI()
	withModel := measured
	withModel.ModelDigest = catalog[0].Digest
	e.hub.ai.qualification = qualificationPolicy{runtime: func(CatalogModel) qualificationBinding { return withModel }, records: records}
	if _, err := e.hub.ai.qualify(assistantTask, languagePacks["de"]); err != nil {
		t.Fatalf("measured deployment denied: %v", err)
	}
	if _, err := e.hub.ai.qualify(assistantTask, languagePacks["en"]); err == nil {
		t.Fatal("other language granted")
	}
	if _, err := e.hub.ai.qualify("documents.read", languagePacks["de"]); err == nil {
		t.Fatal("unmeasured task granted")
	}
	changed := withModel
	changed.OSBuild = "macOS update"
	e.hub.ai.qualification.runtime = func(CatalogModel) qualificationBinding { return changed }
	if _, err := e.hub.ai.qualify(assistantTask, languagePacks["de"]); err == nil {
		t.Fatal("changed OS still granted")
	}
}

func writeTestModel(t *testing.T, models string, blobs map[string][]byte) CatalogModel {
	t.Helper()
	layers := []map[string]string{}
	config := ""
	for name, content := range blobs {
		sum := sha256.Sum256(content)
		digest := "sha256:" + hex.EncodeToString(sum[:])
		_ = os.MkdirAll(filepath.Join(models, "blobs"), 0700)
		_ = os.WriteFile(filepath.Join(models, "blobs", "sha256-"+hex.EncodeToString(sum[:])), content, 0600)
		if name == "config" {
			config = digest
		} else {
			layers = append(layers, map[string]string{"digest": digest})
		}
	}
	manifest, _ := json.Marshal(map[string]any{"config": map[string]string{"digest": config}, "layers": layers})
	path := filepath.Join(models, "manifests", "registry.ollama.ai", "library", "test", "model")
	_ = os.MkdirAll(filepath.Dir(path), 0700)
	_ = os.WriteFile(path, manifest, 0600)
	sum := sha256.Sum256(manifest)
	return CatalogModel{ID: "test:model", Digest: hex.EncodeToString(sum[:])}
}

func TestModelFilesAreVerifiedNotTakenFromTheEngine(t *testing.T) {
	dir := t.TempDir()
	e := newEngine(dir, "", true)
	model := writeTestModel(t, e.modelsDir(), map[string][]byte{"config": []byte("{}"), "weights": []byte("weights-v1")})
	a := &attestation{engine: e, models: map[string]string{}, verifying: map[string]bool{}}
	if a.bindingFor(model).ModelDigest != "" {
		t.Fatal("unverified model attested")
	}
	if err := a.verifyModel(model); err != nil {
		t.Fatal(err)
	}
	if a.bindingFor(model).ModelDigest != model.Digest {
		t.Fatal("verified model not attested")
	}
	// Replacing a blob's content (same name) changes its signature and fails
	// the next verification.
	files, _ := modelFiles(e.modelsDir(), model)
	time.Sleep(10 * time.Millisecond)
	_ = os.WriteFile(files[2], []byte("tampered!!"), 0600)
	if a.bindingFor(model).ModelDigest != "" {
		t.Fatal("changed blob still attested")
	}
	if err := a.verifyModel(model); err == nil {
		t.Fatal("tampered blob verified")
	}
	// A manifest that is not the catalog's is refused.
	other := model
	other.Digest = strings.Repeat("0", 64)
	if err := a.verifyModel(other); err == nil {
		t.Fatal("foreign manifest verified")
	}
}

func TestEngineDigestIsStaleAfterAnEngineStart(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "ollama")
	_ = os.WriteFile(binary, []byte("engine"), 0700)
	e := newEngine(t.TempDir(), binary, true)
	digest, _ := directoryDigest(dir)
	a := &attestation{engine: e, dir: dir, digest: digest, sandbox: true, models: map[string]string{}, verifying: map[string]bool{}}
	e.mu.Lock()
	e.starts++
	e.mu.Unlock()
	if a.bindingFor(CatalogModel{}).EngineDigest != "" {
		t.Fatal("digest from before the start still used")
	}
	a.recheck()
	if a.bindingFor(CatalogModel{}).EngineDigest != digest {
		t.Fatal("recheck did not restore the digest")
	}
}

func TestEngineDirectoryLinksMustStayInside(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "lib.dylib"), []byte("x"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "ollama"), []byte("engine"), 0700)
	_ = os.Symlink(filepath.Join(outside, "lib.dylib"), filepath.Join(dir, "lib.dylib"))
	if _, err := directoryDigest(dir); err == nil {
		t.Fatal("link to an unmeasured file accepted")
	}
	// The directory itself may be reached through a link.
	_ = os.Remove(filepath.Join(dir, "lib.dylib"))
	link := filepath.Join(t.TempDir(), "engine")
	_ = os.Symlink(dir, link)
	direct, _ := directoryDigest(dir)
	via, err := directoryDigest(link)
	if err != nil || via != direct {
		t.Fatalf("linked directory: %v", err)
	}
}

// A run needs the managed engine the evidence was measured on and an
// explicit thinking setting; otherwise it fails before the model is asked.
func TestRunsFailClosedOnExternalEngineOrUnknownThinking(t *testing.T) {
	e := newTestEnv(t, "")
	e.configureAI()
	var conv map[string]any
	e.do("POST", "/ai/conversations", "token-a", map[string]string{}, &conv)
	cid := conv["id"].(string)
	finished := func(key string) map[string]any {
		out, code := e.ask("token-a", cid, "Hallo", key)
		if code != 202 {
			t.Fatalf("ask %d %v", code, out)
		}
		events, _ := e.events(runID(out), cid, "token-a", func(ev sseEvent) bool { return ev.Type == "done" })
		return events[len(events)-1].Data
	}
	e.hub.ai.qualification.externalEngine = false
	if done := finished("x1"); done["state"] != "failed" || done["code"] != "qualification_required" {
		t.Fatalf("external engine %v", done)
	}
	e.hub.ai.qualification.externalEngine = true
	e.engine.showFails.Store(true)
	if done := finished("x2"); done["state"] != "failed" || done["code"] != "engine" {
		t.Fatalf("unknown thinking %v", done)
	}
	if n := e.engine.chats.Load(); n != 0 {
		t.Fatalf("model asked %d times", n)
	}
	e.engine.showFails.Store(false)
	if done := finished("x3"); done["state"] != "completed" {
		t.Fatalf("recovered %v", done)
	}
}
