// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestHubBackupExcludesModelsAndRestoresWithRollbackCopy(t *testing.T) {
	root := t.TempDir()
	m := &Manager{Options: Options{Root: root}}
	owner, conv, att := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	files := map[string]string{
		"hub.json":            `{"version":1}`,
		"document-tasks.json": `[]`,
		"ai/conversations/" + owner + "/" + conv + ".json":          `{"id":"x"}`,
		"ai/attachments/" + owner + "/" + conv + "/" + att:          "raw",
		"ai/attachments/" + owner + "/" + conv + "/" + att + ".txt": "text",
		"ai/models/blobs/sha256-big":                                "model",
		"ai/engine.log":                                             "log",
		"ai/conversations/" + owner + "/../escape.json":             "x",
	}
	for name, content := range files {
		path := filepath.Join(root, "hub", filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(path), 0700)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(root, "backup")
	names, err := m.snapshotHub(filepath.Join(dir, "hub"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	if len(names) != 5 {
		t.Fatalf("snapshot %v", names)
	}
	for _, n := range names {
		if strings.Contains(n, "models") || strings.Contains(n, "engine.log") || strings.Contains(n, "escape") {
			t.Fatalf("unexpected backup member %s", n)
		}
	}
	b := &storedBackup{Hub: true, Hashes: map[string]string{}, directory: dir}
	for _, n := range names {
		b.Hashes[n] = "verified-by-readBackup"
	}
	_ = os.WriteFile(filepath.Join(root, "hub", "hub.json"), []byte(`{"version":1,"changed":true}`), 0600)
	if err = m.restoreHub(b); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(filepath.Join(root, "hub", "hub.json"))
	if string(restored) != `{"version":1}` {
		t.Fatalf("restored %s", restored)
	}
	if _, err = os.Stat(filepath.Join(root, "hub", "ai", "models", "blobs", "sha256-big")); err != nil {
		t.Fatal("models removed by restore")
	}
	previous, _ := filepath.Glob(filepath.Join(root, "hub-before-restore-*", "hub.json"))
	if len(previous) != 1 {
		t.Fatal("previous module state not kept")
	}
	if hubBackupName.MatchString("hub/ai/conversations/"+owner+"/../../x.json") || hubBackupName.MatchString("hub/ai/models/x") {
		t.Fatal("unsafe backup names accepted")
	}
}
