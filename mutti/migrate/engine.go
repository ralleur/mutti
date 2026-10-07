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
	"strings"
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
	Progress       Progress `json:"progress"`
	Ready          bool     `json:"ready"`
	SetupComplete  bool     `json:"setupComplete"`
	NewSetup       bool     `json:"newSetup"`
	Phase          string   `json:"phase"`
	Message        string   `json:"message"`
	Report         *Report  `json:"report,omitempty"`
	Target         string   `json:"target"`
	Active         string   `json:"active"`
	ServiceMessage string   `json:"serviceMessage,omitempty"`
	// Update describes a pending, verified, failed or blocked server update.
	Update *UpdateState `json:"update,omitempty"`
}
type Manager struct {
	Options Options
	mu      sync.Mutex
	change  sync.Mutex
	// admitted is the package version prepareStart accepted for this run.
	admitted         dataVersion
	state            State
	child, connect   *process
	hub              *process
	hubPeer          string
	hubStarted       time.Time
	api              *API
	cancel           context.CancelFunc
	jobCancel        context.CancelFunc
	jobs             sync.WaitGroup
	token            string
	nativeOwnerToken string
	lock             *os.File
	connectSettings  []byte
	closing          bool
	maintenance      MaintenanceJob
	recovery         recoveryBudget
	// unblock wakes a start that waits after a blocked update (rollback).
	unblock        chan struct{}
	updateLaunched time.Time
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
	m := &Manager{Options: o, nativeOwnerToken: nativeOwnerToken, token: randomID(), hubPeer: randomID(), lock: lock, api: a, state: State{Phase: "idle", Target: o.TargetOrigin + "/web/", Active: root},
		unblock: make(chan struct{}, 1)}
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
	if b, err := os.ReadFile(filepath.Join(root, "maintenance-job.json")); err == nil {
		_ = json.Unmarshal(b, &m.maintenance)
		if m.maintenance.State == "running" {
			m.maintenance.State = "interrupted"
			m.maintenance.Message = "Der letzte Wartungsvorgang wurde durch einen Neustart unterbrochen. Bitte den aktiven Datenstand prüfen, bevor du erneut startest."
		}
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
func (m *Manager) setPhase(phase, message string) {
	m.mu.Lock()
	m.state.Phase = phase
	m.state.Message = message
	if phase == "error" {
		m.state.Progress.FinishedAt = time.Now()
	}
	m.mu.Unlock()
}

// errPackageReplaced: the package changed while Mutti runs; nothing of it
// is started until Mutti is opened again and checks it.
var errPackageReplaced = errors.New("Das Mutti-Paket wurde während des Betriebs ersetzt. Bitte Mutti beenden und neu öffnen; bis dahin wird nichts neu gestartet.")

// packageReplaced compares the package with the version admitted at start
// (cheap: version files and the component list digest).
func (m *Manager) packageReplaced() bool {
	m.mu.Lock()
	admitted := m.admitted
	m.mu.Unlock()
	if admitted.Jellyfin == "" {
		return false
	}
	now, ok := m.Options.packageVersion()
	return !ok || now.Jellyfin != admitted.Jellyfin || now.Build != admitted.Build || now.Components != admitted.Components
}

func (m *Manager) launch(root string) error {
	if m.packageReplaced() {
		return errPackageReplaced
	}
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
		m.hub.stop()
		m.child.stop()
		m.change.Unlock()
		syscall.Flock(int(m.lock.Fd()), syscall.LOCK_UN)
		m.lock.Close()
	}()
	// A different server build only starts after a snapshot; an older build
	// never starts on data a newer one may have migrated.
	for {
		e := m.prepareStart()
		if e == nil {
			break
		}
		if !errors.Is(e, errUpdateBlocked) {
			return e
		}
		select {
		case <-ctx.Done():
			return nil
		case <-m.unblock:
		}
	}
	m.updateLaunched = time.Now()
	if e := m.launch(m.State().Active); e != nil {
		return e
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		m.change.Lock()
		requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		var info PublicInfo
		e := m.api.call(requestCtx, "GET", "/System/Info/Public", nil, &info)
		cancel()
		ready := e == nil && info.Id != "" && m.child.running()
		if ready {
			healthCtx, healthCancel := context.WithTimeout(ctx, 2*time.Second)
			ready = m.api.call(healthCtx, "GET", "/Users/Public", nil, nil) == nil
			healthCancel()
		}
		m.mu.Lock()
		m.state.Ready = ready
		m.state.SetupComplete = ready && info.StartupWizardCompleted
		active := m.state.Active
		update := m.state.Update
		m.mu.Unlock()
		if update != nil && update.State == "pending" {
			if ready {
				m.verifyUpdate(info.Version, info.StartupWizardCompleted)
			} else if time.Since(m.updateLaunched) > 20*time.Minute {
				m.failUpdate(update, "der neue Server ist nach 20 Minuten nicht bereit")
			}
		}
		if !m.child.running() {
			m.connect.stop()
			m.connect = nil
			message := "Mutti wurde unerwartet beendet. Automatischer Wiederanlauf wartet; bei wiederholtem Fehler bitte das Paket und freien Speicher prüfen."
			if m.recovery.allow(time.Now()) {
				if e := m.launch(active); e == nil {
					message = "Mutti wird nach einem unerwarteten Ende neu gestartet …"
				} else if errors.Is(e, errPackageReplaced) {
					message = e.Error()
				}
			} else if update != nil && update.State == "pending" {
				// Repeated crashes right after an update: report it as failed
				// with the way back instead of retrying silently.
				m.failUpdate(update, "der neue Server beendet sich wiederholt")
			}
			m.mu.Lock()
			m.state.ServiceMessage = message
			m.mu.Unlock()
		} else if ready {
			m.mu.Lock()
			m.state.ServiceMessage = ""
			m.mu.Unlock()
		}
		replaced := m.packageReplaced()
		if replaced {
			m.mu.Lock()
			m.state.ServiceMessage = errPackageReplaced.Error()
			m.mu.Unlock()
		}
		if !replaced && ready && info.StartupWizardCompleted && m.Options.Hub != "" && !m.hub.running() && time.Since(m.hubStarted) > 10*time.Second {
			// Optional modules run in their own process: a failing module never
			// stops Jellyfin, Connect or playback and is retried with a pause.
			m.hubStarted = time.Now()
			m.hub, e = m.startHub()
			if e != nil {
				m.hub = nil
			}
		}
		if !replaced && ready && info.StartupWizardCompleted && m.Options.Connect != "" {
			settings, _ := os.ReadFile(filepath.Join(active, "connect-settings.json"))
			if !m.connect.running() || string(settings) != string(m.connectSettings) {
				m.connect.stop()
				m.connect = nil
				var cs struct{ Broker, Stun string }
				_ = json.Unmarshal(settings, &cs)
				args := []string{"--state", filepath.Join(active, "connect"), "--listen", m.Options.ConnectListen, "--admin-origin", m.Options.ConnectOrigin, "--target", m.Options.Backend, "--target-host", m.api.Host}
				env := os.Environ()
				if m.Options.Hub != "" {
					args = append(args, "--hub", "http://"+m.Options.HubListen)
					env = append(env, "MUTTI_HUB_PEER="+m.hubPeer)
				}
				if cs.Broker != "" {
					args = append(args, "--broker", cs.Broker)
				}
				if cs.Stun != "" {
					args = append(args, "--stun", cs.Stun)
				}
				m.connect, e = startProcess(m.Options.Connect, args, env, filepath.Join(active, "logs", "connect.log"))
				if e != nil {
					m.setPhase("error", "Die Geräteverbindung konnte nicht gestartet werden.")
				}
				m.connectSettings = settings
			}
		}
		m.change.Unlock()
	}
}

// hubDirectory is shared by all instances so that an import or restore of the
// media library does not discard module settings or conversations.
func (m *Manager) hubDirectory() string { return filepath.Join(m.Options.Root, "hub") }

func (m *Manager) startHub() (*process, error) {
	args := []string{"--state", m.hubDirectory(), "--listen", m.Options.HubListen, "--jellyfin", m.Options.Backend, "--jellyfin-host", m.api.Host}
	if m.Options.Ollama != "" {
		args = append(args, "--ollama", m.Options.Ollama)
	}
	env := []string{"MUTTI_HUB_PEER=" + m.hubPeer, "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "HOME=") || strings.HasPrefix(v, "TMPDIR=") || strings.HasPrefix(v, "TZ=") {
			env = append(env, v)
		}
	}
	return startProcess(m.Options.Hub, args, env, filepath.Join(m.Options.Root, "logs", "hub.log"))
}

func (m *Manager) StartImport(ctx context.Context, input SourceInput) error {
	m.mu.Lock()
	if m.jobCancel != nil || m.closing {
		m.mu.Unlock()
		return errors.New("Eine Übernahme läuft bereits.")
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
		if !input.nativeOwner && !input.targetAuthorized {
			owner, _ := NewAPI(m.Options.Backend)
			owner.Host = m.api.Host
			_, e := login(ctx, owner, input.TargetUsername, input.TargetPassword)
			owner.logout()
			if e != nil {
				return errors.New("Bitte mit dem Administrator der bestehenden Bibliothek anmelden oder den Import direkt in der Mutti-Mac-App bestätigen. Ein zusätzliches Mutti-Konto wird nicht benötigt.")
			}
		}
	}
	var s *Source
	var e error
	if input.backup != nil {
		a, _ := NewAPI(m.Options.Backend)
		s = &Source{API: a, Info: input.backup.Info, Libraries: input.backup.Libraries, Username: input.Username, Password: input.Password}
	} else {
		s, e = m.openImportSource(ctx, input, current.Id)
		if e != nil {
			return e
		}
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
	if input.backup == nil {
		if e = s.API.call(ctx, "GET", "/Plugins", nil, &plugins); e != nil {
			return e
		}
	}
	hasIntro, e := qualifyPlugins(plugins)
	if input.backup != nil {
		hasIntro = input.backup.Intro
	}
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
	if input.backup != nil {
		defer func() {
			if m.State().Active != instance {
				_ = os.RemoveAll(instance)
			}
		}()
	}
	m.setStep("backup", 2, "Jellyfin sichert Benutzer, Bibliotheken und Metadaten …")
	var archive string
	if input.Archive != "" {
		return errors.New("Bitte den automatischen Import verwenden. Archivdateien werden nur nach Quellenprüfung angenommen.")
	}
	if input.backup != nil {
		archive = filepath.Join(input.backup.directory, "library.zip")
	} else {
		archive, e = m.sourceBackup(ctx, s, instance)
	}
	if e != nil {
		return e
	}
	var intro introSnapshot
	if hasIntro {
		m.setStep("importing", 3, "Intro-Skipper-Einstellungen und Analysedaten werden gesichert …")
		if input.backup != nil {
			intro, e = readIntroSnapshot(filepath.Join(input.backup.directory, "intro"))
		} else {
			intro, e = m.Options.sourceIntro(ctx, s, archive, filepath.Join(instance, "intro-source"))
		}
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
	if input.backup == nil {
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
	} else if _, e = m.readBackup(input.backup.ID); e != nil {
		return e
	}
	report := Report{Source: s.Info.ServerName, Users: audit.Counts["Users"], Libraries: len(libraries), Items: audit.Counts["BaseItems"], UserData: audit.Counts["UserData"], Instance: id, Verified: true, Finished: time.Now(), Notes: []string{"Benutzerzugänge, Rechte, Bibliotheken und Wiedergabestand wurden verglichen.", "Alte Gerätesitzungen und API-Schlüssel werden nicht übernommen; Geräte neu koppeln.", "Netzwerkzugang und Transcoding sind auf Mutti angepasst. Hardwarebeschleunigung bei Bedarf erneut wählen.", "Jellyfin bleibt erhalten. Ab jetzt bitte Mutti verwenden; spätere Änderungen auf Jellyfin werden nicht synchronisiert."}}
	if m.Options.IntroSkipper != "" {
		note := "Intro Skipper ist enthalten und erkennt künftig Intros und Abspann."
		if hasIntro {
			note = "Intro Skipper einschließlich Einstellungen und vorhandener Analysedaten wurde geprüft übernommen."
		}
		report.Notes = append(report.Notes, note)
	}
	if input.backup != nil {
		report.Source = "Mutti-Sicherung " + input.backup.ID
		report.Notes = []string{"Sicherung und wiederhergestellte Daten wurden verglichen.", "Medienoriginale bleiben an ihren bisherigen Speicherorten und müssen separat gesichert werden.", "Alte Gerätesitzungen und API-Schlüssel werden nicht wiederhergestellt. Geräte bitte neu koppeln."}
		if input.verifyOnly {
			m.setStep("complete", 7, "Wiederherstellung in einer getrennten Testinstanz geprüft. Deine aktive Bibliothek bleibt unverändert.")
			// This directory is exclusively generated by this verification run.
			return os.RemoveAll(instance)
		}
	}
	b, _ := json.Marshal(report)
	if e = privateWrite(filepath.Join(instance, "report.json"), b); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return errors.New("Übernahme abgebrochen. Es wurde nicht umgeschaltet.")
	}
	if input.authorizeActivation != nil {
		if e = input.authorizeActivation(ctx); e != nil {
			return e
		}
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
	if input.backup != nil {
		m.state.Message = "Sicherung wiederhergestellt. Bitte erneut anmelden und Geräte neu koppeln."
	}
	m.state.NewSetup = false
	m.mu.Unlock()
	if input.backup != nil {
		if err := m.restoreHub(input.backup); err != nil {
			m.mu.Lock()
			m.state.Message = "Bibliothek wiederhergestellt. " + err.Error()
			m.mu.Unlock()
		}
	}
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
