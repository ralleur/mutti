// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePackage(t *testing.T, dir string) {
	t.Helper()
	for rel, content := range map[string]string{"server/jellyfin": "server", "ai-engine/libggml.0.19.dylib": "lib", "web/index.html": "<html>"} {
		_ = os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0755)
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.Symlink("libggml.0.19.dylib", filepath.Join(dir, "ai-engine", "libggml.dylib"))
}

func signPackage(t *testing.T, dir, keyID string, private ed25519.PrivateKey) {
	t.Helper()
	raw, _ := os.ReadFile(filepath.Join(dir, componentsFile))
	b, _ := json.Marshal(componentSignature{KeyID: keyID, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, raw))})
	_ = os.WriteFile(filepath.Join(dir, signatureFile), b, 0644)
}

func TestComponentListDetectsEveryChange(t *testing.T) {
	dir := t.TempDir()
	writePackage(t, dir)
	digest, err := WriteComponents(dir)
	if err != nil {
		t.Fatal(err)
	}
	check, err := verifyComponents(dir, nil)
	if err != nil || check.Digest != digest || check.Signed {
		t.Fatalf("intact package: %+v %v", check, err)
	}
	for name, change := range map[string]func(){
		"changed file": func() { _ = os.WriteFile(filepath.Join(dir, "web/index.html"), []byte("<evil>"), 0644) },
		"added file":   func() { _ = os.WriteFile(filepath.Join(dir, "server/extra.dll"), []byte("x"), 0644) },
		"removed file": func() { _ = os.Remove(filepath.Join(dir, "server/jellyfin")) },
		"changed link": func() {
			_ = os.Remove(filepath.Join(dir, "ai-engine/libggml.dylib"))
			_ = os.Symlink("web/index.html", filepath.Join(dir, "ai-engine/libggml.dylib"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir = t.TempDir()
			writePackage(t, dir)
			_, _ = WriteComponents(dir)
			change()
			if _, err := verifyComponents(dir, nil); err == nil || errors.Is(err, errNoComponentList) {
				t.Fatalf("%s not detected: %v", name, err)
			}
		})
	}
	outside := t.TempDir()
	_ = os.Symlink("/etc/hosts", filepath.Join(outside, "hosts"))
	if _, err := WriteComponents(outside); err == nil {
		t.Fatal("link out of the package listed")
	}
	if _, err := verifyComponents(t.TempDir(), nil); !errors.Is(err, errNoComponentList) {
		t.Fatalf("missing list: %v", err)
	}
}

func TestComponentSignature(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	trusted := map[string]string{"release-test": base64.StdEncoding.EncodeToString(public)}
	dir := t.TempDir()
	writePackage(t, dir)
	_, _ = WriteComponents(dir)
	signPackage(t, dir, "release-test", private)
	if check, err := verifyComponents(dir, trusted); err != nil || !check.Signed || check.KeyID != "release-test" {
		t.Fatalf("signed package: %+v %v", check, err)
	}
	signPackage(t, dir, "other-key", private)
	if _, err := verifyComponents(dir, trusted); err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("unknown key: %v", err)
	}
	_, foreign, _ := ed25519.GenerateKey(rand.Reader)
	signPackage(t, dir, "release-test", foreign)
	if _, err := verifyComponents(dir, trusted); err == nil {
		t.Fatal("foreign signature accepted")
	}
	// A rebuilt list drops the old signature.
	signPackage(t, dir, "release-test", private)
	_, _ = WriteComponents(dir)
	if check, err := verifyComponents(dir, trusted); err != nil || check.Signed {
		t.Fatalf("stale signature kept: %+v %v", check, err)
	}
}

func TestDamagedPackageNeverTouchesData(t *testing.T) {
	f := newUpdateFixture(t)
	writePackage(t, f.resources)
	if _, err := WriteComponents(f.resources); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(f.resources, "web", "index.html"), []byte("<changed>"), 0644)
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) || !strings.Contains(f.m.State().Update.Message, "beschädigt") {
		t.Fatalf("damaged fresh install: %v", err)
	}
	if _, err := os.Stat(f.m.dataVersionPath()); !os.IsNotExist(err) {
		t.Fatal("data version written by a damaged package")
	}
	// Intact list: installed and recorded with its digest.
	digest, _ := WriteComponents(f.resources)
	if err := f.m.prepareStart(); err != nil || f.m.State().Update != nil {
		t.Fatalf("intact package: %v, stale state %+v", err, f.m.State().Update)
	}
	var stored dataVersion
	b, _ := os.ReadFile(f.m.dataVersionPath())
	_ = json.Unmarshal(b, &stored)
	if stored.Components != digest || stored.Signed {
		t.Fatalf("recorded %+v", stored)
	}
	// A rebuild of the same commit with other files counts as a new build;
	// damaged, it is blocked before any snapshot.
	f.populate()
	_ = os.WriteFile(filepath.Join(f.resources, "server", "jellyfin"), []byte("server-rebuilt"), 0644)
	_, _ = WriteComponents(f.resources)
	_ = os.WriteFile(filepath.Join(f.resources, "server", "extra"), []byte("x"), 0644)
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatalf("damaged new build: %v", err)
	}
	if entries, _ := os.ReadDir(f.m.snapshotDir()); len(entries) != 0 {
		t.Fatal("snapshot taken for a damaged package")
	}
	_ = os.Remove(filepath.Join(f.resources, "server", "extra"))
	if err := f.m.prepareStart(); err != nil || f.m.State().Update == nil || f.m.State().Update.Snapshot == "" {
		t.Fatalf("rebuilt package not treated as an update: %v %+v", err, f.m.State().Update)
	}
}

func TestReleaseChannelNeedsASignedComponentList(t *testing.T) {
	f := newUpdateFixture(t)
	prov, _ := json.Marshal(map[string]any{"channel": "release", "server": map[string]any{"commit": "commit-a", "dirty": false}})
	_ = os.WriteFile(filepath.Join(f.resources, "build-provenance.json"), prov, 0600)
	writePackage(t, f.resources)
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatalf("release without component list: %v", err)
	}
	_, _ = WriteComponents(f.resources)
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) || !strings.Contains(f.m.State().Update.Message, "nicht mit dem Release-Schlüssel") {
		t.Fatalf("unsigned release: %v", err)
	}
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	trustedComponentKeys["release-test"] = base64.StdEncoding.EncodeToString(public)
	t.Cleanup(func() { delete(trustedComponentKeys, "release-test") })
	signPackage(t, f.resources, "release-test", private)
	if err := f.m.prepareStart(); err != nil {
		t.Fatalf("signed release: %v", err)
	}
}

func TestPackageIsCheckedOnEveryStart(t *testing.T) {
	f := newUpdateFixture(t)
	writePackage(t, f.resources)
	_, _ = WriteComponents(f.resources)
	f.populate()
	if err := f.m.prepareStart(); err != nil {
		t.Fatal(err)
	}
	f.m.verifyUpdate("12.1.0", true)
	if err := f.m.prepareStart(); err != nil {
		t.Fatalf("unchanged package: %v", err)
	}
	_ = os.WriteFile(filepath.Join(f.resources, "ai-engine", "libggml.0.19.dylib"), []byte("swapped"), 0644)
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatalf("package changed after installation: %v", err)
	}
}
