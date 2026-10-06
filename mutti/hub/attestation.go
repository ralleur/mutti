// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// trustedReleaseKeys verify qualification records. Only reviewed evidence
// signed with one of these keys can grant an AI task; the private key is
// kept outside the repository and the product.
var trustedReleaseKeys = map[string]string{
	"mutti-qualification-2026-10": "VaYe6SGRVGuFoxVwK4UlKH0t5FuTGUd1BFSjV4HyFjM=",
}

// attestation measures the running deployment itself. Nothing in it comes
// from configuration, the client or the engine's own claims: the engine
// digest covers every file of the bundled engine directory, the adapter
// digest is the running hub executable, hardware and OS come from the kernel.
// An external or unconfined engine has no engine digest and is never granted.
type attestation struct {
	mu        sync.Mutex
	engine    *engine
	dir       string
	digest    string // engine directory digest, "" while unknown
	starts    int    // engine starts covered by digest
	sandbox   bool
	hardware  string
	osBuild   string
	adapter   string
	computing bool
}

func newAttestation(e *engine) *attestation {
	a := &attestation{engine: e}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || e.binary == "" {
		return a // Day 1 qualifies Apple Silicon Macs only.
	}
	a.dir = filepath.Dir(e.binary)
	a.computing = true
	go func() {
		hw, osb := hardwareProfile(), osBuild()
		adapter, _ := executableDigest()
		sandbox := e.sandbox && verifySandbox()
		digest, err := directoryDigest(a.dir)
		a.mu.Lock()
		a.hardware, a.osBuild, a.adapter, a.sandbox = hw, osb, adapter, sandbox
		if err == nil {
			a.digest = digest
		}
		a.starts = e.startCount()
		a.computing = false
		a.mu.Unlock()
	}()
	return a
}

// binding returns the measured deployment. Missing parts stay empty and
// therefore never match a record.
func (a *attestation) binding() qualificationBinding {
	a.mu.Lock()
	defer a.mu.Unlock()
	b := qualificationBinding{HardwareProfile: a.hardware, OSBuild: a.osBuild, AdapterDigest: a.adapter}
	if managed, binary := a.engine.managedBinary(); managed && binary != "" && filepath.Dir(binary) == a.dir && a.sandbox {
		b.EngineDigest = a.digest
	}
	return b
}

// recheck re-measures the engine directory after the engine was started,
// so files swapped after startup are noticed before the engine answers.
func (a *attestation) recheck() {
	if a == nil || a.dir == "" {
		return
	}
	starts := a.engine.startCount()
	a.mu.Lock()
	current := a.starts == starts && !a.computing
	a.mu.Unlock()
	if current {
		return
	}
	digest, err := directoryDigest(a.dir)
	a.mu.Lock()
	if err != nil {
		digest = ""
	}
	a.digest, a.starts = digest, starts
	a.mu.Unlock()
}

// directoryDigest hashes every path, type and content below dir in order.
func directoryDigest(dir string) (string, error) {
	type entry struct{ rel, kind, sum string }
	var entries []entry
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entries = append(entries, entry{rel, "link", target})
		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				return err
			}
			sum, err := fileDigest(path)
			if err != nil {
				return err
			}
			entries = append(entries, entry{rel, fmt.Sprintf("file:%o", info.Mode().Perm()&0o111), sum})
		case d.IsDir():
		default:
			return fmt.Errorf("unexpected file type at %s", rel)
		}
		return nil
	})
	if err != nil || len(entries) == 0 {
		return "", errors.New("engine directory not measurable")
	}
	slices.SortFunc(entries, func(x, y entry) int { return strings.Compare(x.rel, y.rel) })
	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", filepath.ToSlash(e.rel), e.kind, e.sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func executableDigest() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	return fileDigest(exe)
}

func sysctl(name string) string {
	out, err := exec.Command("/usr/sbin/sysctl", "-n", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// hardwareProfile is the exact machine class measured: model identifier,
// chip, CPU and GPU core counts and memory. Every part must be known.
func hardwareProfile() string {
	gpu := ""
	if out, err := exec.Command("/usr/sbin/ioreg", "-rc", "AGXAccelerator", "-d", "1").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if _, value, ok := strings.Cut(line, `"gpu-core-count" = `); ok {
				gpu = strings.TrimSpace(value)
			}
		}
	}
	parts := []string{sysctl("hw.model"), sysctl("machdep.cpu.brand_string"), "P" + sysctl("hw.perflevel0.physicalcpu") + "+E" + sysctl("hw.perflevel1.physicalcpu"),
		"GPU" + gpu, "RAM" + sysctl("hw.memsize")}
	for _, p := range parts {
		if p == "" || strings.HasSuffix(p, "+E") || p == "GPU" || p == "RAM" || strings.HasPrefix(p, "P+") {
			return ""
		}
	}
	return strings.Join(parts, "|")
}

func osBuild() string {
	version, build := sysctl("kern.osproductversion"), sysctl("kern.osversion")
	if version == "" || build == "" {
		return ""
	}
	return "macOS " + version + " (" + build + ")"
}

// signedRecords is the reviewed evidence file shipped next to the hub.
type signedRecords struct {
	Version   int    `json:"version"`
	KeyID     string `json:"keyId"`
	Payload   string `json:"payload"`   // base64 of a recordsPayload document
	Signature string `json:"signature"` // base64 Ed25519 signature of the payload bytes
}

type recordsPayload struct {
	Version int                   `json:"version"`
	Issued  time.Time             `json:"issued"`
	Records []qualificationRecord `json:"records"`
	Revoked []string              `json:"revoked"`
}

// loadSignedRecords verifies and returns the records of a signed file.
// Revoked IDs are removed; any defect yields no records at all.
func loadSignedRecords(path string, keys map[string]string) ([]qualificationRecord, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file signedRecords
	if err = json.Unmarshal(raw, &file); err != nil || file.Version != 1 {
		return nil, errors.New("invalid qualification file")
	}
	encoded, ok := keys[file.KeyID]
	if !ok || encoded == "" {
		return nil, errors.New("qualification file signed with an untrusted key")
	}
	public, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(public) != ed25519.PublicKeySize {
		return nil, errors.New("invalid trusted key")
	}
	payload, err1 := base64.StdEncoding.DecodeString(file.Payload)
	signature, err2 := base64.StdEncoding.DecodeString(file.Signature)
	if err1 != nil || err2 != nil || !ed25519.Verify(ed25519.PublicKey(public), payload, signature) {
		return nil, errors.New("qualification file signature invalid")
	}
	var doc recordsPayload
	if err = json.Unmarshal(payload, &doc); err != nil || doc.Version != 1 {
		return nil, errors.New("invalid qualification payload")
	}
	out := []qualificationRecord{}
	for _, r := range doc.Records {
		if !slices.Contains(doc.Revoked, r.ID) {
			out = append(out, r)
		}
	}
	return out, nil
}

// SignRecords produces a signed qualification file (release tooling only).
func SignRecords(doc recordsPayload, keyID string, private ed25519.PrivateKey) ([]byte, error) {
	payload, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(signedRecords{Version: 1, KeyID: keyID, Payload: base64.StdEncoding.EncodeToString(payload),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, payload))}, "", "  ")
}

// qualificationFile is where a package ships its reviewed evidence.
func qualificationFile(opts Options) string {
	if opts.Qualification != "" {
		return opts.Qualification
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "qualification", "records.json")
}

// productionPolicy combines attestation with the shipped, signed records.
func productionPolicy(opts Options, e *engine) (qualificationPolicy, *attestation, error) {
	att := newAttestation(e)
	p := qualificationPolicy{runtime: att.binding}
	path := qualificationFile(opts)
	if path == "" {
		return p, att, errors.New("no qualification file location")
	}
	records, err := loadSignedRecords(path, trustedReleaseKeys)
	if err != nil {
		return p, att, err
	}
	p.records = records
	return p, att, nil
}

// SignCandidates signs the selected passed candidates of a measurement.
// Release tooling only; the product never signs.
func SignCandidates(candidatesPath, out, keyID string, private ed25519.PrivateKey, tasks, languages []string) error {
	raw, err := os.ReadFile(candidatesPath)
	if err != nil {
		return err
	}
	var doc recordsPayload
	if err = json.Unmarshal(raw, &doc); err != nil || doc.Version != 1 {
		return errors.New("invalid candidates file")
	}
	selected := []qualificationRecord{}
	for _, r := range doc.Records {
		if r.Status != "passed" || (tasks != nil && !slices.Contains(tasks, r.Task)) || (languages != nil && !slices.Contains(languages, r.Binding.Language)) {
			continue
		}
		selected = append(selected, r)
	}
	if len(selected) == 0 {
		return errors.New("nothing to sign")
	}
	doc.Records, doc.Issued = selected, time.Now().UTC()
	b, err := SignRecords(doc, keyID, private)
	if err != nil {
		return err
	}
	if _, err := os.Stat(out); err == nil {
		return errors.New("output exists")
	}
	return os.WriteFile(out, b, 0644)
}

// VerifyRecordsFile checks a file against the keys built into this hub.
func VerifyRecordsFile(path string) (string, error) {
	records, err := loadSignedRecords(path, trustedReleaseKeys)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, r := range records {
		fmt.Fprintf(&b, "%s %s %s %s expires %s\n", r.ID, r.Task, r.Binding.Language, r.Status, r.Expires.Format(time.DateOnly))
	}
	return b.String(), nil
}
