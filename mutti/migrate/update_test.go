// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type updateFixture struct {
	t         *testing.T
	m         *Manager
	resources string
	instance  string
}

func newUpdateFixture(t *testing.T) *updateFixture {
	root := t.TempDir()
	resources := filepath.Join(t.TempDir(), "Resources")
	_ = os.MkdirAll(filepath.Join(resources, "server"), 0700)
	instance := root
	m := &Manager{Options: Options{Root: root, Server: filepath.Join(resources, "server", "jellyfin")}, state: State{Active: instance}, unblock: make(chan struct{}, 1)}
	f := &updateFixture{t: t, m: m, resources: resources, instance: instance}
	f.packageBuild("12.1", "commit-a")
	return f
}

func (f *updateFixture) packageBuild(jellyfin, commit string) {
	lock, _ := json.Marshal(map[string]any{"version": "0.1.0", "jellyfin": map[string]string{"version": jellyfin, "serverCommit": "upstream"}})
	prov, _ := json.Marshal(map[string]any{"server": map[string]any{"commit": commit, "dirty": false}})
	_ = os.WriteFile(filepath.Join(f.resources, "components.lock.json"), lock, 0600)
	_ = os.WriteFile(filepath.Join(f.resources, "build-provenance.json"), prov, 0600)
}

func (f *updateFixture) write(rel, content string) {
	path := filepath.Join(f.m.Options.Root, rel)
	_ = os.MkdirAll(filepath.Dir(path), 0700)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *updateFixture) read(rel string) string {
	b, _ := os.ReadFile(filepath.Join(f.m.Options.Root, rel))
	return string(b)
}

func (f *updateFixture) populate() {
	f.write("config/system.xml", "<ServerConfiguration/>")
	f.write("data/jellyfin.db", "db-v1")
	f.write("data/metadata/poster.jpg", "image")
	f.write("data/transcodes/segment.ts", "temporary")
	f.write("cache/images/x", "cache")
	f.write("logs/log.txt", "log")
	f.write("connect/connect.json", `{"devices":{}}`)
	f.write("hub/hub.json", `{"version":1}`)
	f.write("hub/ai/models/blobs/sha256-x", "model")
}

func TestUpdateGuardFreshInstallRecordsVersion(t *testing.T) {
	f := newUpdateFixture(t)
	if err := f.m.prepareStart(); err != nil {
		t.Fatal(err)
	}
	var v dataVersion
	if json.Unmarshal([]byte(f.read("data-version.json")), &v) != nil || v.Jellyfin != "12.1" || v.Build != "commit-a" {
		t.Fatalf("version %q", f.read("data-version.json"))
	}
	if entries, _ := os.ReadDir(f.m.snapshotDir()); len(entries) != 0 {
		t.Fatal("snapshot for a fresh installation")
	}
}

func TestUpdateGuardSnapshotsBeforeNewBuildAndVerifies(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	if err := f.m.prepareStart(); err != nil { // data from before the guard: snapshot first
		t.Fatal(err)
	}
	f.m.verifyUpdate("12.1.0", true)
	f.packageBuild("12.1", "commit-b")
	if err := f.m.prepareStart(); err != nil {
		t.Fatal(err)
	}
	u := f.m.State().Update
	if u == nil || u.State != "pending" || u.Snapshot == "" || u.From.Build != "commit-a" || u.To.Build != "commit-b" {
		t.Fatalf("update %+v", u)
	}
	s, dir, err := f.m.readSnapshot(u.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"instance/config/system.xml", "instance/data/jellyfin.db", "instance/data/metadata/poster.jpg", "instance/connect/connect.json", "hub/hub.json"} {
		if s.Hashes[want] == "" {
			t.Errorf("missing %s", want)
		}
	}
	for rel := range s.Hashes {
		if strings.Contains(rel, "cache/") || strings.Contains(rel, "logs/") || strings.Contains(rel, "transcodes/") || strings.HasPrefix(rel, "instance/update-snapshots") {
			t.Errorf("excluded or recursive file %s", rel)
		}
	}
	// A tampered snapshot is refused.
	_ = os.WriteFile(filepath.Join(dir, "instance", "data", "jellyfin.db"), []byte("tampered"), 0600)
	if _, _, err := f.m.readSnapshot(u.Snapshot); err == nil {
		t.Fatal("tampered snapshot accepted")
	}
	_ = os.WriteFile(filepath.Join(dir, "instance", "data", "jellyfin.db"), []byte("db-v1"), 0600)
	// Not yet verified while setup is incomplete or the version differs.
	f.m.verifyUpdate("12.1.0", false)
	f.m.verifyUpdate("12.2.0", true)
	if f.m.State().Update.State != "pending" {
		t.Fatal("verified too early")
	}
	f.m.verifyUpdate("12.1.3", true)
	if f.m.State().Update.State != "verified" || !strings.Contains(f.read("data-version.json"), "commit-b") {
		t.Fatalf("not verified: %+v %s", f.m.State().Update, f.read("data-version.json"))
	}
}

func TestOlderBuildIsBlockedAndRollbackRestoresTheSnapshot(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	f.packageBuild("12.2", "commit-new")
	if err := f.m.prepareStart(); err != nil {
		t.Fatal(err)
	}
	snapshot := f.m.State().Update.Snapshot
	// The new server migrates the database and adds data.
	f.write("data/jellyfin.db", "db-v2-migrated")
	f.write("hub/conversation.json", "new")
	// The owner reinstalls the previous version before the update finished.
	f.packageBuild("12.1", "commit-a")
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatalf("older build started on migrated data: %v", err)
	}
	if u := f.m.State().Update; u.State != "blocked" || u.Snapshot != snapshot || f.m.State().Phase != "update_blocked" {
		t.Fatalf("blocked state %+v", u)
	}
	// A snapshot of another version is refused for this build.
	f.packageBuild("12.2", "commit-new")
	if err := f.m.rollbackUpdate(snapshot); err == nil || !strings.Contains(err.Error(), "Jellyfin 12.1") {
		t.Fatalf("foreign version rollback: %v", err)
	}
	f.packageBuild("12.1", "commit-a")
	if err := f.m.rollbackUpdate(snapshot); err != nil {
		t.Fatal(err)
	}
	if f.read("data/jellyfin.db") != "db-v1" || f.read("hub/conversation.json") != "" || f.read("hub/hub.json") != `{"version":1}` {
		t.Fatalf("not restored: %q", f.read("data/jellyfin.db"))
	}
	if f.read("hub/ai/models/blobs/sha256-x") != "model" {
		t.Fatal("downloaded models lost")
	}
	kept, _ := filepath.Glob(filepath.Join(f.m.Options.Root, "before-rollback-*", "instance", "data", "jellyfin.db"))
	if len(kept) != 1 {
		t.Fatal("replaced data not kept")
	}
	if b, _ := os.ReadFile(kept[0]); string(b) != "db-v2-migrated" {
		t.Fatalf("kept %q", b)
	}
	if err := f.m.prepareStart(); err != nil {
		t.Fatalf("restored build still blocked: %v", err)
	}
}

func TestNewerDataVersionBlocksOlderPackage(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	f.write("data-version.json", `{"schema":1,"jellyfin":"12.10","build":"x"}`)
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatalf("12.1 started on 12.10 data: %v", err)
	}
	if !strings.Contains(f.m.State().Message, "älter") {
		t.Fatalf("message %q", f.m.State().Message)
	}
}

func TestUpdateIsRefusedWithoutSpaceAndNothingChanges(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	saved := freeBytes
	freeBytes = func(string) (uint64, error) { return 1024, nil }
	defer func() { freeBytes = saved }()
	f.packageBuild("12.1", "commit-b")
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatalf("started without a snapshot: %v", err)
	}
	if _, err := f.m.readUpdate(); err != nil {
		t.Fatal(err)
	}
	if u, _ := f.m.readUpdate(); u != nil && u.To.Build == "commit-b" {
		t.Fatalf("update recorded without snapshot: %+v", u)
	}
	if !strings.Contains(f.read("data-version.json"), "commit-a") || f.read("data/jellyfin.db") != "db-v1" {
		t.Fatal("data changed")
	}
	// With space the next start proceeds.
	freeBytes = saved
	if err := f.m.prepareStart(); err != nil {
		t.Fatal(err)
	}
}

func TestFailedUpdateIsRetriedByTheSameBuild(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	f.packageBuild("12.1", "commit-b")
	_ = f.m.prepareStart()
	f.m.failUpdate(f.m.State().Update, "test")
	if u, _ := f.m.readUpdate(); u.State != "failed" || !strings.Contains(u.Message, "vorherige Mutti-Version") {
		t.Fatalf("%+v", u)
	}
	if err := f.m.prepareStart(); err != nil || f.m.State().Update.State != "pending" {
		t.Fatalf("retry %v %+v", err, f.m.State().Update)
	}
	// A newer fix build takes over the open update and keeps its snapshot.
	snapshot := f.m.State().Update.Snapshot
	f.packageBuild("12.1", "commit-c")
	if err := f.m.prepareStart(); err != nil || f.m.State().Update.To.Build != "commit-c" || f.m.State().Update.Snapshot != snapshot {
		t.Fatalf("takeover %v %+v", err, f.m.State().Update)
	}
	// The version the update started from may not continue on that data.
	f.packageBuild("12.1", "commit-a")
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatalf("previous build continued: %v", err)
	}
	f.packageBuild("12.1", "commit-c")
	_ = f.m.prepareStart()
	// Corrupt module state fails verification.
	f.write("hub/hub.json", "{broken")
	f.m.verifyUpdate("12.1.0", true)
	if f.m.State().Update.State != "failed" {
		t.Fatal("broken module state verified")
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"12.1", "12.1", 0}, {"12.1", "12.10", -1}, {"12.2", "12.1", 1}, {"12.1", "12.1.0", 0}, {"13", "12.9", 1}} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("%s vs %s = %d", c.a, c.b, got)
		}
	}
}

func TestRollbackRouteNeedsNativeAppAndBlockedStart(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	f.packageBuild("12.2", "commit-new")
	_ = f.m.prepareStart()
	snapshot := f.m.State().Update.Snapshot
	f.m.Options.Origin = "http://127.0.0.1:18594"
	f.m.token, f.m.nativeOwnerToken = randomID(), randomID()
	h, err := f.m.Handler()
	if err != nil {
		t.Fatal(err)
	}
	post := func(native string) int {
		r := httptest.NewRequest("POST", "/api/update/rollback", strings.NewReader(`{"Snapshot":"`+snapshot+`"}`))
		r.Host = "127.0.0.1:18594"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Mutti-CSRF", f.m.token)
		if native != "" {
			r.Header.Set("X-Mutti-Native-Owner", native)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if code := post(""); code != 403 {
		t.Fatalf("browser rollback %d", code)
	}
	if code := post(f.m.nativeOwnerToken); code != 409 {
		t.Fatalf("rollback while not blocked %d", code)
	}
	f.packageBuild("12.1", "commit-a")
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatal(err)
	}
	if code := post(f.m.nativeOwnerToken); code != 202 {
		t.Fatalf("native rollback %d", code)
	}
	select {
	case <-f.m.unblock:
	default:
		t.Fatal("start not unblocked")
	}
	if f.read("data/jellyfin.db") != "db-v1" {
		t.Fatal("not restored")
	}
}

func TestInterruptedSnapshotIsDiscardedOnNextStart(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	f.write("update-snapshots/.incomplete-crashed/instance/data/jellyfin.db", "partial")
	f.packageBuild("12.1", "commit-b")
	if err := f.m.prepareStart(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.m.snapshotDir(), ".incomplete-crashed")); !os.IsNotExist(err) {
		t.Fatal("interrupted snapshot kept")
	}
	if _, _, err := f.m.readSnapshot(f.m.State().Update.Snapshot); err != nil {
		t.Fatal(err)
	}
}

// Access withdrawn while the new version ran stays withdrawn after rolling
// back; devices paired after the snapshot must pair again.
func TestRollbackKeepsLaterRevocations(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	f.write("connect/connect.json", `{"owner":"o","devices":{"A":{"name":"iPad"},"B":{"name":"iPhone"}}}`)
	f.write("hub/hub.json", `{"version":1,"modules":{"documents":{"enabled":true,"grants":{"u1":true,"u2":true},"links":{"u1":{"account":"a"},"u2":{"account":"b"}}},
		"photos":{"enabled":true,"grants":{"u1":true}}}}`)
	_ = f.m.prepareStart()
	f.m.verifyUpdate("12.1.0", true)
	f.packageBuild("12.2", "commit-new")
	if err := f.m.prepareStart(); err != nil {
		t.Fatal(err)
	}
	snapshot := f.m.State().Update.Snapshot
	// While the new version runs: B is revoked, C paired, u2 loses documents
	// and its link, photos is switched off.
	f.write("connect/connect.json", `{"owner":"o","devices":{"A":{"name":"iPad"},"C":{"name":"Mac"}}}`)
	f.write("hub/hub.json", `{"version":1,"modules":{"documents":{"enabled":true,"grants":{"u1":true,"u2":false},"links":{"u1":{"account":"a"}}},
		"photos":{"enabled":false,"grants":{"u1":true}}}}`)
	f.packageBuild("12.1", "commit-a")
	if err := f.m.prepareStart(); !errors.Is(err, errUpdateBlocked) {
		t.Fatal(err)
	}
	if err := f.m.rollbackUpdate(snapshot); err != nil {
		t.Fatal(err)
	}
	var connect struct {
		Owner   string
		Devices map[string]any
	}
	_ = json.Unmarshal([]byte(f.read("connect/connect.json")), &connect)
	if connect.Owner != "o" || len(connect.Devices) != 1 || connect.Devices["A"] == nil {
		t.Fatalf("devices after rollback %v", connect.Devices)
	}
	var hub struct {
		Modules map[string]struct {
			Enabled bool
			Grants  map[string]bool
			Links   map[string]any
		}
	}
	_ = json.Unmarshal([]byte(f.read("hub/hub.json")), &hub)
	docs, photos := hub.Modules["documents"], hub.Modules["photos"]
	if !docs.Enabled || !docs.Grants["u1"] || docs.Grants["u2"] || docs.Links["u2"] != nil || docs.Links["u1"] == nil || photos.Enabled {
		t.Fatalf("hub after rollback %+v", hub.Modules)
	}
	if u := f.m.State().Update; !strings.Contains(u.Message, "4 nach der Sicherung entzogene") {
		t.Fatalf("message %q", u.Message)
	}
}

func TestUnreadableDeviceStateFailsVerification(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "connect"), 0700)
	_ = os.WriteFile(filepath.Join(dir, "connect", "connect.json"), []byte("{broken"), 0600)
	if err := checkModuleState(t.TempDir(), dir); err == nil {
		t.Fatal("broken device state accepted")
	}
}
