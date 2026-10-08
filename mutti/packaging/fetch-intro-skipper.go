// SPDX-License-Identifier: GPL-2.0-or-later
// Fetch the curated plugin and matching source with pinned SHA-256 checks.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func fail(err error) {
	if err != nil {
		panic(err)
	}
}
func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func fetch(url, digest string) []byte {
	if !strings.HasPrefix(url, "https://") {
		panic("HTTPS required")
	}
	c := &http.Client{Timeout: 2 * time.Minute}
	r, e := c.Get(url)
	fail(e)
	defer r.Body.Close()
	if r.StatusCode != 200 {
		panic(fmt.Sprintf("download: %d", r.StatusCode))
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	fail(e)
	if hash(b) != digest {
		panic("Intro Skipper checksum mismatch")
	}
	return b
}
func main() {
	if len(os.Args) != 3 {
		panic("manifest and output directory required")
	}
	var lock struct {
		IntroSkipper struct{ Version, PluginId, ArchiveUrl, ArchiveSha256, DllSha256, SourceUrl, SourceSha256 string }
	}
	b, e := os.ReadFile(os.Args[1])
	fail(e)
	fail(json.Unmarshal(b, &lock))
	p := lock.IntroSkipper
	out := os.Args[2]
	fail(os.MkdirAll(out, 0755))
	cached := func(name, url, digest string) []byte {
		dest := filepath.Join(out, name)
		b, e := os.ReadFile(dest)
		if e == nil && hash(b) == digest {
			return b
		}
		b = fetch(url, digest)
		fail(os.WriteFile(dest, b, 0644))
		return b
	}
	archive := cached("plugin.zip", p.ArchiveUrl, p.ArchiveSha256)
	z, e := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	fail(e)
	if len(z.File) != 1 || z.File[0].Name != "IntroSkipper.dll" {
		panic("unexpected plugin payload")
	}
	r, e := z.File[0].Open()
	fail(e)
	dll, e := io.ReadAll(io.LimitReader(r, 4<<20))
	r.Close()
	fail(e)
	if hash(dll) != p.DllSha256 {
		panic("plugin binary checksum mismatch")
	}
	fail(os.WriteFile(filepath.Join(out, "IntroSkipper.dll"), dll, 0644))
	source := cached("source.tar.gz", p.SourceUrl, p.SourceSha256)
	gz, e := gzip.NewReader(bytes.NewReader(source))
	fail(e)
	defer gz.Close()
	tr := tar.NewReader(gz)
	found := false
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		fail(e)
		if strings.HasSuffix(h.Name, "/LICENSE") && strings.Count(h.Name, "/") == 1 {
			license, e := io.ReadAll(io.LimitReader(tr, 1<<20))
			fail(e)
			fail(os.WriteFile(filepath.Join(out, "LICENSE"), license, 0644))
			found = true
			break
		}
	}
	if !found {
		panic("license missing")
	}
	fmt.Println("Verified Intro Skipper", p.Version)
}
