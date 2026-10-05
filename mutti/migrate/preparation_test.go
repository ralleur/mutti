// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAutomaticSourcePreparation(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		allowed, pending, failWrite, foreign bool
	}{
		{name: "normal", allowed: true}, {name: "retry pending restart", allowed: true, pending: true}, {name: "no consent"}, {name: "write fails", allowed: true, failWrite: true}, {name: "foreign server", allowed: true, foreign: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			config := map[string]json.RawMessage{"DatabaseType": json.RawMessage(`"Jellyfin-SQLite"`), "LockingBehavior": json.RawMessage(`"Pessimistic"`), "UnknownSetting": json.RawMessage(`{"Preserve":42}`)}
			original, _ := json.Marshal(config)
			changed, restarts, polls := 0, 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.URL.Path {
				case "/System/Configuration/database":
					if r.Method == "POST" {
						changed++
						if tc.failWrite {
							w.WriteHeader(500)
							return
						}
						if json.NewDecoder(r.Body).Decode(&config) != nil {
							t.Error("bad JSON")
						}
						w.WriteHeader(204)
						return
					}
					json.NewEncoder(w).Encode(config)
				case "/System/Restart":
					restarts++
					w.WriteHeader(204)
				case "/System/Info/Public":
					id := "source"
					ready := true
					if restarts > 0 {
						polls++
						if polls == 1 {
							ready = false
						}
						if tc.foreign && polls > 1 {
							id = "foreign"
						}
					}
					json.NewEncoder(w).Encode(PublicInfo{Id: id, Version: "12.1.0", StartupWizardCompleted: ready})
				default:
					t.Error("unexpected request", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			api, _ := NewAPI(srv.URL)
			source := &Source{API: api, Info: SystemInfo{PublicInfo: PublicInfo{Id: "source"}}}
			root := t.TempDir()
			m := &Manager{Options: Options{Root: root}}
			if tc.pending {
				config["LockingBehavior"] = json.RawMessage(`"NoLock"`)
				prepared, _ := json.Marshal(config)
				if e := savePreparation(root, "source", srv.URL, "json", original, prepared); e != nil {
					t.Fatal(e)
				}
			}
			e := m.prepareSource(context.Background(), source, SourceInput{PrepareSource: tc.allowed})
			wantError := !tc.allowed || tc.failWrite || tc.foreign
			if (e != nil) != wantError {
				t.Fatal(e)
			}
			if !tc.allowed {
				if changed != 0 || restarts != 0 {
					t.Fatal("changed without consent")
				}
				return
			}
			if string(config["UnknownSetting"]) != `{"Preserve":42}` {
				t.Fatal("unknown setting lost")
			}
			dir := preparationDir(root, "source", srv.URL)
			backups, _ := filepath.Glob(filepath.Join(dir, "database-original-*.json"))
			if len(backups) != 1 {
				t.Fatal("missing original", backups)
			}
			b, _ := os.ReadFile(backups[0])
			st, _ := os.Stat(backups[0])
			if !bytes.Equal(b, original) || st.Mode().Perm() != 0600 {
				t.Fatal("original not retained privately")
			}
			if tc.failWrite {
				if restarts != 0 {
					t.Fatal("restarted after rejected config")
				}
				return
			}
			if restarts != 1 {
				t.Fatal("restart count", restarts)
			}
			if tc.pending && changed != 0 {
				t.Fatal("pending retry changed configuration again")
			}
			_, pendingErr := os.Stat(filepath.Join(dir, "pending.json"))
			if tc.foreign {
				if pendingErr != nil {
					t.Fatal("lost recovery receipt")
				}
			} else if !os.IsNotExist(pendingErr) {
				t.Fatal("pending receipt not completed")
			}
		})
	}
}
func TestRestartRequiresObservedRestart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/System/Restart" {
			w.WriteHeader(204)
			return
		}
		json.NewEncoder(w).Encode(PublicInfo{Id: "source", StartupWizardCompleted: true})
	}))
	defer srv.Close()
	api, _ := NewAPI(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	if e := restartSourceAPI(ctx, &Source{API: api, Info: SystemInfo{PublicInfo: PublicInfo{Id: "source"}}}); e == nil {
		t.Fatal("saved config treated as restart proof")
	}
}
func TestSourcePreparationRequiresAdmin(t *testing.T) {
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Users/AuthenticateByName" {
			writes++
		}
		w.WriteHeader(401)
	}))
	defer srv.Close()
	m := &Manager{Options: Options{Root: t.TempDir()}}
	if _, e := m.openImportSource(context.Background(), SourceInput{Address: srv.URL, PrepareSource: true}, "target"); e == nil {
		t.Fatal("source login bypassed")
	}
	if writes != 0 {
		t.Fatal("changed source before admin login")
	}
}
func TestNativeAgentQualificationAndXML(t *testing.T) {
	exe := "/Applications/Jellyfin.app/Contents/MacOS/jellyfin"
	valid := launchAgent{Label: "org.example.jellyfin", ProgramArguments: []string{exe, "--datadir", "/Users/test/jellyfin", "--configdir=/Users/test/jellyfin/config"}, KeepAlive: json.RawMessage(`true`)}
	if data, config, ok := qualifyAgent(valid, exe); !ok || data != "/Users/test/jellyfin" || config != "/Users/test/jellyfin/config" {
		t.Fatal(data, config, ok)
	}
	for _, change := range []func(*launchAgent){func(a *launchAgent) { a.Label = "../../other" }, func(a *launchAgent) { a.KeepAlive = json.RawMessage(`false`) }, func(a *launchAgent) { a.Program = "/bin/sh" }, func(a *launchAgent) { a.ProgramArguments = []string{"/bin/sh", "-c", exe} }, func(a *launchAgent) { a.EnvironmentVariables = map[string]string{}; a.ProgramArguments = []string{exe} }} {
		a := valid
		change(&a)
		if _, _, ok := qualifyAgent(a, exe); ok {
			t.Fatal("unsafe agent accepted", a.Label)
		}
	}
	original := []byte("<?xml version=\"1.0\"?>\n<DatabaseConfigurationOptions><DatabaseType>Jellyfin-SQLite</DatabaseType><LockingBehavior>Pessimistic</LockingBehavior><Unknown>keep</Unknown></DatabaseConfigurationOptions>")
	got, changed, e := prepareDatabaseXML(original)
	if e != nil || !changed || !bytes.Equal(got, bytes.Replace(original, []byte("Pessimistic"), []byte("NoLock"), 1)) {
		t.Fatal("changed unrelated XML", e)
	}
	for _, b := range [][]byte{bytes.Replace(original, []byte("Pessimistic"), []byte("NoLock"), 1), bytes.Replace(original, []byte("Jellyfin-SQLite"), []byte("External"), 1)} {
		if _, changed, e = prepareDatabaseXML(b); e != nil || changed {
			t.Fatal("unneeded change")
		}
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	path := filepath.Join(root, "database.xml")
	os.WriteFile(path, original, 0600)
	if _, e = readOwnedFile(path, 1<<20); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(root, "link.xml")
	os.Symlink(path, link)
	if _, e = readOwnedFile(link, 1<<20); e == nil {
		t.Fatal("symlink accepted")
	}
	os.Chmod(path, 0666)
	if _, e = readOwnedFile(path, 1<<20); e == nil {
		t.Fatal("shared config accepted")
	}
}
func TestPreparationReceiptBoundToExactSourceAndConfig(t *testing.T) {
	root := t.TempDir()
	if e := savePreparation(root, "one", "http://localhost:1", "xml", []byte("old"), []byte("new")); e != nil {
		t.Fatal(e)
	}
	if !preparationConflict(root, "one", "http://localhost:1", "xml", []byte("changed")) || preparationConflict(root, "one", "http://localhost:1", "xml", []byte("old")) || preparationConflict(root, "one", "http://localhost:1", "xml", []byte("new")) {
		t.Fatal("configuration conflict was not distinguished from a resumable write")
	}
	if !pendingPreparation(root, "one", "http://localhost:1", "xml", []byte("new")) {
		t.Fatal("lost pending change")
	}
	for _, arg := range []struct{ id, address, format, data string }{{"two", "http://localhost:1", "xml", "new"}, {"one", "http://localhost:2", "xml", "new"}, {"one", "http://localhost:1", "json", "new"}, {"one", "http://localhost:1", "xml", "changed"}} {
		if pendingPreparation(root, arg.id, arg.address, arg.format, []byte(arg.data)) {
			t.Fatal("receipt crossed boundary")
		}
	}
}
func TestInspectRealLocalSourceReadOnly(t *testing.T) {
	if os.Getenv("MUTTI_INSPECT_SOURCE") != "1" {
		t.Skip("read-only local qualification")
	}
	api, _ := NewAPI("http://127.0.0.1:8096")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var info PublicInfo
	if e := api.call(ctx, "GET", "/System/Info/Public", nil, &info); e != nil {
		t.Fatal(e)
	}
	home, _ := os.UserHomeDir()
	source, e := inspectLocalSource(ctx, api, info, filepath.Join(home, "Library/Application Support/Mutti Preview"))
	if e != nil || source == nil {
		t.Fatal("source could not be qualified", e)
	}
	b, e := readOwnedFile(source.Config, 1<<20)
	if e != nil {
		t.Fatal(e)
	}
	_, changes, e := prepareDatabaseXML(b)
	if e != nil || !changes {
		t.Fatal("expected preparation not identified", e)
	}
	t.Log("PASS: local source identity, user service and source mode qualified without writes or restart")
}
