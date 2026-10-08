// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Progress reports observed work, not an estimated completion percentage.
// Times are sampled on the server so browser clock skew cannot change durations.
type Progress struct {
	StartedAt      time.Time `json:"startedAt"`
	StepStartedAt  time.Time `json:"stepStartedAt"`
	LastActivityAt time.Time `json:"lastActivityAt"`
	FinishedAt     time.Time `json:"finishedAt"`
	Step           int       `json:"step"`
	ElapsedSeconds int64     `json:"elapsedSeconds"`
	StepSeconds    int64     `json:"stepSeconds"`
	IdleSeconds    int64     `json:"idleSeconds"`
	Bytes          int64     `json:"bytes"`
	Measured       bool      `json:"measured"`
	Measurement    string    `json:"measurement"`
}

func (p Progress) at(now time.Time) Progress {
	if p.StartedAt.IsZero() {
		return p
	}
	if !p.FinishedAt.IsZero() {
		now = p.FinishedAt
	}
	p.ElapsedSeconds = max(0, int64(now.Sub(p.StartedAt).Seconds()))
	if !p.StepStartedAt.IsZero() {
		p.StepSeconds = max(0, int64(now.Sub(p.StepStartedAt).Seconds()))
	}
	if !p.LastActivityAt.IsZero() {
		p.IdleSeconds = max(0, int64(now.Sub(p.LastActivityAt).Seconds()))
	}
	return p
}
func (m *Manager) setStep(phase string, step int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	p := &m.state.Progress
	if p.StartedAt.IsZero() {
		p.StartedAt = now
	}
	if p.Step != step {
		p.StepStartedAt = now
	}
	p.Step, p.LastActivityAt = step, now
	p.Bytes, p.Measured, p.Measurement = 0, false, ""
	m.state.Phase, m.state.Message = phase, message
}

type fileSample struct {
	bytes    int64
	modified time.Time
}
type fileProbe func() (fileSample, bool)

func sampleFile(path string) (fileSample, bool) {
	st, e := os.Lstat(path)
	if e != nil || !st.Mode().IsRegular() {
		return fileSample{}, false
	}
	return fileSample{st.Size(), st.ModTime()}, true
}
func fixedFile(path string) fileProbe {
	return func() (fileSample, bool) { return sampleFile(path) }
}

// Only attribute a newly created/changed archive when it is unambiguous.
// An unrelated old ZIP or two concurrent backups must not look like our progress.
func newBackup(dir string) fileProbe {
	scan := func() map[string]fileSample {
		found := map[string]fileSample{}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "jellyfin-backup-") && strings.HasSuffix(e.Name(), ".zip") {
				if s, ok := sampleFile(filepath.Join(dir, e.Name())); ok {
					found[e.Name()] = s
				}
			}
		}
		return found
	}
	before := scan()
	return func() (fileSample, bool) {
		var result fileSample
		n := 0
		for name, s := range scan() {
			if old, ok := before[name]; !ok || old != s {
				result = s
				n++
			}
		}
		return result, n == 1
	}
}

// The caller joins this observer before moving to the next step. HTTP polling
// does not update LastActivityAt: only a changed observed file does.
func (m *Manager) watchFiles(ctx context.Context, probe fileProbe, label string) func() {
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var previous fileSample
		observed := false
		sample := func() {
			current, ok := probe()
			m.mu.Lock()
			defer m.mu.Unlock()
			p := &m.state.Progress
			p.Measured = ok
			if ok {
				p.Bytes, p.Measurement = current.bytes, label
				if !observed || current != previous {
					p.LastActivityAt = time.Now()
				}
				previous, observed = current, true
			}
		}
		sample()
		for {
			select {
			case <-done:
				sample()
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				sample()
			}
		}
	}()
	return func() { close(done); <-stopped }
}
func (m *Manager) sourceBackup(ctx context.Context, s *Source, instance string) (string, error) {
	local := s.Local && locallyReadable(s.Info.ProgramDataPath)
	var probe fileProbe
	label := "Empfangene Sicherung"
	if local {
		probe = newBackup(filepath.Join(s.Info.ProgramDataPath, "data", "backups"))
		label = "Sicherung auf Jellyfin"
	} else {
		probe = fixedFile(filepath.Join(instance, "source.zip"))
	}
	stop := m.watchFiles(ctx, probe, label)
	defer stop()
	if local {
		return s.Backup(ctx)
	}
	return remoteBackup(ctx, s, instance)
}
