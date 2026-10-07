// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type Report struct {
	Source                            string `json:"source"`
	Users, Libraries, Items, UserData int
	Instance                          string    `json:"instance"`
	Verified                          bool      `json:"verified"`
	Notes                             []string  `json:"notes"`
	Finished                          time.Time `json:"finished"`
}
type State struct {
	Progress      Progress `json:"progress"`
	Ready         bool     `json:"ready"`
	SetupComplete bool     `json:"setupComplete"`
	NewSetup      bool     `json:"newSetup"`
	Phase         string   `json:"phase"`
	Message       string   `json:"message"`
	Report        *Report  `json:"report,omitempty"`
	Target        string   `json:"target"`
	Active        string   `json:"active"`
	// Restarts counts consecutive automatic Jellyfin relaunches in the current
	// unstable period; it returns to 0 after 60s of continuous readiness.
	Restarts int `json:"restarts"`
	// ConnectState is "" (Connect is not running and no start is in progress:
	// not configured, setup not finished, or the server is down), "starting"
	// (spawned, alive for less than 10s), "running" (alive for 10s) or
	// "failed" (still retried every minute). ConnectMessage explains "failed";
	// otherwise it is empty.
	ConnectState   string `json:"connectState"`
	ConnectMessage string `json:"connectMessage"`
}
type Manager struct {
	Options          Options
	mu               sync.Mutex
	change           sync.Mutex
	state            State
	child, connect   *process
	api              *API
	cancel           context.CancelFunc
	jobCancel        context.CancelFunc
	jobs             sync.WaitGroup
	token            string
	nativeOwnerToken string
	lock             *os.File
	connectSettings  []byte
	closing          bool
	// resumePhase and resumeMessage hold what the state showed before the
	// phase became "restarting" (idle, or an import outcome); they come back
	// once the relaunched server is ready. Guarded by mu.
	resumePhase, resumeMessage string
}

func NewManager(o Options) (*Manager, error) {
	if o.NativeOwnerToken != "" {
		secret, err := hex.DecodeString(o.NativeOwnerToken)
		host, _, listenErr := net.SplitHostPort(o.Listen)
		if runtime.GOOS != "darwin" || o.Container || o.Bind != "127.0.0.1" || listenErr != nil || host != "127.0.0.1" || err != nil || len(secret) != 32 {
			return nil, errors.New("Die native Importfreigabe benötigt die lokale Mac-App.")
		}
	}
	nativeOwnerToken := o.NativeOwnerToken
	o.NativeOwnerToken = ""

	if o.Container {
		if runtime.GOOS != "linux" {
			return nil, errors.New("Container-Modus ist nur im Docker-Paket erlaubt.")
		}
		if _, e := os.Stat("/.dockerenv"); e != nil {
			return nil, errors.New("Container-Modus benötigt eine Docker-Umgebung.")
		}
	}
	if o.Bind != "127.0.0.1" && !o.Container {
		return nil, errors.New("Außerhalb des Docker-Pakets ist ausschließlich Loopback erlaubt.")
	}
	root, e := filepath.Abs(o.Root)
	if e != nil {
		return nil, e
	}
	o.Root = root
	if e = os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	lock, e := os.OpenFile(filepath.Join(root, "manager.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		lock.Close()
		return nil, errors.New("Mutti wird bereits verwaltet.")
	}
	success := false
	defer func() {
		if !success {
			lock.Close()
		}
	}()
	a, e := NewAPI(o.Backend)
	if e != nil {
		lock.Close()
		return nil, e
	}
	target, e := NewAPI(o.TargetOrigin)
	if e != nil {
		lock.Close()
		return nil, e
	}
	a.Host = target.Base.Host
	m := &Manager{Options: o, nativeOwnerToken: nativeOwnerToken, token: randomID(), lock: lock, api: a, state: State{Phase: "idle", Target: o.TargetOrigin + "/web/", Active: root}}
	if b, e := os.ReadFile(filepath.Join(root, "active-instance.json")); e == nil {
		var pointer struct{ ID string }
		if json.Unmarshal(b, &pointer) != nil || !validID(pointer.ID) {
			return nil, errors.New("Die aktive Importinstanz ist ungültig.")
		}
		instance := filepath.Join(root, "instances", pointer.ID)
		b, e = os.ReadFile(filepath.Join(instance, "report.json"))
		var report Report
		if e != nil || json.Unmarshal(b, &report) != nil || !report.Verified {
			return nil, errors.New("Für die importierte Instanz fehlt der Prüfnachweis.")
		}
		m.state.Active = instance
		m.state.Report = &report
	}
	if _, e := os.Stat(filepath.Join(m.state.Active, "setup-new")); e == nil {
		m.state.NewSetup = true
	}
	success = true
	return m, nil
}
func validID(id string) bool {
	if len(id) != 48 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state
	s.Progress = s.Progress.at(time.Now())
	return s
}

// applyChildLocked publishes a supervision decision; m.mu must be held.
func (m *Manager) applyChildLocked(a childAction, restarts int, now time.Time) {
	switch a {
	case childRestarting:
		if m.state.Phase != "restarting" {
			// A failed relaunch reports restarting again; only the first notice
			// of an unstable period replaces something worth bringing back, such
			// as the explanation of a failed import.
			m.resumePhase, m.resumeMessage = m.state.Phase, m.state.Message
		}
		m.state.Phase, m.state.Message, m.state.Restarts = "restarting", restartingMessage, restarts
	case childGiveUp:
		m.state.Phase, m.state.Message = "error", restartFailedMessage
		m.state.Progress.FinishedAt = now
	case childRecovered:
		// Never clobber an import phase or report; only our own notice goes
		// away, and what it had replaced comes back.
		if m.state.Phase == "restarting" {
			m.state.Phase, m.state.Message = m.resumePhase, m.resumeMessage
		}
	case childStable:
		m.state.Restarts = 0
	}
}
func (m *Manager) launch(root string) error {
	_, port, e := net.SplitHostPort(m.api.Base.Host)
	if e != nil {
		return e
	}
	n, e := strconv.Atoi(port)
	if e != nil {
		return e
	}
	child, e := m.Options.startServer(root, n, m.api.Host, "", false)
	if e != nil {
		return e
	}
	m.child = child
	return nil
}
func (m *Manager) Run(ctx context.Context) error {
	ctx, m.cancel = context.WithCancel(ctx)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer func() {
		m.cancel()
		m.mu.Lock()
		m.closing = true
		if m.jobCancel != nil {
			m.jobCancel()
		}
		m.mu.Unlock()
		m.jobs.Wait()
		m.change.Lock()
		m.connect.stop()
		m.child.stop()
		m.change.Unlock()
		syscall.Flock(int(m.lock.Fd()), syscall.LOCK_UN)
		m.lock.Close()
	}()
	if e := m.launch(m.state.Active); e != nil {
		return e
	}
	child := newChildSupervisor()
	connect := newConnectSupervisor()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		if ctx.Err() != nil {
			return nil
		}
		m.change.Lock()
		now := time.Now()
		requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		var info PublicInfo
		e := m.api.call(requestCtx, "GET", "/System/Info/Public", nil, &info)
		cancel()
		running := m.child.running()
		ready := e == nil && info.Id != "" && running
		if ready {
			healthCtx, healthCancel := context.WithTimeout(ctx, 2*time.Second)
			ready = m.api.call(healthCtx, "GET", "/Users/Public", nil, nil) == nil
			healthCancel()
		}
		m.mu.Lock()
		m.state.Ready = ready
		m.state.SetupComplete = ready && info.StartupWizardCompleted
		active := m.state.Active
		// The import job owns the child while it runs. Decide under the same lock
		// StartImport takes, so no job can begin between the check and the phase.
		action := child.observe(now, running, ready, m.jobCancel != nil)
		m.applyChildLocked(action, child.restarts(), now)
		m.mu.Unlock()
		if !running {
			m.connect.stop()
			m.connect = nil
			connect.stopped()
		}
		if action == childRelaunch {
			if e := m.launch(active); e != nil {
				m.mu.Lock()
				m.applyChildLocked(child.failed(now), child.restarts(), now)
				m.mu.Unlock()
			}
		}
		if ready && info.StartupWizardCompleted && m.Options.Connect != "" {
			m.superviseConnect(connect, active, now)
		}
		if m.Options.Connect != "" {
			m.mu.Lock()
			m.state.ConnectState, m.state.ConnectMessage = connect.state, connect.message
			m.mu.Unlock()
		}
		m.change.Unlock()
	}
}

// superviseConnect keeps the Connect helper running next to a ready server.
// Settings changes restart it deliberately; its own exits and spawn errors go
// through the backoff so a taken port never becomes a tight respawn loop and
// never fails the server itself. m.change must be held.
func (m *Manager) superviseConnect(s *connectSupervisor, active string, now time.Time) {
	settings, _ := os.ReadFile(filepath.Join(active, "connect-settings.json"))
	if m.connect.running() {
		if string(settings) == string(m.connectSettings) {
			s.alive(now)
			return
		}
		m.connect.stop()
		m.connect = nil
		s.stopped()
	} else if m.connect != nil {
		m.connect = nil
		s.exited(now)
	}
	if !s.due(now) {
		return
	}
	var cs struct{ Broker, Stun string }
	_ = json.Unmarshal(settings, &cs)
	args := []string{"--state", filepath.Join(active, "connect"), "--listen", m.Options.ConnectListen, "--admin-origin", m.Options.ConnectOrigin, "--target", m.Options.Backend, "--target-host", m.api.Host}
	if cs.Broker != "" {
		args = append(args, "--broker", cs.Broker)
	}
	if cs.Stun != "" {
		args = append(args, "--stun", cs.Stun)
	}
	m.connectSettings = settings
	c, e := startProcess(m.Options.Connect, args, os.Environ(), filepath.Join(active, "logs", "connect.log"))
	if e != nil {
		s.exited(now)
		return
	}
	m.connect = c
	s.started(now)
}
func (m *Manager) StartImport(ctx context.Context, input SourceInput) error {
	m.mu.Lock()
	if m.jobCancel != nil || m.closing {
		m.mu.Unlock()
		return errors.New("Eine Übernahme läuft bereits.")
	}
	if m.state.Phase == "restarting" {
		// The supervisor is about to relaunch the child; a job would race it.
		m.mu.Unlock()
		return errors.New("Der Server startet gerade neu. Bitte kurz warten.")
	}
	jobCtx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	m.jobCancel = cancel
	m.state.Report = nil
	now := time.Now()
	m.state.Progress = Progress{StartedAt: now, StepStartedAt: now, LastActivityAt: now, Step: 1}
	m.state.Phase = "checking"
	m.state.Message = "Jellyfin und Zugriffsrechte werden geprüft …"
	m.jobs.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.jobs.Done()
		defer cancel()
		e := m.importSource(jobCtx, input)
		m.mu.Lock()
		m.jobCancel = nil
		if e != nil {
			if errors.Is(jobCtx.Err(), context.DeadlineExceeded) {
				e = errors.New("Die Übernahme hat das Zeitlimit von 45 Minuten erreicht. Es wurde nicht umgeschaltet. Jellyfin kann die angeforderte Sicherung noch weiterführen; bitte vor einem neuen Versuch seinen Status prüfen.")
			} else if jobCtx.Err() != nil {
				e = errors.New("Übernahme abgebrochen. Deine bisherige Einrichtung bleibt erhalten. Eine bereits angeforderte Jellyfin-Sicherung kann auf der Quelle weiterlaufen.")
			}
			m.state.Phase = "error"
			m.state.Message = e.Error()
			m.state.Progress.FinishedAt = time.Now()
		}
		m.mu.Unlock()
	}()
	return nil
}
func (m *Manager) importSource(ctx context.Context, input SourceInput) error {
	m.setStep("checking", 1, "Jellyfin und Zugriffsrechte werden geprüft …")
	var current PublicInfo
	if e := m.api.call(ctx, "GET", "/System/Info/Public", nil, &current); e != nil {
		return e
	}
	if current.StartupWizardCompleted {
		if !input.Replace {
			return errors.New("Bitte bestätigen, dass du zu einer importierten Bibliothek wechseln möchtest. Der bisherige Mutti-Datenstand bleibt erhalten.")
		}
		// The Mac app already owns this private data directory and child process.
		// Its per-launch capability authorizes a retained-data switch after a native
		// confirmation. Browser/Docker requests still require the existing admin.
		if !input.nativeOwner {
			owner, _ := NewAPI(m.Options.Backend)
			owner.Host = m.api.Host
			_, e := login(ctx, owner, input.TargetUsername, input.TargetPassword)
			owner.logout()
			if e != nil {
				return errors.New("Bitte mit dem Administrator der bestehenden Bibliothek anmelden oder den Import direkt in der Mutti-Mac-App bestätigen. Ein zusätzliches Mutti-Konto wird nicht benötigt.")
			}
		}
	}
	s, e := m.openImportSource(ctx, input, current.Id)
	if e != nil {
		return e
	}
	defer s.API.logout()
	if s.Local {
		sourceRoot, err := filepath.EvalSymlinks(s.Info.ProgramDataPath)
		targetRoot, _ := filepath.EvalSymlinks(m.Options.Root)
		if err == nil && (sourceRoot == targetRoot || within(targetRoot, sourceRoot) || within(sourceRoot, targetRoot)) {
			return errors.New("Die Quelle und Mutti müssen getrennte Datenordner verwenden.")
		}
	}
	var plugins []PluginInfo
	if e = s.API.call(ctx, "GET", "/Plugins", nil, &plugins); e != nil {
		return e
	}
	hasIntro, e := qualifyPlugins(plugins)
	if e != nil {
		return e
	}
	if hasIntro && m.Options.IntroSkipper == "" {
		return errors.New("Für Intro Skipper bitte das vollständige Mutti-Paket verwenden.")
	}
	for _, library := range s.Libraries {
		for _, location := range library.Locations {
			p := replacePath(location, input.Mappings)
			st, e := os.Stat(p)
			if e != nil || !st.IsDir() {
				return fmt.Errorf("Der Medienordner %s ist hier nicht erreichbar. Ordnerzuordnung ergänzen und erneut versuchen.", location)
			}
		}
	}
	id := randomID()
	instance := filepath.Join(m.Options.Root, "instances", id)
	if e = os.MkdirAll(instance, 0700); e != nil {
		return e
	}
	m.setStep("backup", 2, "Jellyfin sichert Benutzer, Bibliotheken und Metadaten …")
	var archive string
	if input.Archive != "" {
		return errors.New("Bitte den automatischen Import verwenden. Archivdateien werden nur nach Quellenprüfung angenommen.")
	}
	archive, e = m.sourceBackup(ctx, s, instance)
	if e != nil {
		return e
	}
	var intro introSnapshot
	if hasIntro {
		m.setStep("importing", 3, "Intro-Skipper-Einstellungen und Analysedaten werden gesichert …")
		intro, e = m.Options.sourceIntro(ctx, s, archive, filepath.Join(instance, "intro-source"))
		if e != nil {
			return e
		}
	}
	m.setStep("importing", 3, "Sicherung wird für Mutti vorbereitet …")
	port, e := freePort()
	if e != nil {
		return e
	}
	prepared := filepath.Join(instance, "prepared.zip")
	stopProgress := m.watchFiles(ctx, fixedFile(prepared), "Vorbereitete Sicherung")
	audit, e := TransformArchive(archive, prepared, instance, port, s.Info, input.Mappings, m.Options.FFmpeg)
	stopProgress()
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return errors.New("Übernahme abgebrochen. Der bisherige Datenstand bleibt erhalten.")
	}
	m.setStep("importing", 4, "Benutzer, Bibliotheken und Wiedergabestand werden in Mutti wiederhergestellt …")
	validationOptions := m.Options
	validationOptions.Bind = "127.0.0.1"
	// Jellyfin's restore purges existing tables; initialize a fresh schema first.
	bootstrap, e := validationOptions.startServer(instance, port, fmt.Sprintf("127.0.0.1:%d", port), "", true)
	if e != nil {
		return e
	}
	staged, _ := NewAPI(fmt.Sprintf("http://127.0.0.1:%d", port))
	timeout, cancel := context.WithTimeout(ctx, 90*time.Second)
	_, e = waitServer(timeout, bootstrap, staged)
	if e == nil {
		for {
			e = staged.call(timeout, "GET", "/Startup/User", nil, nil)
			if e == nil || timeout.Err() != nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	cancel()
	bootstrap.stop()
	if e != nil {
		return e
	}
	var expectedIntro map[string]string
	if hasIntro {
		expectedIntro, e = restoreIntro(intro, instance, input.Mappings)
		if e != nil {
			return e
		}
	}
	p, e := validationOptions.startServer(instance, port, fmt.Sprintf("127.0.0.1:%d", port), prepared, true)
	if e != nil {
		return e
	}
	defer p.stop()
	timeout, cancel = context.WithTimeout(ctx, 5*time.Minute)
	info, e := waitServer(timeout, p, staged)
	cancel()
	if e != nil {
		return e
	}
	if !info.StartupWizardCompleted {
		return errors.New("Die importierte Einrichtung wurde nicht korrekt wiederhergestellt.")
	}
	m.setStep("verifying", 5, "Benutzerzugänge, Bibliotheken und Intro Skipper werden geprüft …")
	if _, e = login(ctx, staged, s.Username, s.Password); e != nil {
		return errors.New("Der bestehende Benutzerzugang funktioniert in der importierten Instanz nicht. Es wurde nicht umgeschaltet.")
	}
	defer staged.logout()
	if m.Options.IntroSkipper != "" {
		if e = verifyIntroLoaded(ctx, staged); e != nil {
			return e
		}
	}
	var libraries []Library
	if e = staged.call(ctx, "GET", "/Library/VirtualFolders", nil, &libraries); e != nil {
		return e
	}
	if e = compareLibraries(s.Libraries, libraries, importPathMappings(s.Info, instance, input.Mappings)); e != nil {
		return e
	}
	m.setStep("verifying", 5, "Die übernommene Datenbank wird mit der Sicherung verglichen …")
	stopProgress = m.watchFiles(ctx, newBackup(filepath.Join(instance, "data", "data", "backups")), "Prüfsicherung")
	var verifiedBackup struct{ Path string }
	e = staged.call(ctx, "POST", "/Backup/Create", map[string]bool{"Database": true}, &verifiedBackup)
	stopProgress()
	if e != nil {
		return e
	}
	if !within(instance, verifiedBackup.Path) {
		return errors.New("Ungültiger Prüfpfad.")
	}
	actual, e := ReadAudit(verifiedBackup.Path)
	if e != nil {
		return e
	}
	if e = CompareAudit(audit, actual); e != nil {
		return e
	}
	if e = VerifyFiles(audit, instance); e != nil {
		return e
	}
	staged.logout()
	p.stop()
	if hasIntro {
		restored, err := m.Options.snapshotIntro(ctx, filepath.Join(instance, "data"), filepath.Join(instance, "intro-verified"))
		if err != nil {
			return err
		}
		if e = compareIntro(expectedIntro, restored.Hashes, false); e != nil {
			return e
		}
	}
	// Refuse activation if the source changed while the snapshot was checked.
	// This catches new playback, favorites, settings and library changes.
	m.setStep("verifying", 6, "Jellyfin erstellt eine zweite Sicherung zur Abschlussprüfung …")
	var finalArchive string
	if s.Local && locallyReadable(s.Info.ProgramDataPath) {
		finalArchive, e = m.sourceBackup(ctx, s, instance)
	} else {
		_ = os.Remove(filepath.Join(instance, "source.zip"))
		finalArchive, e = m.sourceBackup(ctx, s, instance)
	}
	if e != nil {
		return e
	}
	if hasIntro {
		latestIntro, err := m.Options.sourceIntro(ctx, s, finalArchive, filepath.Join(instance, "intro-final"))
		if err != nil {
			return err
		}
		if e = compareIntro(intro.Hashes, latestIntro.Hashes, true); e != nil {
			return e
		}
	}
	finalPrepared := filepath.Join(instance, "source-final.zip")
	m.setStep("verifying", 6, "Die Abschlussprüfung sucht nach Änderungen während des Umzugs …")
	stopProgress = m.watchFiles(ctx, fixedFile(finalPrepared), "Abschließend geprüfte Sicherung")
	latest, e := TransformArchive(finalArchive, finalPrepared, instance, port, s.Info, input.Mappings, m.Options.FFmpeg)
	stopProgress()
	if e != nil {
		return e
	}
	_ = os.Remove(finalPrepared)
	if e = CompareAudit(audit, latest); e != nil {
		return errors.New("Jellyfin wurde während des Umzugs verändert. Bitte die Wiedergabe pausieren und die Übernahme erneut starten. Es wurde nicht umgeschaltet.")
	}
	if len(audit.Files) != len(latest.Files) {
		return errors.New("Die Quelldateien haben sich während des Umzugs geändert. Bitte erneut versuchen.")
	}
	if len(audit.Settings) != len(latest.Settings) {
		return errors.New("Die Quelleinstellungen wurden während des Imports verändert. Bitte erneut versuchen.")
	}
	for name, hash := range audit.Settings {
		if latest.Settings[name] != hash {
			return errors.New("Die Quelleinstellungen wurden während des Imports verändert. Bitte erneut versuchen.")
		}
	}
	for name, hash := range audit.Files {
		if latest.Files[name] != hash {
			return errors.New("Die Quelldateien haben sich während des Umzugs geändert. Bitte erneut versuchen.")
		}
	}
	report := Report{Source: s.Info.ServerName, Users: audit.Counts["Users"], Libraries: len(libraries), Items: audit.Counts["BaseItems"], UserData: audit.Counts["UserData"], Instance: id, Verified: true, Finished: time.Now(), Notes: []string{"Benutzerzugänge, Rechte, Bibliotheken und Wiedergabestand wurden verglichen.", "Alte Gerätesitzungen und API-Schlüssel werden nicht übernommen; Geräte neu koppeln.", "Netzwerkzugang und Transcoding sind auf Mutti angepasst. Hardwarebeschleunigung bei Bedarf erneut wählen.", "Jellyfin bleibt erhalten. Ab jetzt bitte Mutti verwenden; spätere Änderungen auf Jellyfin werden nicht synchronisiert."}}
	if m.Options.IntroSkipper != "" {
		note := "Intro Skipper ist enthalten und erkennt künftig Intros und Abspann."
		if hasIntro {
			note = "Intro Skipper einschließlich Einstellungen und vorhandener Analysedaten wurde geprüft übernommen."
		}
		report.Notes = append(report.Notes, note)
	}
	b, _ := json.Marshal(report)
	if e = privateWrite(filepath.Join(instance, "report.json"), b); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return errors.New("Übernahme abgebrochen. Es wurde nicht umgeschaltet.")
	}
	m.setStep("activating", 7, "Die geprüfte Bibliothek wird aktiviert …")
	m.change.Lock()
	defer m.change.Unlock()
	old := m.State().Active
	if settings, err := os.ReadFile(filepath.Join(old, "connect-settings.json")); err == nil {
		if err = privateWrite(filepath.Join(instance, "connect-settings.json"), settings); err != nil {
			return err
		}
	}
	m.connect.stop()
	m.connect = nil
	m.child.stop()
	if e = m.launch(instance); e != nil {
		_ = m.launch(old)
		return e
	}
	timeout, cancel = context.WithTimeout(ctx, 90*time.Second)
	_, e = waitServer(timeout, m.child, m.api)
	cancel()
	if e != nil {
		m.child.stop()
		_ = m.launch(old)
		return errors.New("Die Aktivierung ist fehlgeschlagen. Der bisherige Mutti-Datenstand wurde wieder gestartet.")
	}
	b, _ = json.Marshal(map[string]string{"ID": id})
	if e = privateWrite(filepath.Join(m.Options.Root, "active-instance.json"), b); e != nil {
		m.child.stop()
		_ = m.launch(old)
		return e
	}
	m.mu.Lock()
	m.state.Active = instance
	m.state.Report = &report
	m.state.Progress.FinishedAt = time.Now()
	m.state.Phase = "complete"
	m.state.Message = "Deine Jellyfin-Bibliothek ist jetzt in Mutti bereit."
	m.state.NewSetup = false
	m.mu.Unlock()
	// Only generated staging artifacts are removed, never the source backup/data.
	_ = os.Remove(prepared)
	_ = os.Remove(filepath.Join(instance, "source.zip"))
	_ = os.Remove(verifiedBackup.Path)
	return nil
}
func compareLibraries(source, target []Library, mappings map[string]string) error {
	fail := func(detail string) error {
		return fmt.Errorf("Die Bibliothekszuordnung konnte nicht vollständig bestätigt werden: %s Der bisherige Datenstand bleibt aktiv.", detail)
	}
	if len(source) != len(target) {
		return fail(fmt.Sprintf("Erwartet wurden %d Bibliotheken, gefunden wurden %d.", len(source), len(target)))
	}
	byID := make(map[string]Library, len(target))
	for _, library := range target {
		if _, duplicate := byID[library.ItemId]; duplicate || library.ItemId == "" {
			return fail("Eine übernommene Bibliothek hat keine eindeutige Kennung.")
		}
		byID[library.ItemId] = library
	}
	for _, library := range source {
		restored, found := byID[library.ItemId]
		if !found {
			return fail(fmt.Sprintf("Die ursprüngliche Kennung der Bibliothek %q fehlt.", library.Name))
		}
		delete(byID, library.ItemId)
		if library.Name != restored.Name {
			return fail(fmt.Sprintf("Der Name der Bibliothek %q hat sich geändert.", library.Name))
		}
		expected := make([]string, len(library.Locations))
		for i, path := range library.Locations {
			expected[i] = replacePath(path, mappings)
		}
		actual := append([]string{}, restored.Locations...)
		sort.Strings(expected)
		sort.Strings(actual)
		if !slices.Equal(expected, actual) {
			return fail(fmt.Sprintf("Die Ordner der Bibliothek %q stimmen nicht mit der vorgesehenen Übernahme überein.", library.Name))
		}
	}
	return nil
}
func remoteBackup(ctx context.Context, s *Source, instance string) (string, error) {
	var job struct{ Id, Secret string }
	if e := s.API.call(ctx, "POST", "/MuttiExport/Begin", map[string]string{"Recipient": s.API.Device}, &job); e != nil {
		return "", fmt.Errorf("Der Jellyfin-Export konnte nicht bereitgestellt werden: %w Für entfernte Server muss der mitgelieferte Mutti-Umzugshelfer installiert und Jellyfin danach neu gestartet sein.", e)
	}
	if !validID(job.Id) || len(job.Secret) != 64 {
		return "", errors.New("Ungültige Antwort des Umzugshelfers.")
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.API.call(c, "POST", "/MuttiExport/Finish", job, nil)
	}()
	r, e := s.API.request(ctx, "POST", "/MuttiExport/Download", job)
	if e != nil {
		return "", e
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return "", errors.New("Der geschützte Export wurde abgelehnt.")
	}
	path := filepath.Join(instance, "source.zip")
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	defer f.Close()
	n, e := io.Copy(f, io.LimitReader(r.Body, (20<<30)+1))
	if e != nil || n > 20<<30 {
		return "", errors.New("Der Export wurde abgebrochen oder ist zu groß.")
	}
	return path, f.Sync()
}

func locallyReadable(path string) bool {
	st, e := os.Stat(path)
	return filepath.IsAbs(path) && e == nil && st.IsDir()
}
