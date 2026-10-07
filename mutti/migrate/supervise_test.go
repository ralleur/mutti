// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"errors"
	"io"
	"testing"
	"time"
)

func TestBackoffSchedule(t *testing.T) {
	b := backoff{schedule: []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}}
	if b.exhausted() {
		t.Fatal("fresh backoff is exhausted")
	}
	for i, want := range b.schedule {
		if b.exhausted() {
			t.Fatalf("exhausted before failure %d", i+1)
		}
		if got := b.next(); got != want {
			t.Fatalf("failure %d: delay %v, want %v", i+1, got, want)
		}
		if b.failures != i+1 {
			t.Fatalf("failure %d: counter %d", i+1, b.failures)
		}
	}
	if !b.exhausted() {
		t.Fatal("not exhausted after the schedule")
	}
	// Past the end the last delay repeats and the counter saturates, so a
	// caller that never gives up keeps retrying at the longest interval.
	if got := b.next(); got != 10*time.Second || b.failures != 3 || !b.exhausted() {
		t.Fatal("saturation", got, b.failures, b.exhausted())
	}
	b.reset()
	if b.failures != 0 || b.exhausted() {
		t.Fatal("reset", b.failures, b.exhausted())
	}
	if got := b.next(); got != 2*time.Second {
		t.Fatal("after reset", got)
	}
	var empty backoff
	if empty.next() != 0 || !empty.exhausted() {
		t.Fatal("empty schedule")
	}
	if got, want := len(childSchedule), 6; got != want {
		t.Fatal("jellyfin schedule length", got)
	}
	if got, want := len(connectSchedule), 5; got != want {
		t.Fatal("connect schedule length", got)
	}
}

func TestWatchLifeline(t *testing.T) {
	for name, end := range map[string]func(*io.PipeWriter) error{
		"eof":   (*io.PipeWriter).Close,
		"error": func(w *io.PipeWriter) error { return w.CloseWithError(errors.New("parent died")) },
	} {
		t.Run(name, func(t *testing.T) {
			r, w := io.Pipe()
			closed := make(chan struct{}, 2)
			WatchLifeline(r, func() { closed <- struct{}{} })
			// Bytes after the token are drained and must not count as a close.
			if _, e := w.Write([]byte("still alive")); e != nil {
				t.Fatal(e)
			}
			select {
			case <-closed:
				t.Fatal("onClose while the pipe was open")
			case <-time.After(50 * time.Millisecond):
			}
			if e := end(w); e != nil {
				t.Fatal(e)
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("onClose not called after the pipe ended")
			}
			select {
			case <-closed:
				t.Fatal("onClose called twice")
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}

type childClock struct {
	t   *testing.T
	now time.Time
	s   *childSupervisor
}

func (c *childClock) tick(d time.Duration, running, ready, job bool, want childAction, what string) {
	c.t.Helper()
	c.now = c.now.Add(d)
	if got := c.s.observe(c.now, running, ready, job); got != want {
		c.t.Fatalf("%s: action %d, want %d", what, got, want)
	}
}
func (c *childClock) restarts(want int, what string) {
	c.t.Helper()
	if got := c.s.restarts(); got != want {
		c.t.Fatalf("%s: restarts %d, want %d", what, got, want)
	}
}

func TestChildSupervisorRestartsWithBackoff(t *testing.T) {
	c := &childClock{t: t, now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), s: newChildSupervisor()}
	// A booting and then healthy server produces no action.
	c.tick(0, true, false, false, childNone, "booting")
	c.tick(time.Second, true, true, false, childNone, "first ready")
	c.tick(time.Second, true, true, false, childNone, "steady")
	c.restarts(0, "healthy")
	// The first exit schedules a relaunch after 2s; the loop waits until then.
	c.tick(time.Second, false, false, false, childRestarting, "first exit")
	c.restarts(1, "first exit")
	if want := c.now.Add(2 * time.Second); !c.s.restartAt.Equal(want) {
		t.Fatal("restartAt", c.s.restartAt, want)
	}
	c.tick(time.Second, false, false, false, childNone, "waiting")
	c.tick(time.Second, false, false, false, childRelaunch, "due")
	// Readiness after the relaunch clears our notice exactly once.
	c.tick(time.Second, true, false, false, childNone, "booting after relaunch")
	c.tick(time.Second, true, true, false, childRecovered, "recovered")
	c.tick(time.Second, true, true, false, childNone, "no repeated recovery")
	// Another exit before 60s of readiness keeps counting: the delay grows to 5s.
	c.tick(30*time.Second, false, false, false, childRestarting, "second exit")
	c.restarts(2, "second exit")
	c.tick(4*time.Second, false, false, false, childNone, "waiting 5s")
	c.tick(time.Second, false, false, false, childRelaunch, "due after 5s")
	c.tick(time.Second, true, true, false, childRecovered, "recovered again")
	// A readiness hiccup restarts the stability clock.
	c.tick(30*time.Second, true, false, false, childNone, "hiccup")
	c.tick(time.Second, true, true, false, childNone, "ready after hiccup")
	c.tick(59*time.Second, true, true, false, childNone, "59s ready")
	c.restarts(2, "still unstable")
	c.tick(time.Second, true, true, false, childStable, "60s ready")
	c.restarts(0, "stable")
	c.tick(time.Second, true, true, false, childNone, "stable only once")
	// After a stable period the schedule starts over.
	c.tick(time.Second, false, false, false, childRestarting, "exit after stable")
	c.restarts(1, "exit after stable")
	if want := c.now.Add(2 * time.Second); !c.s.restartAt.Equal(want) {
		t.Fatal("restartAt after stable", c.s.restartAt, want)
	}
}

func TestChildSupervisorDefersToImportJob(t *testing.T) {
	c := &childClock{t: t, now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), s: newChildSupervisor()}
	c.tick(0, true, true, false, childNone, "ready")
	// The job stops and relaunches the child itself during activation.
	c.tick(time.Second, false, false, true, childNone, "exit during job")
	c.tick(time.Second, false, false, true, childNone, "still during job")
	c.restarts(0, "job owns the child")
	c.tick(time.Second, true, false, true, childNone, "job relaunched")
	c.tick(time.Second, true, true, true, childNone, "job instance ready, no recovery notice")
	// Only an exit outside a job is ours to handle.
	c.tick(time.Second, false, false, true, childNone, "second exit during job")
	c.tick(time.Second, false, false, false, childRestarting, "job finished with a dead child")
	c.restarts(1, "after job")
	c.tick(2*time.Second, false, false, false, childRelaunch, "due")
	// A relaunch by somebody else cancels a pending restart instead of
	// turning a later exit into an uncounted immediate relaunch.
	c.tick(time.Second, true, true, false, childRecovered, "recovered")
	c.tick(time.Second, false, false, false, childRestarting, "exit again")
	c.restarts(2, "counted again")
}

func TestChildSupervisorGivesUp(t *testing.T) {
	c := &childClock{t: t, now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), s: newChildSupervisor()}
	c.tick(0, true, true, false, childNone, "ready")
	for i, delay := range childSchedule {
		c.tick(time.Second, false, false, false, childRestarting, "exit")
		c.restarts(i+1, "exit")
		if want := c.now.Add(delay); !c.s.restartAt.Equal(want) {
			t.Fatalf("relaunch %d scheduled at %v, want %v", i+1, c.s.restartAt, want)
		}
		c.tick(delay-time.Second, false, false, false, childNone, "waiting")
		c.tick(time.Second, false, false, false, childRelaunch, "due")
		if i%2 == 0 {
			// The relaunched child dies before ever becoming ready.
			continue
		}
		// The relaunched child is ready briefly, then dies again.
		c.tick(time.Second, true, true, false, childRecovered, "recovered")
	}
	c.tick(time.Second, false, false, false, childGiveUp, "exhausted")
	c.restarts(len(childSchedule), "exhausted")
	for i := 0; i < 3; i++ {
		c.tick(time.Minute, false, false, false, childNone, "stays dead")
	}
	if c.s.restartAt != (time.Time{}) {
		t.Fatal("relaunch pending after giving up")
	}
}

func TestChildSupervisorLaunchFailure(t *testing.T) {
	c := &childClock{t: t, now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), s: newChildSupervisor()}
	c.tick(0, true, true, false, childNone, "ready")
	c.tick(time.Second, false, false, false, childRestarting, "exit")
	c.tick(2*time.Second, false, false, false, childRelaunch, "due")
	// Run() reports a failed launch; the next attempt waits for the next delay.
	if got := c.s.failed(c.now); got != childRestarting {
		t.Fatal("launch failure", got)
	}
	c.restarts(2, "launch failure counted")
	if want := c.now.Add(5 * time.Second); !c.s.restartAt.Equal(want) {
		t.Fatal("restartAt", c.s.restartAt, want)
	}
	c.tick(4*time.Second, false, false, false, childNone, "waiting")
	c.tick(time.Second, false, false, false, childRelaunch, "due again")
	c.tick(time.Second, true, true, false, childRecovered, "recovered")
}

func TestConnectSupervisor(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newConnectSupervisor()
	check := func(state, message string, what string) {
		t.Helper()
		if s.state != state || s.message != message {
			t.Fatalf("%s: state %q message %q, want %q %q", what, s.state, s.message, state, message)
		}
	}
	check("", "", "before the first start")
	if !s.due(now) {
		t.Fatal("first start not due")
	}
	// Five quick exits (ports taken) back off and end in a reported failure.
	for i, delay := range connectSchedule {
		s.started(now)
		if i < len(connectSchedule)-1 {
			check("starting", "", "started")
		}
		now = now.Add(time.Second)
		s.alive(now)
		now = now.Add(time.Second)
		s.exited(now)
		if !s.retryAt.Equal(now.Add(delay)) {
			t.Fatalf("quick failure %d: retry at %v, want %v", i+1, s.retryAt, now.Add(delay))
		}
		if s.due(now.Add(delay - time.Second)) {
			t.Fatalf("quick failure %d: retry due early", i+1)
		}
		now = now.Add(delay)
		if !s.due(now) {
			t.Fatalf("quick failure %d: retry not due", i+1)
		}
	}
	check("failed", connectFailedMessage, "exhausted")
	// Never give up: the next quick exit retries after the longest delay.
	s.started(now)
	check("failed", connectFailedMessage, "failure report survives a new attempt")
	now = now.Add(time.Second)
	s.exited(now)
	if !s.retryAt.Equal(now.Add(60 * time.Second)) {
		t.Fatal("retry after exhaustion", s.retryAt)
	}
	check("failed", connectFailedMessage, "still failed")
	// A spawn error counts as a quick failure too.
	now = now.Add(time.Minute)
	s.exited(now)
	if !s.retryAt.Equal(now.Add(60*time.Second)) || s.state != "failed" {
		t.Fatal("spawn error", s.retryAt, s.state)
	}
	// Ten seconds alive clears the report; sixty reset the counter.
	now = now.Add(time.Minute)
	s.started(now)
	s.alive(now.Add(9 * time.Second))
	check("failed", connectFailedMessage, "9s alive")
	s.alive(now.Add(10 * time.Second))
	check("running", "", "10s alive")
	if s.backoff.failures == 0 {
		t.Fatal("counter reset too early")
	}
	s.alive(now.Add(60 * time.Second))
	if s.backoff.failures != 0 {
		t.Fatal("counter not reset after 60s")
	}
	// An exit after a long run is not a quick failure: restart at once.
	now = now.Add(90 * time.Second)
	s.exited(now)
	check("starting", "", "late exit")
	if !s.due(now) || s.backoff.failures != 0 {
		t.Fatal("late exit", s.due(now), s.backoff.failures)
	}
	// Deliberate stops (server restart, settings change) are never failures.
	s.started(now)
	s.alive(now.Add(10 * time.Second))
	check("running", "", "running again")
	s.stopped()
	check("", "", "stopped deliberately: nothing is being started")
	if !s.due(now.Add(10*time.Second)) || s.backoff.failures != 0 {
		t.Fatal("deliberate stop", s.backoff.failures)
	}
	s.stopped()
	check("", "", "repeated stop while the server stays down")
	// A failure report waits for a surviving instance, not for a stop.
	s.state, s.message = "failed", connectFailedMessage
	s.stopped()
	check("failed", connectFailedMessage, "failure report survives a deliberate stop")
}

func TestApplyChildRestoresPhase(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m := &Manager{state: State{Phase: "idle"}}
	check := func(phase, message string, restarts int, what string) {
		t.Helper()
		if m.state.Phase != phase || m.state.Message != message || m.state.Restarts != restarts {
			t.Fatalf("%s: phase %q message %q restarts %d, want %q %q %d", what, m.state.Phase, m.state.Message, m.state.Restarts, phase, message, restarts)
		}
	}
	m.applyChildLocked(childNone, 0, now)
	check("idle", "", 0, "no action")
	// An idle server comes back idle.
	m.applyChildLocked(childRestarting, 1, now)
	check("restarting", restartingMessage, 1, "restarting from idle")
	m.applyChildLocked(childRecovered, 1, now)
	check("idle", "", 1, "recovered to idle")
	m.applyChildLocked(childStable, 0, now)
	check("idle", "", 0, "stable")
	// The explanation of a failed import survives a restart, also when the
	// first relaunch fails and the notice is reported again.
	m.state.Phase, m.state.Message = "error", "Der Medienordner fehlt."
	m.applyChildLocked(childRestarting, 1, now)
	check("restarting", restartingMessage, 1, "restarting after a failed import")
	m.applyChildLocked(childRestarting, 2, now)
	check("restarting", restartingMessage, 2, "relaunch failed")
	m.applyChildLocked(childRecovered, 2, now)
	check("error", "Der Medienordner fehlt.", 2, "import outcome restored")
	report := &Report{Verified: true}
	m.state.Phase, m.state.Message, m.state.Report = "complete", "Fertig.", report
	m.applyChildLocked(childRestarting, 3, now)
	m.applyChildLocked(childRecovered, 3, now)
	check("complete", "Fertig.", 3, "completed import restored")
	if m.state.Report != report {
		t.Fatal("report clobbered")
	}
	// A phase somebody else set in the meantime is never clobbered.
	m.applyChildLocked(childRestarting, 4, now)
	m.state.Phase, m.state.Message = "checking", "Jellyfin wird geprüft …"
	m.applyChildLocked(childRecovered, 4, now)
	check("checking", "Jellyfin wird geprüft …", 4, "foreign phase kept")
	// Giving up is an error of its own and ends the progress clock.
	m.state.Phase, m.state.Message = "idle", ""
	m.applyChildLocked(childRestarting, 6, now)
	m.applyChildLocked(childGiveUp, 6, now)
	check("error", restartFailedMessage, 6, "gave up")
	if !m.state.Progress.FinishedAt.Equal(now) {
		t.Fatal("FinishedAt", m.state.Progress.FinishedAt)
	}
}
