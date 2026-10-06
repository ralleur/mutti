// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
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
	if a.binding().EngineDigest != digest {
		t.Fatal("managed confined engine not attested")
	}
	a.sandbox = false
	if a.binding().EngineDigest != "" {
		t.Fatal("unconfined engine attested")
	}
	a.sandbox = true
	e.configure(&Engine{Mode: "external", URL: "http://127.0.0.1:11434"})
	if a.binding().EngineDigest != "" {
		t.Fatal("external engine attested")
	}
	e.configure(&Engine{Mode: "managed"})
	// After a new engine start, swapped files are noticed by the re-check.
	e.mu.Lock()
	e.starts++
	e.mu.Unlock()
	_ = os.WriteFile(binary, []byte("swapped"), 0700)
	a.recheck()
	if got := a.binding().EngineDigest; got == digest || got == "" {
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
	e.hub.ai.qualification = qualificationPolicy{runtime: func() qualificationBinding { return measured }, records: records}
	if _, err := e.hub.ai.qualify(assistantTask, languagePacks["de"]); err != nil {
		t.Fatalf("measured deployment denied: %v", err)
	}
	if _, err := e.hub.ai.qualify(assistantTask, languagePacks["en"]); err == nil {
		t.Fatal("other language granted")
	}
	if _, err := e.hub.ai.qualify("documents.read", languagePacks["de"]); err == nil {
		t.Fatal("unmeasured task granted")
	}
	changed := measured
	changed.OSBuild = "macOS update"
	e.hub.ai.qualification.runtime = func() qualificationBinding { return changed }
	if _, err := e.hub.ai.qualify(assistantTask, languagePacks["de"]); err == nil {
		t.Fatal("changed OS still granted")
	}
}
