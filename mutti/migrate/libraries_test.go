// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLibraryVerificationRelocatesInternalPaths(t *testing.T) {
	instance := filepath.Join(t.TempDir(), "instance")
	media := map[string]string{"/old/media": "/new/media"}
	info := SystemInfo{ProgramDataPath: "/old/jellyfin", InternalMetadataPath: "/old/metadata", CachePath: "/old/cache"}
	mappings := importPathMappings(info, instance, media)
	source := []Library{
		{ItemId: "collections", Name: "Sammlungen", Locations: []string{"/old/jellyfin/data/collections"}},
		{ItemId: "movies", Name: "Filme", Locations: []string{"/old/media/a", "/unchanged/movies"}},
	}
	target := []Library{
		{ItemId: "movies", Name: "Filme", Locations: []string{"/unchanged/movies", "/new/media/a"}},
		{ItemId: "collections", Name: "Sammlungen", Locations: []string{filepath.Join(instance, "data", "data", "collections")}},
	}
	if err := compareLibraries(source, target, mappings); err != nil {
		t.Fatal(err)
	}
	if len(media) != 1 || media["/old/media"] != "/new/media" {
		t.Fatal("caller mappings changed")
	}
	if err := compareLibraries(source, target, media); err == nil || !strings.Contains(err.Error(), "Sammlungen") {
		t.Fatal("regression fixture must require the internal relocation", err)
	}
	for old, want := range map[string]string{
		"/old/jellyfin/root/default": filepath.Join(instance, "data", "root", "default"),
		"/old/metadata/collections":  filepath.Join(instance, "data", "metadata", "collections"),
		"/old/cache/images":          filepath.Join(instance, "cache", "images"),
		"/old/jellyfin-other/media":  "/old/jellyfin-other/media",
	} {
		if got := replacePath(old, mappings); got != filepath.ToSlash(want) {
			t.Fatalf("%s: got %s, want %s", old, got, want)
		}
	}
}

func TestLibraryVerificationRejectsChangedIdentityAndLocations(t *testing.T) {
	original := Library{ItemId: "original", Name: "Sammlungen", Locations: []string{"/source/data/collections"}}
	valid := Library{ItemId: "original", Name: "Sammlungen", Locations: []string{"/target/data/collections"}}
	for _, test := range []struct {
		name   string
		target []Library
		detail string
	}{
		{"old internal path", []Library{original}, "Ordner"},
		{"different path", []Library{{ItemId: "original", Name: "Sammlungen", Locations: []string{"/target/data/other"}}}, "Ordner"},
		{"different identity", []Library{{ItemId: "changed", Name: "Sammlungen", Locations: valid.Locations}}, "Kennung"},
		{"missing identity", []Library{{Name: "Sammlungen", Locations: valid.Locations}}, "Kennung"},
		{"different name", []Library{{ItemId: "original", Name: "Andere", Locations: valid.Locations}}, "Name"},
		{"missing library", nil, "Erwartet"},
		{"extra library", []Library{valid, {ItemId: "extra"}}, "Erwartet"},
		{"missing path", []Library{{ItemId: "original", Name: "Sammlungen"}}, "Ordner"},
		{"extra path", []Library{{ItemId: "original", Name: "Sammlungen", Locations: []string{valid.Locations[0], "/extra"}}}, "Ordner"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := compareLibraries([]Library{original}, test.target, map[string]string{"/source": "/target"})
			if err == nil || !strings.Contains(err.Error(), test.detail) || !strings.Contains(err.Error(), "Der bisherige Datenstand bleibt aktiv.") {
				t.Fatal("missing diagnostic or accepted mismatch", err)
			}
		})
	}
	if err := compareLibraries([]Library{original, original}, []Library{valid, valid}, map[string]string{"/source": "/target"}); err == nil {
		t.Fatal("duplicate identity accepted")
	}
	// Delimiters are legal path characters, not interchangeable list separators.
	if err := compareLibraries([]Library{{ItemId: "id", Name: "a|b", Locations: []string{"c"}}}, []Library{{ItemId: "id", Name: "a", Locations: []string{"b|c"}}}, nil); err == nil {
		t.Fatal("ambiguous concatenation accepted")
	}
}
