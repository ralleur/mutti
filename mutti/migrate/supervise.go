// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"io"
	"time"
)

// Supervision keeps the library usable without the user's attention: the
// Jellyfin child is relaunched with a backoff when it exits unexpectedly, the
// Connect helper is retried forever without ever failing the server, and the
// Mac app's stdin pipe doubles as a lifeline. The types here are pure state
// machines; Run() feeds observations in and performs the process work itself.

var (
	childSchedule   = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second}
	connectSchedule = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second}
)

const (
	stableAfter      = 60 * time.Second // Continuous readiness or uptime that ends an unstable period.
	connectQuickExit = 10 * time.Second // A Connect exit this soon after its start counts as a failure.
)

const (
	restartingMessage    = "Der Server wurde unerwartet beendet. Mutti startet ihn neu …"
	restartFailedMessage = "Der Server konnte nicht neu gestartet werden. Bitte Mutti erneut starten und das lokale Protokoll prüfen."
	connectFailedMessage = "Die Geräteverbindung konnte nicht gestartet werden. Prüfe, ob ein anderes Programm Port 18595 oder 18599 verwendet."
)

// backoff walks a fixed delay schedule. next records a failure and returns the
// delay before the following attempt. Past the end of the schedule the last
// delay repeats and the counter saturates, so callers decide via exhausted
// whether to keep going (Connect does) or to stop (Jellyfin does).
type backoff struct {
	schedule []time.Duration
	failures int
}

func (b *backoff) next() time.Duration {
	if len(b.schedule) == 0 {
		return 0
	}
	if b.failures < len(b.schedule) {
		b.failures++
	}
	return b.schedule[b.failures-1]
}
func (b *backoff) reset()          { b.failures = 0 }
func (b *backoff) exhausted() bool { return b.failures >= len(b.schedule) }

type childAction int

const (
	childNone       childAction = iota
	childRestarting             // The child exited; a relaunch is scheduled after the backoff delay.
	childRelaunch               // The delay elapsed; relaunch the child now.
	childGiveUp                 // Consecutive relaunches are exhausted; stop trying.
	childRecovered              // Readiness returned after a relaunch.
	childStable                 // Ready for stableAfter; the unstable period is over.
)

// childSupervisor decides when the Jellyfin child is relaunched. Every exit
// before the child has been ready for stableAfter counts as a consecutive
// failure; the import job owns the child while it runs and relaunches it
// itself during activation, so exits are not counted while a job runs.
type childSupervisor struct {
	backoff    backoff
	readySince time.Time // Zero while the child is not ready.
	restartAt  time.Time // Zero while no relaunch is pending.
	relaunched bool      // A relaunch happened and readiness has not returned since.
	gaveUp     bool
}

func newChildSupervisor() *childSupervisor {
	return &childSupervisor{backoff: backoff{schedule: childSchedule}}
}

// observe feeds one tick of the Run loop and returns at most one action.
func (s *childSupervisor) observe(now time.Time, running, ready, jobRunning bool) childAction {
	if running {
		// A live child makes any pending relaunch moot, whoever started it.
		s.restartAt = time.Time{}
		if !ready {
			s.readySince = time.Time{}
			return childNone
		}
		if s.readySince.IsZero() {
			s.readySince = now
			if s.relaunched {
				s.relaunched = false
				return childRecovered
			}
			return childNone
		}
		if now.Sub(s.readySince) >= stableAfter && s.backoff.failures > 0 {
			s.backoff.reset()
			s.gaveUp = false
			return childStable
		}
		return childNone
	}
	s.readySince = time.Time{}
	if s.gaveUp || jobRunning {
		return childNone
	}
	if s.restartAt.IsZero() {
		return s.failed(now)
	}
	if now.Before(s.restartAt) {
		return childNone
	}
	s.restartAt = time.Time{}
	s.relaunched = true
	return childRelaunch
}

// failed records an unexpected exit or a failed relaunch and schedules the
// next attempt, or gives up once the schedule is exhausted: every delay of the
// schedule is used for one relaunch, so the exit after the sixth relaunch is
// the one that gives up (restarts reaches six).
func (s *childSupervisor) failed(now time.Time) childAction {
	if s.backoff.exhausted() {
		s.gaveUp = true
		s.restartAt = time.Time{}
		return childGiveUp
	}
	s.restartAt = now.Add(s.backoff.next())
	return childRestarting
}

// restarts is the number of consecutive automatic relaunches in the current
// unstable period; it is published as State.Restarts.
func (s *childSupervisor) restarts() int { return s.backoff.failures }

// connectSupervisor tracks the Connect helper. A port that is already taken
// makes Connect exit at once; such quick exits back off and, once exhausted,
// are reported as State.ConnectState "failed" while the manager keeps trying
// every minute. The main phase is never touched: the library must keep working
// without device pairing.
type connectSupervisor struct {
	backoff   backoff
	startedAt time.Time // Zero while Connect is not running.
	retryAt   time.Time // Zero when the next start may happen at once.
	state     string    // "", "starting", "running" or "failed"; see State.ConnectState.
	message   string
}

func newConnectSupervisor() *connectSupervisor {
	return &connectSupervisor{backoff: backoff{schedule: connectSchedule}}
}

// started records a successful spawn at now. A "failed" report stays until
// Connect has proven itself alive.
func (s *connectSupervisor) started(now time.Time) {
	s.startedAt, s.retryAt = now, time.Time{}
	if s.state != "failed" {
		s.state, s.message = "starting", ""
	}
}

// alive is called on every tick while Connect is running.
func (s *connectSupervisor) alive(now time.Time) {
	up := now.Sub(s.startedAt)
	if up >= connectQuickExit {
		s.state, s.message = "running", ""
	}
	if up >= stableAfter {
		s.backoff.reset()
	}
}

// stopped records a deliberate stop (server down or settings change); it is
// never a failure. Nothing is being started now, so the state goes back to
// "" unless a failure report is still waiting for a surviving instance; a
// settings change starts the next instance in the same tick.
func (s *connectSupervisor) stopped() {
	s.startedAt = time.Time{}
	if s.state != "failed" {
		s.state, s.message = "", ""
	}
}

// exited records an exit or a failed spawn observed at now.
func (s *connectSupervisor) exited(now time.Time) {
	quick := s.startedAt.IsZero() || now.Sub(s.startedAt) < connectQuickExit
	s.startedAt = time.Time{}
	if !quick {
		s.retryAt = time.Time{}
		s.state, s.message = "starting", ""
		return
	}
	s.retryAt = now.Add(s.backoff.next())
	if s.backoff.exhausted() {
		s.state, s.message = "failed", connectFailedMessage
	} else {
		s.state, s.message = "starting", ""
	}
}

// due reports whether a start attempt may happen at now.
func (s *connectSupervisor) due(now time.Time) bool { return !now.Before(s.retryAt) }

// WatchLifeline drains r in the background and calls onClose exactly once when
// r ends, whether by EOF or by an error. The Mac app keeps the write end of the
// manager's stdin pipe open for as long as it lives, so the end of the pipe
// means the parent is gone and the manager must shut down with its children.
func WatchLifeline(r io.Reader, onClose func()) {
	go func() {
		_, _ = io.Copy(io.Discard, r)
		onClose()
	}()
}
