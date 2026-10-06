// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// ActionEntry records one confirmed action. The intent is written durably
// before any side effect; an entry still "executing" after a restart becomes
// "outcome_unknown" and is never repeated automatically.
type ActionEntry struct {
	ID            string         `json:"id"` // proposal ID, also the idempotency key
	Profile       string         `json:"profile"`
	Device        string         `json:"device,omitempty"`
	Task          string         `json:"task"`
	Target        contentKey     `json:"target"`
	ContentID     string         `json:"contentId,omitempty"`
	Revision      string         `json:"revision,omitempty"`
	Title         string         `json:"title,omitempty"`
	Args          map[string]any `json:"args"`
	Qualification string         `json:"qualification"`
	State         string         `json:"state"` // executing, confirmed, failed, outcome_unknown
	Result        string         `json:"result,omitempty"`
	Created       time.Time      `json:"created"`
	Updated       time.Time      `json:"updated"`
}

type actionJournal struct {
	mu      sync.Mutex
	path    string
	entries []*ActionEntry
	targets map[contentKey]*sync.Mutex
}

type journalFile struct {
	Version int            `json:"version"`
	Entries []*ActionEntry `json:"entries"`
}

// journalLimit bounds the file; only finished entries are dropped, oldest first.
const journalLimit = 500

func openJournal(dir string) (*actionJournal, error) {
	j := &actionJournal{path: filepath.Join(dir, "actions.json"), targets: map[contentKey]*sync.Mutex{}}
	b, err := os.ReadFile(j.path)
	switch {
	case os.IsNotExist(err):
		return j, nil
	case err != nil:
		return nil, err
	}
	var f journalFile
	if err = json.Unmarshal(b, &f); err != nil || f.Version != 1 {
		return nil, errors.New("invalid action journal")
	}
	j.entries = f.Entries
	changed := false
	for _, e := range j.entries {
		if e.State == "executing" {
			e.State, e.Result, e.Updated = "outcome_unknown", "Mutti wurde während der Ausführung beendet. Bitte das Ergebnis in der Bibliothek prüfen.", time.Now().UTC()
			changed = true
		}
	}
	if changed {
		if err = j.persist(); err != nil {
			return nil, err
		}
	}
	return j, nil
}

func (j *actionJournal) persist() error {
	return writePrivateJSON(j.path, journalFile{Version: 1, Entries: j.entries})
}

func (j *actionJournal) find(id string) *ActionEntry {
	for _, e := range j.entries {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// get returns a copy of an existing entry.
func (j *actionJournal) get(id string) (ActionEntry, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if e := j.find(id); e != nil {
		return *e, true
	}
	return ActionEntry{}, false
}

// begin durably records the intent. It fails if the ID is already known or
// the intent cannot be stored; then no side effect may happen.
func (j *actionJournal) begin(e ActionEntry) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.find(e.ID) != nil {
		return errors.New("action already recorded")
	}
	now := time.Now().UTC()
	e.State, e.Created, e.Updated = "executing", now, now
	j.entries = append(j.entries, &e)
	j.prune()
	if err := j.persist(); err != nil {
		j.entries = slices.DeleteFunc(j.entries, func(x *ActionEntry) bool { return x.ID == e.ID })
		return err
	}
	return nil
}

func (j *actionJournal) finish(id, state, result string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	e := j.find(id)
	if e == nil {
		return errors.New("unknown action")
	}
	e.State, e.Result, e.Updated = state, result, time.Now().UTC()
	return j.persist()
}

func (j *actionJournal) prune() {
	for len(j.entries) > journalLimit {
		i := slices.IndexFunc(j.entries, func(e *ActionEntry) bool { return e.State != "executing" })
		if i < 0 {
			return
		}
		j.entries = slices.Delete(j.entries, i, i+1)
	}
}

// list returns the profile's newest entries first.
func (j *actionJournal) list(profile string, n int) []ActionEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := []ActionEntry{}
	for i := len(j.entries) - 1; i >= 0 && len(out) < n; i-- {
		if j.entries[i].Profile == profile {
			out = append(out, *j.entries[i])
		}
	}
	return out
}

// lockTarget serializes conflicting operations on one object across
// conversations and devices.
func (j *actionJournal) lockTarget(key contentKey) func() {
	j.mu.Lock()
	l := j.targets[key]
	if l == nil {
		l = &sync.Mutex{}
		j.targets[key] = l
	}
	j.mu.Unlock()
	l.Lock()
	return l.Unlock
}
