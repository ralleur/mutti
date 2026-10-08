// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"strings"
	"testing"
	"time"
)

// Regressions from the review of the supervisor/update-guard merge (2026-10-07).

func TestReplacedPackageHoldsTheSupervisor(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m := &Manager{state: State{Phase: "idle"}}
	child := newChildSupervisor()
	tick := func(replaced bool) childAction {
		now = now.Add(time.Second)
		m.notePackageLocked(replaced)
		a := child.observe(now, false, false, replaced)
		m.applyChildLocked(a, child.restarts(), now)
		return a
	}
	if tick(false); m.state.Phase != "restarting" {
		t.Fatalf("exit not announced: %q", m.state.Phase)
	}
	// The package is replaced meanwhile: no relaunch is attempted or counted,
	// the restart notice gives way and the replacement is reported instead.
	for i := 0; i < 600; i++ {
		if a := tick(true); a != childNone {
			t.Fatalf("tick %d with a replaced package: action %v", i, a)
		}
	}
	if m.state.Phase != "idle" || m.state.ServiceMessage != errPackageReplaced.Error() || child.restarts() != 1 {
		t.Fatalf("replaced package: phase %q service %q restarts %d", m.state.Phase, m.state.ServiceMessage, child.restarts())
	}
	// The admitted package is back: the message goes and Jellyfin is relaunched.
	if a := tick(false); a != childRelaunch || m.state.ServiceMessage != "" {
		t.Fatalf("package restored: action %v service %q", a, m.state.ServiceMessage)
	}
}

func TestFailedRelaunchesFailAPendingUpdate(t *testing.T) {
	f := newUpdateFixture(t)
	f.populate()
	if err := f.m.prepareStart(); err != nil {
		t.Fatal(err)
	}
	f.m.verifyUpdate("12.1.0", true)
	f.packageBuild("12.1", "commit-b")
	if err := f.m.prepareStart(); err != nil || f.m.State().Update.State != "pending" {
		t.Fatalf("no pending update: %v", err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	child := newChildSupervisor()
	child.observe(now, false, false, false) // the new server exited
	for i := 0; i < 20 && f.m.State().Phase != "error"; i++ {
		f.m.relaunchFailed(child, f.m.State().Update, now) // every relaunch fails to spawn
	}
	u := f.m.State().Update
	if f.m.State().Phase != "error" || u.State != "failed" || !strings.Contains(u.Message, "lässt sich nicht starten") {
		t.Fatalf("phase %q update %+v", f.m.State().Phase, u)
	}
}
