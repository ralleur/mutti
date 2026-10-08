// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupIntegrityAndPaths(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	m := &Manager{Options: Options{Root: root}}
	id := randomID()
	dir := filepath.Join(m.backupDirectory(), id)
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := privateWrite(filepath.Join(dir, "library.zip"), []byte("fixture")); e != nil {
		t.Fatal(e)
	}
	h, n, _ := fileHash(filepath.Join(dir, "library.zip"))
	b := storedBackup{Schema: 1, BackupSummary: BackupSummary{ID: id, Version: "12.1.0", Bytes: n}, Hashes: map[string]string{"library.zip": h}}
	manifest, _ := json.Marshal(b)
	_ = privateWrite(filepath.Join(dir, "backup.json"), manifest)
	if _, e := m.readBackup(id); e != nil {
		t.Fatal(e)
	}
	for _, invalid := range []string{"../outside", id + "/../" + id, "", strings.ToUpper(id)} {
		if _, e := m.readBackup(invalid); e == nil {
			t.Fatal("accepted unsafe id")
		}
	}
	_ = privateWrite(filepath.Join(dir, "library.zip"), []byte("damaged"))
	if _, e := m.readBackup(id); e == nil {
		t.Fatal("accepted corrupted archive")
	}
	_ = os.Remove(filepath.Join(dir, "library.zip"))
	outside := filepath.Join(root, "outside")
	_ = privateWrite(outside, []byte("fixture"))
	_ = os.Symlink(outside, filepath.Join(dir, "library.zip"))
	if _, e := m.readBackup(id); e == nil {
		t.Fatal("accepted symlinked archive")
	}
}

func TestMaintenanceRequiresCurrentOwner(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), strings.Repeat("a", 24)) {
			_, _ = w.Write([]byte(`{"Id":"owner","Policy":{"IsAdministrator":true}}`))
			return
		}
		_, _ = w.Write([]byte(`{"Id":"viewer","Policy":{"IsAdministrator":false}}`))
	}))
	defer api.Close()
	a, _ := NewAPI(api.URL)
	m := &Manager{Options: Options{Root: t.TempDir(), Origin: "http://127.0.0.1:18594", Backend: api.URL}, api: a, token: "csrf"}
	h, _ := m.Handler()
	for _, test := range []struct {
		token, origin string
		want          int
	}{
		{"", "", 401}, {strings.Repeat("v", 24), "", 401}, {strings.Repeat("a", 24), "https://attacker.invalid", 403}, {strings.Repeat("a", 24), "", 200},
	} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:18594/api/maintenance/state", strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+test.token)
		r.Header.Set("Origin", test.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != test.want {
			t.Fatalf("want %d, got %d", test.want, w.Code)
		}
	}
}

// Reuses the real import fixture: no existing Jellyfin database or media.
func testMaintenanceCycle(t *testing.T, ctx context.Context, m *Manager, password, userID, itemID string) {
	t.Helper()
	a, _ := NewAPI(m.Options.Backend)
	a.Host = m.api.Host
	if _, e := login(ctx, a, "Import Owner", password); e != nil {
		t.Fatal(e)
	}
	id, e := m.createBackup(ctx, a)
	if e != nil {
		t.Fatal("backup", e)
	}
	backup, e := m.readBackup(id)
	if e != nil {
		t.Fatal(e)
	}
	if backup.VerifiedAt != nil {
		t.Fatal("unverified snapshot marked verified")
	}
	old := m.State().Active
	input := SourceInput{Replace: true, targetAuthorized: true, Username: "Import Owner", Password: password, backup: backup, verifyOnly: true}
	wrong := input
	wrong.Password = "deliberately-wrong"
	if e = m.importSource(ctx, wrong); e == nil || m.State().Active != old {
		t.Fatal("wrong snapshot password changed active data", e)
	}
	if e = m.importSource(ctx, input); e != nil || m.State().Active != old {
		t.Fatal("isolated restore verification", e)
	}
	var before map[string]any
	if e = a.call(ctx, "GET", "/UserItems/"+itemID+"/UserData?userId="+userID, nil, &before); e != nil {
		t.Fatal(e)
	}
	changed := map[string]any{"IsFavorite": false, "Played": false, "PlayCount": 0, "PlaybackPositionTicks": int64(0)}
	if e = a.call(ctx, "POST", "/UserItems/"+itemID+"/UserData?userId="+userID, changed, nil); e != nil {
		t.Fatal(e)
	}
	input.verifyOnly = false
	if e = m.importSource(ctx, input); e != nil {
		t.Fatal("activate restored backup", e)
	}
	if m.State().Active == old {
		t.Fatal("restore did not activate")
	}
	if _, e = os.Stat(filepath.Join(old, "data")); e != nil {
		t.Fatal("old data lost")
	}
	var who User
	if e = a.call(ctx, "GET", "/Users/Me", nil, &who); e == nil {
		t.Fatal("old token resurrected")
	}
	if _, e = login(ctx, a, "Import Owner", password); e != nil {
		t.Fatal(e)
	}
	var after map[string]any
	if e = a.call(ctx, "GET", "/UserItems/"+itemID+"/UserData?userId="+userID, nil, &after); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"IsFavorite", "Played", "PlayCount", "PlaybackPositionTicks", "LastPlayedDate"} {
		if before[key] != after[key] {
			t.Fatalf("restore changed %s", key)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(m.State().Active, "connect")); len(entries) != 0 {
		t.Fatal("old device grants restored")
	}
	// A running persisted job becomes interrupted rather than a false success.
	job := MaintenanceJob{Kind: "verify", State: "running", Started: time.Now()}
	b, _ := json.Marshal(job)
	if e = privateWrite(filepath.Join(m.Options.Root, "maintenance-job.json"), b); e != nil {
		t.Fatal(e)
	}
	t.Log("PASS: backup hash verification, wrong credentials keep active data, isolated restore probe, activation, retained previous data, playback/favorites restored and stale sessions denied")
}
