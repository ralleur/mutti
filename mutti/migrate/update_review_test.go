// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regressions from the P3 review (2026-10-07).

func (f *updateFixture) build(jellyfin, commit string, commitTime int64) {
	f.packageBuild(jellyfin, commit)
	prov, _ := json.Marshal(map[string]any{"server": map[string]any{"commit": commit, "commitTime": commitTime, "dirty": false}})
	_ = os.WriteFile(filepath.Join(f.resources, "build-provenance.json"), prov, 0600)
}

func TestMissingVersionFileNeverDisablesTheGuard(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	_ = os.Remove(filepath.Join(f.resources, "components.lock.json"))
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatalf("lock removed after data was recorded: %v", err)
	}
	g := newUpdateFixture(t)
	_ = os.Remove(filepath.Join(g.resources, "components.lock.json"))
	writePackage(t, g.resources)
	_, _ = WriteComponents(g.resources)
	if err := g.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatalf("package with component list but no version: %v", err)
	}
	h := newUpdateFixture(t)
	_ = os.Remove(filepath.Join(h.resources, "components.lock.json"))
	if err := h.m.prepareStart(); err != nil {
		t.Fatalf("development run without package: %v", err)
	}
}

func TestBuildsOfTheSameReleaseAreOrdered(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	f.build("12.1", "commit-a", 1000)
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	f.build("12.1", "commit-b", 2000)
	if err := f.m.prepareStart(); err != nil || f.m.State().Update.State != "pending" {
		t.Fatalf("newer build: %v", err)
	}
	// An older build of the same release does not take over the update.
	f.build("12.1", "commit-z", 500)
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatalf("older build took over: %v", err)
	}
	f.build("12.1", "commit-b", 2000)
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.3", true)
	// Reinstalling the previous build after B was verified is a downgrade.
	f.build("12.1", "commit-a", 1000)
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) || f.m.State().Update.Snapshot == "" {
		t.Fatalf("older build after verification: %v %+v", err, f.m.State().Update)
	}
	// A rebuild of the same sources (same commit time) is an update.
	f.build("12.1", "commit-b+dirty", 2000)
	if err := f.m.prepareStart(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
}

func TestRollbackRestoresTheActiveInstancePointer(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	f.packageBuild("12.2", "commit-new")
	_ = f.m.prepareStart()
	snapshot := f.m.State().Update.Snapshot
	// Under the new version the owner imports a library: instance X.
	id := randomID()
	x := filepath.Join(f.m.Options.Root, "instances", id)
	f.write("instances/"+id+"/data/jellyfin.db", "db-created-by-12.2")
	f.write("instances/"+id+"/report.json", `{"verified":true}`)
	f.write("instances/"+id+"/connect/connect.json", `{"devices":{}}`)
	f.write("active-instance.json", `{"ID":"`+id+`"}`)
	f.m.mu.Lock()
	f.m.state.Active = x
	f.m.mu.Unlock()
	f.packageBuild("12.1", "commit-a")
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatal(err)
	}
	if err := f.m.rollbackUpdate(snapshot); err != nil {
		t.Fatal(err)
	}
	if f.m.State().Active != f.m.Options.Root || f.read("active-instance.json") != "" {
		t.Fatalf("active %s, pointer %q", f.m.State().Active, f.read("active-instance.json"))
	}
	if f.read("data/jellyfin.db") != "db-v1" || f.read("instances/"+id+"/data/jellyfin.db") != "db-created-by-12.2" {
		t.Fatal("wrong data active after rollback")
	}
}

func TestInterruptedRollbackResumesWithTheOriginalAccess(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	f.write("connect/connect.json", `{"devices":{"A":{"userId":"u","token":"t"},"B":{"userId":"u","token":"t2"}}}`)
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	f.packageBuild("12.2", "commit-new")
	_ = f.m.prepareStart()
	snapshot := f.m.State().Update.Snapshot
	f.write("connect/connect.json", `{"devices":{"A":{"userId":"u","token":"t"}}}`) // B revoked
	f.write("data/jellyfin.db", "db-v2-migrated")
	f.packageBuild("12.1", "commit-a")
	_ = f.m.prepareStart()
	// Simulate a crash after the move and a partial restore.
	keepRel := "before-rollback-crashed"
	keep := filepath.Join(f.m.Options.Root, keepRel)
	before := filepath.Join(keep, "access-before-rollback")
	_ = os.MkdirAll(before, 0700)
	_ = os.WriteFile(filepath.Join(before, "connect.json"), []byte(f.read("connect/connect.json")), 0600)
	_ = os.WriteFile(filepath.Join(before, ".complete"), []byte("1"), 0600)
	for _, entry := range []string{"config", "data", "connect"} {
		_ = os.MkdirAll(filepath.Join(keep, "instance"), 0700)
		_ = os.Rename(filepath.Join(f.m.Options.Root, entry), filepath.Join(keep, "instance", entry))
	}
	_ = os.Rename(filepath.Join(f.m.Options.Root, "hub"), filepath.Join(keep, "hub"))
	_ = os.WriteFile(filepath.Join(keep, ".moved"), []byte("1"), 0600)
	f.write("connect/connect.json", `{"devices":{"A":{"userId":"u","token":"t"},"B":{"userId":"u","token":"t2"}}}`) // partial restore
	u := &UpdateState{ID: randomID(), State: "rolling_back", Snapshot: snapshot, Keep: keepRel}
	_ = f.m.writeUpdate(u)
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) || f.m.State().Update.State != "rolling_back" {
		t.Fatalf("interrupted rollback not blocked: %v", err)
	}
	if err := f.m.rollbackUpdate(snapshot); err != nil {
		t.Fatal(err)
	}
	var connect struct{ Devices map[string]any }
	_ = json.Unmarshal([]byte(f.read("connect/connect.json")), &connect)
	if len(connect.Devices) != 1 || connect.Devices["B"] != nil || f.read("data/jellyfin.db") != "db-v1" {
		t.Fatalf("resumed rollback: devices %v db %q", connect.Devices, f.read("data/jellyfin.db"))
	}
	if b, _ := os.ReadFile(filepath.Join(keep, "instance", "data", "jellyfin.db")); string(b) != "db-v2-migrated" {
		t.Fatal("replaced data lost")
	}
	if err := f.m.prepareStart(); err != nil {
		t.Fatalf("after resumed rollback: %v", err)
	}
}

func TestRevocationsCompareTheBinding(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte(content), 0600)
		return p
	}
	restoredConnect := write("restored-connect.json", `{"devices":{"A":{"userId":"u1","token":"t1"},"B":{"userId":"u1","token":"t2"}}}`)
	beforeConnect := write("before-connect.json", `{"devices":{"A":{"userId":"u1","token":"t1"},"B":{"userId":"u2","token":"t9"}}}`)
	restoredHub := write("restored-hub.json", `{"modules":{"photos":{"enabled":true,"grants":{"u1":true},"links":{"u1":{"account":"old","secret":"s1"}}}}}`)
	beforeHub := write("before-hub.json", `{"modules":{"photos":{"enabled":true,"grants":{"u1":true},"links":{"u1":{"account":"new","secret":"s2"}}}}}`)
	n, err := keepRevocations(beforeConnect, restoredConnect, beforeHub, restoredHub)
	if err != nil || n != 2 {
		t.Fatalf("withdrawn %d %v", n, err)
	}
	b, _ := os.ReadFile(restoredConnect)
	h, _ := os.ReadFile(restoredHub)
	if strings.Contains(string(b), `"B"`) || strings.Contains(string(h), `"old"`) {
		t.Fatalf("rebound device or re-pointed link kept: %s %s", b, h)
	}
}

func TestSymlinkedDataFolderBlocksTheUpdate(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	snapshots, _ := os.ReadDir(f.m.snapshotDir())
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "jellyfin.db"), []byte("db-v1"), 0600)
	_ = os.RemoveAll(filepath.Join(f.m.Options.Root, "data"))
	_ = os.Symlink(outside, filepath.Join(f.m.Options.Root, "data"))
	f.packageBuild("12.2", "commit-new")
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) || !strings.Contains(f.m.State().Update.Message, "verweist") {
		t.Fatalf("symlinked data folder: %v", err)
	}
	if entries, _ := os.ReadDir(f.m.snapshotDir()); len(entries) != len(snapshots) {
		t.Fatalf("snapshot of a link taken: %v", entries)
	}
}

func TestTamperedSnapshotIsRefused(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	f.packageBuild("12.2", "commit-new")
	_ = f.m.prepareStart()
	id := f.m.State().Update.Snapshot
	dir := filepath.Join(f.m.snapshotDir(), id)
	_ = os.WriteFile(filepath.Join(dir, "instance", "config", "extra.xml"), []byte("x"), 0600)
	if _, _, err := f.m.readSnapshot(id); err == nil {
		t.Fatal("unlisted file accepted")
	}
	_ = os.Remove(filepath.Join(dir, "instance", "config", "extra.xml"))
	var s snapshotManifest
	b, _ := os.ReadFile(filepath.Join(dir, "snapshot.json"))
	_ = json.Unmarshal(b, &s)
	s.Active = "../victim"
	b, _ = json.Marshal(s)
	_ = os.WriteFile(filepath.Join(dir, "snapshot.json"), b, 0600)
	if _, _, err := f.m.readSnapshot(id); err == nil {
		t.Fatal("active path outside the root accepted")
	}
}

func TestPruningKeepsTheWayBack(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	var first string
	for i, v := range []string{"12.2", "12.3", "12.4", "12.5"} {
		f.packageBuild(v, "commit-"+v)
		if err := f.m.prepareStart(); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = f.m.State().Update.Snapshot // data of 12.1
		}
		f.m.verifyUpdate(v+".0", true)
	}
	if _, _, err := f.m.readSnapshot(first); err != nil {
		t.Fatalf("only snapshot of Jellyfin 12.1 pruned: %v", err)
	}
}

func TestReplacedPackageIsNotRelaunched(t *testing.T) {
	f := newUpdateFixture(t)
	writePackage(t, f.resources)
	_, _ = WriteComponents(f.resources)
	if err := f.m.prepareStart(); err != nil {
		t.Fatal(err)
	}
	if f.m.packageReplaced() {
		t.Fatal("unchanged package reported as replaced")
	}
	_ = os.WriteFile(filepath.Join(f.resources, "server", "jellyfin"), []byte("other server"), 0644)
	_, _ = WriteComponents(f.resources)
	if !f.m.packageReplaced() || !errors.Is(f.m.launch(f.m.Options.Root), errPackageReplaced) {
		t.Fatal("replaced package would be launched")
	}
}

func TestComponentScanIgnoresFinderFilesButNotLinkChains(t *testing.T) {
	dir := t.TempDir()
	writePackage(t, dir)
	_, _ = WriteComponents(dir)
	_ = os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("x"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "web", "._index.html"), []byte("x"), 0644)
	if _, err := verifyComponents(dir, nil); err != nil {
		t.Fatalf("Finder metadata: %v", err)
	}
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "evil"), []byte("x"), 0644)
	_ = os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0755)
	rel, _ := filepath.Rel(filepath.Join(dir, "sub", "deep"), outside)
	_ = os.Symlink(rel, filepath.Join(dir, "sub", "deep", "up"))
	if _, err := WriteComponents(dir); err == nil {
		t.Fatal("link out of the package accepted")
	}
}
