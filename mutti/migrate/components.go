// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

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
	"path/filepath"
	"sort"
	"strings"
)

// Component sets (P3). The build lists every file of the package resources
// (servers, web client, module services, AI engine, tools, licenses) with its
// SHA-256 in components.json; a release additionally signs exactly these
// bytes (components.sig). Before a build touches the data for the first time
// the manager checks the whole set, so a damaged, partly copied or modified
// package never migrates data. The app shell itself is covered by the macOS
// code signature, which seals the resources as well.

const (
	componentsFile = "components.json"
	signatureFile  = "components.sig"
)

// errNoComponentList: the package has no components.json at all (a
// development build). Any other problem reading the package is a defect.
var errNoComponentList = errors.New("no component list")

// trustedComponentKeys are the release keys (key ID -> Ed25519 public key,
// base64). The private keys live outside the repository; until the owner
// creates the release key, every package is an unsigned development build.
var trustedComponentKeys = map[string]string{}

type componentManifest struct {
	Schema int               `json:"schema"`
	Files  map[string]string `json:"files"`           // path relative to Resources -> sha256
	Links  map[string]string `json:"links,omitempty"` // symlink path -> target
}

type componentSignature struct {
	KeyID     string `json:"keyId"`
	Signature string `json:"signature"` // Ed25519 over the exact bytes of components.json
}

// componentCheck is what a verified package contributes to the data version.
type componentCheck struct {
	Digest string // sha256 of components.json
	Signed bool
	KeyID  string
}

// scanComponents walks the resources without the manifest and signature.
func scanComponents(resources string) (componentManifest, error) {
	m := componentManifest{Schema: 1, Files: map[string]string{}, Links: map[string]string{}}
	root, err := filepath.EvalSymlinks(resources)
	if err != nil {
		return m, err
	}
	err = filepath.WalkDir(resources, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(resources, path)
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == componentsFile || rel == signatureFile {
			return nil
		}
		// Finder and non-APFS volumes add metadata files; they are never run.
		if name := d.Name(); name == ".DS_Store" || strings.HasPrefix(name, "._") {
			return nil
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			// A link may only point inside the package; its target is hashed there.
			inside := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(rel), target)))
			if filepath.IsAbs(target) || inside == ".." || strings.HasPrefix(inside, "../") {
				return fmt.Errorf("%s points outside the package", rel)
			}
			// Chains of links are resolved, not only read lexically.
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil || (resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator))) {
				return fmt.Errorf("%s points outside the package", rel)
			}
			m.Links[rel] = target
		case d.IsDir():
		case d.Type().IsRegular():
			sum, err := fileSHA256(path)
			if err != nil {
				return err
			}
			m.Files[rel] = sum
		default:
			return fmt.Errorf("%s is not a regular file", rel)
		}
		return nil
	})
	return m, err
}

func fileSHA256(path string) (string, error) {
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

// WriteComponents records the package's component set (build step, after
// every file is in place and before the app is code-signed).
func WriteComponents(resources string) (string, error) {
	m, err := scanComponents(resources)
	if err != nil {
		return "", err
	}
	if len(m.Links) == 0 {
		m.Links = nil
	}
	b, _ := json.MarshalIndent(m, "", " ") // map keys are sorted: deterministic
	b = append(b, '\n')
	_ = os.Remove(filepath.Join(resources, signatureFile)) // a new set needs a new signature
	if err = os.WriteFile(filepath.Join(resources, componentsFile), b, 0644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyComponentSet is VerifyComponents for tools outside this package.
func VerifyComponentSet(resources string) (digest string, signed bool, err error) {
	c, err := VerifyComponents(resources)
	return c.Digest, c.Signed, err
}

// VerifyComponents checks every file, link and the signature if present. A
// missing manifest is reported as errNoComponentList; any difference,
// missing or additional file, or a signature that does not verify is an error.
func VerifyComponents(resources string) (componentCheck, error) {
	return verifyComponents(resources, trustedComponentKeys)
}

func verifyComponents(resources string, trusted map[string]string) (componentCheck, error) {
	raw, err := os.ReadFile(filepath.Join(resources, componentsFile))
	if os.IsNotExist(err) {
		return componentCheck{}, errNoComponentList
	}
	if err != nil {
		return componentCheck{}, errors.New("the component list cannot be read")
	}
	var want componentManifest
	if err = json.Unmarshal(raw, &want); err != nil || want.Schema != 1 || len(want.Files) == 0 {
		return componentCheck{}, errors.New("the component list is unreadable")
	}
	sum := sha256.Sum256(raw)
	check := componentCheck{Digest: hex.EncodeToString(sum[:])}
	got, err := scanComponents(resources)
	if err != nil {
		// Not wrapped: a file vanishing during the scan is a defect, never
		// "no component list".
		return check, fmt.Errorf("the package cannot be read completely: %v", err)
	}
	var diffs []string
	for path, h := range want.Files {
		if got.Files[path] != h {
			diffs = append(diffs, path)
		}
	}
	for path := range got.Files {
		if _, ok := want.Files[path]; !ok {
			diffs = append(diffs, path)
		}
	}
	for path, target := range want.Links {
		if got.Links[path] != target {
			diffs = append(diffs, path)
		}
	}
	for path := range got.Links {
		if _, ok := want.Links[path]; !ok {
			diffs = append(diffs, path)
		}
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		if len(diffs) > 3 {
			diffs = append(diffs[:3], fmt.Sprintf("… (%d in total)", len(diffs)))
		}
		return check, fmt.Errorf("files differ from the component list: %s", strings.Join(diffs, ", "))
	}
	sigRaw, err := os.ReadFile(filepath.Join(resources, signatureFile))
	if os.IsNotExist(err) {
		return check, nil
	}
	var sig componentSignature
	if err != nil || json.Unmarshal(sigRaw, &sig) != nil {
		return check, errors.New("the component signature is unreadable")
	}
	public, err := base64.StdEncoding.DecodeString(trusted[sig.KeyID])
	if err != nil || len(public) != ed25519.PublicKeySize {
		return check, fmt.Errorf("the component list is signed with an unknown key (%s)", sig.KeyID)
	}
	signature, err := base64.StdEncoding.DecodeString(sig.Signature)
	if err != nil || !ed25519.Verify(public, raw, signature) {
		return check, errors.New("the component signature does not match")
	}
	check.Signed, check.KeyID = true, sig.KeyID
	return check, nil
}
