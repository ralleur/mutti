// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Only package-owned snapshots are accepted. These are local recovery copies,
// not media backups or a promise of protection from loss of the host disk.
type BackupSummary struct {
	ID         string     `json:"id"`
	Created    time.Time  `json:"created"`
	VerifiedAt *time.Time `json:"verifiedAt,omitempty"`
	Bytes      int64      `json:"bytes"`
	Version    string     `json:"version"`
}
type storedBackup struct {
	BackupSummary
	Schema    int
	Info      SystemInfo
	Libraries []Library
	Intro     bool
	Hub       bool
	Hashes    map[string]string
	directory string
}
type MaintenanceJob struct {
	Kind     string     `json:"kind"`
	State    string     `json:"state"`
	Message  string     `json:"message"`
	BackupID string     `json:"backupId,omitempty"`
	Started  time.Time  `json:"started"`
	Finished *time.Time `json:"finished,omitempty"`
}
type MaintenanceState struct {
	Job       MaintenanceJob  `json:"job"`
	Backups   []BackupSummary `json:"backups"`
	Directory string          `json:"directory"`
	Busy      bool            `json:"busy"`
}

func fileHash(path string) (string, int64, error) {
	st, e := os.Lstat(path)
	if e != nil {
		return "", 0, e
	}
	if !st.Mode().IsRegular() || st.Size() > 100<<30 {
		return "", 0, errors.New("Ungültige Sicherungsdatei.")
	}
	f, e := os.Open(path)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, e
}
func (m *Manager) backupDirectory() string { return filepath.Join(m.Options.Root, "backups") }
func (m *Manager) readBackup(id string) (*storedBackup, error) {
	if !validID(id) {
		return nil, errors.New("Ungültige Sicherung.")
	}
	dir := filepath.Join(m.backupDirectory(), id)
	st, e := os.Lstat(dir)
	if e != nil || !st.IsDir() {
		return nil, errors.New("Sicherung nicht gefunden.")
	}
	st, e = os.Lstat(filepath.Join(dir, "backup.json"))
	if e != nil || !st.Mode().IsRegular() || st.Size() > 2<<20 {
		return nil, errors.New("Ungültiger Sicherungsnachweis.")
	}
	b, e := os.ReadFile(filepath.Join(dir, "backup.json"))
	var s storedBackup
	if e != nil || json.Unmarshal(b, &s) != nil || s.Schema != 1 || s.ID != id || !strings.HasPrefix(s.Version, "12.1.") || s.Hashes["library.zip"] == "" {
		return nil, errors.New("Sicherung ist unvollständig oder nicht mit diesem Paket kompatibel.")
	}
	for name, expected := range s.Hashes {
		if name != "library.zip" && name != "intro/snapshot.json" && !(strings.HasPrefix(name, "intro/") && introFiles[strings.TrimPrefix(name, "intro/")]) && !hubBackupName.MatchString(name) {
			return nil, errors.New("Unbekannte Datei in der Sicherung.")
		}
		// Reject symlinked parent directories as well as symlinked files.
		resolved, err := filepath.EvalSymlinks(filepath.Join(dir, name))
		if err != nil || resolved != filepath.Join(dir, name) {
			return nil, errors.New("Unsicherer Sicherungspfad.")
		}
		hash, _, err := fileHash(resolved)
		if err != nil || hash != expected {
			return nil, errors.New("Die Prüfsumme der Sicherung stimmt nicht. Es wurde nichts umgeschaltet.")
		}
	}
	if s.Intro {
		intro, err := readIntroSnapshot(filepath.Join(dir, "intro"))
		if err != nil || s.Hashes["intro/snapshot.json"] == "" {
			return nil, errors.New("Die Plugin-Sicherung fehlt.")
		}
		for name := range intro.Hashes {
			if s.Hashes["intro/"+name] == "" {
				return nil, errors.New("Die Plugin-Sicherung ist unvollständig.")
			}
		}
	}
	if s.Hub && s.Hashes["hub/hub.json"] == "" {
		return nil, errors.New("Die Moduldaten der Sicherung sind unvollständig.")
	}
	s.directory = dir
	return &s, nil
}

func (m *Manager) MaintenanceState() MaintenanceState {
	m.mu.Lock()
	s := MaintenanceState{Job: m.maintenance, Busy: m.jobCancel != nil, Backups: []BackupSummary{}, Directory: m.backupDirectory()}
	m.mu.Unlock()
	// Listing reads only manifests; an integrity check happens before every restore.
	entries, _ := os.ReadDir(s.Directory)
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) {
			continue
		}
		path := filepath.Join(s.Directory, entry.Name(), "backup.json")
		st, err := os.Lstat(path)
		if err != nil || !st.Mode().IsRegular() || st.Size() > 2<<20 {
			continue
		}
		b, err := os.ReadFile(path)
		var backup storedBackup
		if err == nil && json.Unmarshal(b, &backup) == nil && backup.Schema == 1 && backup.ID == entry.Name() {
			s.Backups = append(s.Backups, backup.BackupSummary)
		}
	}
	sort.Slice(s.Backups, func(i, j int) bool { return s.Backups[i].Created.After(s.Backups[j].Created) })
	return s
}

func (m *Manager) maintenanceOwner(ctx context.Context, token string) (*API, error) {
	a, e := NewAPI(m.Options.Backend)
	if e != nil {
		return nil, e
	}
	a.Host, a.Token = m.api.Host, token
	var user User
	if len(token) < 20 || a.call(ctx, "GET", "/Users/Me", nil, &user) != nil || !user.Policy.IsAdministrator || user.Policy.IsDisabled {
		a.Client.CloseIdleConnections()
		return nil, errors.New("Aktive Besitzeranmeldung erforderlich.")
	}
	return a, nil
}

func (m *Manager) maintenanceHandler(w http.ResponseWriter, r *http.Request) {
	a, err := m.maintenanceOwner(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	defer a.Client.CloseIdleConnections()
	op := r.PathValue("operation")
	if op == "state" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m.MaintenanceState())
		return
	}
	if op != "backup" && op != "verify" && op != "restore" {
		http.NotFound(w, r)
		return
	}
	var input struct {
		ID, Username, Password string
		Confirm                bool
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil {
		http.Error(w, "Ungültige Anfrage.", 400)
		return
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF || len(input.Username) > 128 || len(input.Password) > 1024 {
		http.Error(w, "Ungültige Anfrage.", 400)
		return
	}
	if op != "backup" && (!validID(input.ID) || input.Username == "" || !input.Confirm) {
		http.Error(w, "Bitte Sicherung und Benutzerzugang prüfen und den Vorgang bestätigen.", 400)
		return
	}
	err = m.startMaintenance(op, input.ID, func(ctx context.Context) (string, error) {
		if op == "backup" {
			return m.createBackup(ctx, a)
		}
		backup, err := m.readBackup(input.ID)
		if err != nil {
			return "", err
		}
		err = m.importSource(ctx, SourceInput{Replace: true, targetAuthorized: true, Username: input.Username, Password: input.Password, backup: backup, verifyOnly: op == "verify", authorizeActivation: func(ctx context.Context) error {
			current, err := m.maintenanceOwner(ctx, a.Token)
			if current != nil {
				current.Client.CloseIdleConnections()
			}
			return err
		}})
		if err != nil {
			return "", err
		}
		now := time.Now().UTC()
		backup.VerifiedAt = &now
		b, _ := json.Marshal(backup)
		if err = privateWrite(filepath.Join(backup.directory, "backup.json"), b); err != nil {
			return input.ID, errors.New("Wiederherstellung geprüft, aber der Nachweis konnte nicht gespeichert werden.")
		}
		return input.ID, nil
	})
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (m *Manager) startMaintenance(kind, id string, work func(context.Context) (string, error)) error {
	m.mu.Lock()
	if m.closing || m.jobCancel != nil || !m.state.Ready || !m.state.SetupComplete {
		m.mu.Unlock()
		return errors.New("Bitte Einrichtung oder laufenden Vorgang abwarten.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	m.jobCancel = cancel
	m.maintenance = MaintenanceJob{Kind: kind, State: "running", BackupID: id, Started: time.Now().UTC(), Message: "Vorgang läuft. Du kannst diese Ansicht später erneut öffnen."}
	b, _ := json.Marshal(m.maintenance)
	if err := privateWrite(filepath.Join(m.Options.Root, "maintenance-job.json"), b); err != nil {
		m.jobCancel = nil
		cancel()
		m.mu.Unlock()
		return err
	}
	m.jobs.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.jobs.Done()
		defer cancel()
		result, err := work(ctx)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.jobCancel = nil
		now := time.Now().UTC()
		m.maintenance.Finished = &now
		if err != nil {
			m.maintenance.State = "failed"
			m.maintenance.Message = err.Error()
		} else {
			m.maintenance.State = "completed"
			m.maintenance.BackupID = result
			switch kind {
			case "backup":
				m.maintenance.Message = "Lokale Sicherung erstellt und Prüfsummen kontrolliert. Die Wiederherstellungsprobe steht noch aus."
			case "verify":
				m.maintenance.Message = "Wiederherstellung in einer getrennten Instanz geprüft. Aktive Daten unverändert."
			case "restore":
				m.maintenance.Message = "Sicherung wiederhergestellt. Bitte erneut anmelden und Geräte neu koppeln."
			}
		}
		b, _ := json.Marshal(m.maintenance)
		_ = privateWrite(filepath.Join(m.Options.Root, "maintenance-job.json"), b)
	}()
	return nil
}

func (m *Manager) createBackup(ctx context.Context, a *API) (string, error) {
	s := &Source{API: a, Local: true}
	if err := a.call(ctx, "GET", "/System/Info", nil, &s.Info); err != nil {
		return "", err
	}
	if err := a.call(ctx, "GET", "/Library/VirtualFolders", nil, &s.Libraries); err != nil {
		return "", err
	}
	var plugins []PluginInfo
	if err := a.call(ctx, "GET", "/Plugins", nil, &plugins); err != nil {
		return "", err
	}
	intro, err := qualifyPlugins(plugins)
	if err != nil {
		return "", err
	}
	active := m.State().Active
	if !within(active, s.Info.ProgramDataPath) {
		return "", errors.New("Die Daten gehören nicht zur verwalteten Instanz.")
	}
	id := randomID()
	dir := filepath.Join(m.backupDirectory(), id)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(dir)
		}
	}()
	// Existing online backup API creates consistent database snapshots. No raw DB copy.
	archive, err := s.Backup(ctx)
	if err != nil {
		return "", err
	}
	if !within(active, archive) {
		return "", errors.New("Ungültiger Sicherungspfad.")
	}
	in, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	out, err := os.OpenFile(filepath.Join(dir, "library.zip"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		in.Close()
		return "", err
	}
	_, err = io.Copy(out, in)
	in.Close()
	syncErr := out.Sync()
	closeErr := out.Close()
	if err != nil {
		return "", err
	}
	if syncErr != nil {
		return "", syncErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if _, err = ReadAudit(filepath.Join(dir, "library.zip")); err != nil {
		return "", err
	}
	paths := []string{"library.zip"}
	if intro {
		snapshot, e := m.Options.snapshotIntro(ctx, s.Info.ProgramDataPath, filepath.Join(dir, "intro"))
		if e != nil {
			return "", e
		}
		paths = append(paths, "intro/snapshot.json")
		for name := range snapshot.Hashes {
			paths = append(paths, "intro/"+name)
		}
	}
	hubFiles, err := m.snapshotHub(filepath.Join(dir, "hub"))
	if err != nil {
		return "", errors.New("Die Moduldaten konnten nicht gesichert werden.")
	}
	paths = append(paths, hubFiles...)
	backup := storedBackup{Schema: 1, BackupSummary: BackupSummary{ID: id, Created: time.Now().UTC(), Version: s.Info.Version}, Info: s.Info, Libraries: s.Libraries, Intro: intro, Hub: len(hubFiles) > 0, Hashes: map[string]string{}}
	for _, name := range paths {
		h, n, e := fileHash(filepath.Join(dir, name))
		if e != nil {
			return "", e
		}
		backup.Hashes[name] = h
		backup.Bytes += n
	}
	b, _ := json.Marshal(backup)
	if err = privateWrite(filepath.Join(dir, "backup.json"), b); err != nil {
		return "", err
	}
	if _, err = m.readBackup(id); err != nil {
		return "", err
	}
	complete = true
	return id, nil
}
