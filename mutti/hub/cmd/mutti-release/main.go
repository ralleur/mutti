// SPDX-License-Identifier: GPL-2.0-or-later

// mutti-release is owner/release tooling, never shipped in the app. It keeps
// the qualification signing key outside the repository and signs only the
// reviewed records a person selects from a measurement's candidates.
//
//	mutti-release keygen --id mutti-qualification-2026-10 --out KEYFILE
//	mutti-release sign   --key KEYFILE --id ID --candidates a.json[,b.json] --out records.json [--tasks a,b] [--languages de,en]
//	mutti-release verify --file records.json
//	mutti-release sign-components --key KEYFILE --id ID --resources Mutti.app/Contents/Resources
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	hub "github.com/ralleur/mutti/hub"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mutti-release keygen|sign|verify|sign-components ...")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	id := fs.String("id", "", "key ID")
	out := fs.String("out", "", "output file")
	key := fs.String("key", "", "private key file")
	candidates := fs.String("candidates", "", "unsigned candidates from a measurement")
	tasks := fs.String("tasks", "", "comma-separated tasks to sign (default: all passed candidates)")
	languages := fs.String("languages", "", "comma-separated answer languages to sign (default: all)")
	file := fs.String("file", "", "signed records file")
	resources := fs.String("resources", "", "package resources with components.json")
	_ = fs.Parse(os.Args[2:])
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(*id, *out)
	case "sign":
		var private []byte
		if private, err = readKey(*key); err == nil {
			var path string
			if path, err = combineCandidates(split(*candidates)); err == nil {
				err = hub.SignCandidates(path, *out, *id, ed25519.NewKeyFromSeed(private), split(*tasks), split(*languages))
			}
		}
	case "sign-components":
		var private []byte
		if private, err = readKey(*key); err == nil {
			err = signComponents(*resources, *id, ed25519.NewKeyFromSeed(private))
		}
	case "verify":
		var summary string
		summary, err = hub.VerifyRecordsFile(*file)
		fmt.Print(summary)
	default:
		err = fmt.Errorf("unknown command %s", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// signComponents signs the exact bytes of the package's component list,
// which mutti-migrate checks before a build touches the data.
func signComponents(resources, id string, key ed25519.PrivateKey) error {
	if resources == "" || id == "" {
		return fmt.Errorf("--resources and --id are required")
	}
	raw, err := os.ReadFile(filepath.Join(resources, "components.json"))
	if err != nil {
		return err
	}
	b, _ := json.Marshal(map[string]string{"keyId": id, "signature": base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw))})
	if err = os.WriteFile(filepath.Join(resources, "components.sig"), b, 0644); err != nil {
		return err
	}
	fmt.Printf("signed components.json with %s\n", id)
	return nil
}

// combineCandidates merges the candidates of several measurements (e.g. one
// per answer language) into one file for signing. It lives in this tool so
// the shipped hub, whose digest is part of the evidence, stays unchanged.
func combineCandidates(paths []string) (string, error) {
	if len(paths) == 1 {
		return paths[0], nil
	}
	type doc struct {
		Version int               `json:"version"`
		Issued  any               `json:"issued,omitempty"`
		Records []json.RawMessage `json:"records"`
		Revoked []string          `json:"revoked"`
	}
	combined := doc{Version: 1, Revoked: []string{}}
	seen := map[string]bool{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		var d doc
		if err = json.Unmarshal(raw, &d); err != nil || d.Version != 1 {
			return "", fmt.Errorf("invalid candidates file %s", p)
		}
		for _, r := range d.Records {
			var id struct{ ID string }
			_ = json.Unmarshal(r, &id)
			if id.ID == "" || seen[id.ID] {
				return "", fmt.Errorf("record %q missing or twice", id.ID)
			}
			seen[id.ID] = true
			combined.Records = append(combined.Records, r)
		}
	}
	f, err := os.CreateTemp("", "mutti-candidates-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	return f.Name(), json.NewEncoder(f).Encode(combined)
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func keygen(id, out string) error {
	if id == "" || out == "" {
		return fmt.Errorf("--id and --out are required")
	}
	if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("%s exists; refusing to overwrite a release key", out)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(base64.StdEncoding.EncodeToString(private.Seed()) + "\n"); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	fmt.Printf("key id: %s\npublic key: %s\n", id, base64.StdEncoding.EncodeToString(public))
	return nil
}

func readKey(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid key file")
	}
	return seed, nil
}
