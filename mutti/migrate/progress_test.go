// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProgressDurationsAndStageChanges(t *testing.T) {
	now := time.Now()
	p := Progress{StartedAt: now.Add(-120 * time.Second), StepStartedAt: now.Add(-90 * time.Second), LastActivityAt: now.Add(-30 * time.Second)}
	got := p.at(now)
	if got.ElapsedSeconds != 120 || got.StepSeconds != 90 || got.IdleSeconds != 30 {
		t.Fatal(got)
	}
	p.FinishedAt = now
	if p.at(now.Add(time.Hour)).ElapsedSeconds != 120 {
		t.Fatal("finished clock kept running")
	}
	m := &Manager{state: State{Progress: p}}
	m.setStep("verifying", 6, "source check")
	first := m.State().Progress
	m.setStep("verifying", 6, "compare")
	second := m.State().Progress
	if first.StepStartedAt != second.StepStartedAt || second.StartedAt != p.StartedAt || second.Measured {
		t.Fatal(second)
	}
}
func TestBackupObservationIgnoresOldAmbiguousAndSymlinkFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "jellyfin-backup-old.zip")
	os.WriteFile(old, []byte("old"), 0600)
	probe := newBackup(dir)
	if _, ok := probe(); ok {
		t.Fatal("old archive reported")
	}
	a := filepath.Join(dir, "jellyfin-backup-a.zip")
	os.WriteFile(a, []byte("123"), 0600)
	if s, ok := probe(); !ok || s.bytes != 3 {
		t.Fatal("new archive missing")
	}
	os.WriteFile(a, []byte("123456"), 0600)
	if s, ok := probe(); !ok || s.bytes != 6 {
		t.Fatal("growth missing")
	}
	b := filepath.Join(dir, "jellyfin-backup-b.zip")
	os.WriteFile(b, []byte("second"), 0600)
	if _, ok := probe(); ok {
		t.Fatal("ambiguous progress reported")
	}
	os.Remove(b)
	os.Symlink(old, b)
	if s, ok := probe(); !ok || s.bytes != 6 {
		t.Fatal("symlink included")
	}
}
func TestObserverReportsGrowthButPollingIsNotActivity(t *testing.T) {
	m := &Manager{}
	m.setStep("backup", 2, "backup")
	var mu sync.Mutex
	sample := fileSample{69, time.Now()}
	probe := func() (fileSample, bool) { mu.Lock(); defer mu.Unlock(); return sample, true }
	stop := m.watchFiles(context.Background(), probe, "Sicherung")
	deadline := time.Now().Add(3 * time.Second)
	for !m.State().Progress.Measured && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	first := m.State().Progress
	if first.Bytes != 69 || !first.Measured {
		t.Fatal(first)
	}
	for i := 0; i < 100; i++ {
		_ = m.State()
	}
	if m.State().Progress.LastActivityAt != first.LastActivityAt {
		t.Fatal("polling invented activity")
	}
	mu.Lock()
	sample = fileSample{4096, time.Now()}
	mu.Unlock()
	stop() // also takes the final measurement and joins the observer
	got := m.State().Progress
	if got.Bytes != 4096 || !got.LastActivityAt.After(first.LastActivityAt) {
		t.Fatal(got)
	}
	m.setStep("importing", 3, "next")
	time.Sleep(10 * time.Millisecond)
	if m.State().Progress.Measured {
		t.Fatal("old observer changed next step")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	end := m.watchFiles(ctx, probe, "cancelled")
	end()
}
func TestBackupModePreflight(t *testing.T) {
	for _, mode := range []string{`"Pessimistic"`, `1`, `"NoLock"`, `0`, `"Optimistic"`, `2`} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/System/Configuration/database" {
					t.Error("preflight mutated source")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"LockingBehavior":` + mode + `}`))
			}))
			defer srv.Close()
			api, _ := NewAPI(srv.URL)
			err := checkBackupMode(context.Background(), api)
			blocked := mode == `"Pessimistic"` || mode == `1`
			if (err != nil) != blocked || calls != 1 {
				t.Fatal(mode, err, calls)
			}
			if err != nil && !strings.Contains(err.Error(), "keine Sicherung gestartet") {
				t.Fatal(err)
			}
		})
	}
}
func TestStateReturnsProgressAndCancellationIsNotNetworkFailure(t *testing.T) {
	m := &Manager{Options: Options{Origin: "http://127.0.0.1:18594"}}
	m.setStep("backup", 2, "Sicherung")
	handler, err := m.Handler()
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:18594/api/state", nil))
	var state State
	if json.Unmarshal(w.Body.Bytes(), &state) != nil || state.Progress.Step != 2 || state.Progress.StartedAt.IsZero() {
		t.Fatal(w.Body.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	api, _ := NewAPI("http://127.0.0.1:1")
	if e := api.call(ctx, "GET", "/", nil, nil); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
