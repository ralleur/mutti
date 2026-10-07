// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Recoverable updates (P3). Jellyfin migrates its database when a newer
// server starts and has no general downgrade. Before a different server build
// touches the data, the manager therefore takes an offline snapshot while
// the server is stopped. An older build never starts on data a newer build
// may already have migrated; it offers the matching snapshot instead.

// dataVersion identifies the server build that owns the data stage.
type dataVersion struct {
	Schema   int    `json:"schema"`
	Jellyfin string `json:"jellyfin"` // packaged Jellyfin version, e.g. "12.1"
	Build    string `json:"build"`    // server source identity of the package
	Product  string `json:"product"`  // Mutti package version
	// Components is the digest of the package's component list; two
	// development builds of the same commit differ here. Empty for packages
	// without a list and data recorded before it existed.
	Components string    `json:"components,omitempty"`
	Signed     bool      `json:"signed,omitempty"`  // component list signed with a release key
	Channel    string    `json:"channel,omitempty"` // "release" requires a signed component list
	Recorded   time.Time `json:"recorded"`
}

func (v dataVersion) same(o dataVersion) bool {
	return v.Jellyfin == o.Jellyfin && v.Build == o.Build && (v.Components == "" || o.Components == "" || v.Components == o.Components)
}

func (v dataVersion) label() string {
	if v.Product != "" {
		return "Mutti " + v.Product + " (Jellyfin " + v.Jellyfin + ")"
	}
	return "Jellyfin " + v.Jellyfin
}

// UpdateState is shown to the owner; snapshots are listed by ID only.
type UpdateState struct {
	ID       string      `json:"id"`
	State    string      `json:"state"` // pending, verified, failed, blocked, rolled_back
	From     dataVersion `json:"from"`
	To       dataVersion `json:"to"`
	Snapshot string      `json:"snapshot,omitempty"`
	Started  time.Time   `json:"started"`
	Finished *time.Time  `json:"finished,omitempty"`
	Message  string      `json:"message,omitempty"`
}

type snapshotManifest struct {
	Schema  int               `json:"schema"`
	ID      string            `json:"id"`
	Created time.Time         `json:"created"`
	Version dataVersion       `json:"version"` // data stage captured (the version before the update)
	Active  string            `json:"active"`  // instance path relative to the root
	Hashes  map[string]string `json:"hashes"`  // relative path -> sha256
	Bytes   int64             `json:"bytes"`
}

// packageVersion reads the shipped component manifest next to the server.
// Without one (development runs) the update guard is not active.
func (o Options) resources() string { return filepath.Dir(filepath.Dir(o.Server)) }

func (o Options) packageVersion() (dataVersion, bool) {
	if o.Server == "" {
		return dataVersion{}, false
	}
	resources := o.resources()
	var lock struct {
		Version  string `json:"version"`
		Jellyfin struct {
			Version      string `json:"version"`
			ServerCommit string `json:"serverCommit"`
		} `json:"jellyfin"`
	}
	b, err := os.ReadFile(filepath.Join(resources, "components.lock.json"))
	if err != nil || json.Unmarshal(b, &lock) != nil || lock.Jellyfin.Version == "" {
		return dataVersion{}, false
	}
	v := dataVersion{Schema: 1, Jellyfin: lock.Jellyfin.Version, Build: lock.Jellyfin.ServerCommit, Product: lock.Version}
	// The package's own server commit distinguishes Mutti builds of the same
	// Jellyfin release; a dirty development tree gets a content-free marker.
	var provenance struct {
		Channel string `json:"channel"`
		Server  struct {
			Commit string `json:"commit"`
			Dirty  bool   `json:"dirty"`
		} `json:"server"`
	}
	if b, err = os.ReadFile(filepath.Join(resources, "build-provenance.json")); err == nil && json.Unmarshal(b, &provenance) == nil && provenance.Server.Commit != "" {
		v.Build = provenance.Server.Commit
		if provenance.Server.Dirty {
			v.Build += "+dirty"
		}
		v.Channel = provenance.Channel
	}
	if b, err = os.ReadFile(filepath.Join(resources, componentsFile)); err == nil {
		sum := sha256.Sum256(b)
		v.Components = hex.EncodeToString(sum[:])
	}
	return v, true
}

// checkPackage verifies the component set before the server starts. A
// damaged or modified package, or a release without a valid signature,
// blocks the start; nothing has changed yet.
func (m *Manager) checkPackage(v *dataVersion) error {
	m.setPhase("update", "Programmpaket wird geprüft …")
	defer m.setPhase("idle", "")
	check, err := VerifyComponents(m.Options.resources())
	switch {
	case errors.Is(err, os.ErrNotExist) && v.Channel != "release":
		return nil // development build without a component list
	case errors.Is(err, os.ErrNotExist):
		return errors.New("Dieses Mutti-Paket enthält keine Komponentenliste. Bitte Mutti neu installieren. Es wurde nichts verändert.")
	case err != nil:
		return errors.New("Das Mutti-Paket ist beschädigt oder verändert (" + err.Error() + "). Bitte Mutti neu installieren. Es wurde nichts verändert.")
	case v.Channel == "release" && !check.Signed:
		return errors.New("Dieses Mutti-Paket ist nicht mit dem Release-Schlüssel signiert. Bitte Mutti aus der offiziellen Quelle installieren. Es wurde nichts verändert.")
	}
	v.Components, v.Signed = check.Digest, check.Signed
	return nil
}

// compareVersions orders dotted numeric versions ("12.1" < "12.10").
func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func (m *Manager) dataVersionPath() string { return filepath.Join(m.Options.Root, "data-version.json") }
func (m *Manager) updatePath() string      { return filepath.Join(m.Options.Root, "update.json") }
func (m *Manager) snapshotDir() string     { return filepath.Join(m.Options.Root, "update-snapshots") }

func (m *Manager) readUpdate() (*UpdateState, error) {
	b, err := os.ReadFile(m.updatePath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	var u UpdateState
	if err != nil || json.Unmarshal(b, &u) != nil {
		return nil, errors.New("Der Update-Nachweis ist beschädigt.")
	}
	return &u, nil
}

func (m *Manager) writeUpdate(u *UpdateState) error {
	b, _ := json.Marshal(u)
	return privateWrite(m.updatePath(), b)
}

func (m *Manager) writeDataVersion(v dataVersion) error {
	v.Recorded = time.Now().UTC()
	b, _ := json.Marshal(v)
	return privateWrite(m.dataVersionPath(), b)
}

// hasData reports an instance that a server has already initialised.
func hasData(instance string) bool {
	entries, err := os.ReadDir(filepath.Join(instance, "data"))
	return err == nil && len(entries) > 0
}

// errUpdateBlocked keeps the server stopped; the state explains why.
var errUpdateBlocked = errors.New("update blocked")

// prepareStart runs before the server is launched. It returns nil to start,
// errUpdateBlocked when this build must not touch the data.
func (m *Manager) prepareStart() error {
	current, ok := m.Options.packageVersion()
	if !ok {
		return nil
	}
	active := m.State().Active
	m.removeIncompleteSnapshots()
	update, err := m.readUpdate()
	if err != nil {
		return m.blockUpdate(&UpdateState{State: "blocked", To: current, Started: time.Now().UTC()}, err.Error())
	}
	// A block is decided anew on every attempt; only the stored record stays.
	m.setUpdate(update)
	if m.State().Phase == "update_blocked" {
		m.setPhase("idle", "")
	}
	// The whole component set is checked on every start (about 0.4 s for
	// the 800 MB Mac package); a damaged or modified package touches nothing.
	if err := m.checkPackage(&current); err != nil {
		return m.blockUpdate(&UpdateState{State: "blocked", To: current, Started: time.Now().UTC()}, err.Error())
	}
	// An unfinished or failed update: its target build continues; a newer
	// build (e.g. a fix) takes it over and keeps the original snapshot. The
	// previous or any older build must not open possibly migrated data.
	if update != nil && (update.State == "pending" || update.State == "failed") {
		switch {
		case update.To.same(current):
		case !update.From.same(current) && compareVersions(current.Jellyfin, update.To.Jellyfin) >= 0:
			update.To = current
			update.Message = "Eine neuere Version übernimmt das laufende Update; die Sicherung vor dem Update bleibt gültig."
		default:
			return m.blockUpdate(update, fmt.Sprintf("Der Datenstand wurde bereits von %s geöffnet. Diese Version darf ihn nicht verwenden. "+
				"Bitte %s oder neuer verwenden oder die Sicherung vor dem Update wiederherstellen.", update.To.label(), update.To.label()))
		}
		if update.State == "failed" {
			update.State, update.Finished, update.Message = "pending", nil, "Neuer Prüfversuch nach dem Update."
		}
		if err := m.writeUpdate(update); err != nil {
			return err
		}
		m.setUpdate(update)
		return nil
	}
	var stored dataVersion
	b, err := os.ReadFile(m.dataVersionPath())
	switch {
	case os.IsNotExist(err) && !hasData(active):
		return m.writeDataVersion(current) // fresh installation
	case os.IsNotExist(err):
		stored = dataVersion{Schema: 1, Jellyfin: "unknown", Build: "unknown"} // data from before this guard
	case err != nil || json.Unmarshal(b, &stored) != nil:
		return m.blockUpdate(&UpdateState{State: "blocked", To: current, Started: time.Now().UTC()}, "Die Versionsangabe des Datenstands ist beschädigt.")
	}
	if stored.same(current) {
		return nil
	}
	if stored.Jellyfin != "unknown" && compareVersions(stored.Jellyfin, current.Jellyfin) > 0 {
		blocked := &UpdateState{State: "blocked", From: stored, To: current, Started: time.Now().UTC()}
		if snap := m.findSnapshot(current); snap != "" {
			blocked.Snapshot = snap
		}
		return m.blockUpdate(blocked, fmt.Sprintf("Diese Version (%s) ist älter als der Datenstand (%s). Ein älteres Programm kann migrierte Daten nicht sicher öffnen.",
			current.label(), stored.label()))
	}
	// A new build: snapshot first, then let the server migrate.
	update = &UpdateState{ID: randomID(), State: "pending", From: stored, To: current, Started: time.Now().UTC()}
	m.setPhase("update", "Sicherung vor dem Update wird erstellt …")
	snap, err := m.createSnapshot(active, stored)
	if err != nil {
		return m.blockUpdate(&UpdateState{State: "blocked", From: stored, To: current, Started: update.Started},
			"Vor dem Update konnte keine vollständige Sicherung erstellt werden: "+err.Error()+" Es wurde nichts verändert.")
	}
	update.Snapshot = snap
	if err = m.writeUpdate(update); err != nil {
		return err
	}
	m.setUpdate(update)
	m.setPhase("idle", "")
	return nil
}

func (m *Manager) setUpdate(u *UpdateState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u == nil {
		m.state.Update = nil
		return
	}
	snapshot := *u
	m.state.Update = &snapshot
}

func (m *Manager) blockUpdate(u *UpdateState, message string) error {
	u.State, u.Message = "blocked", message
	m.setUpdate(u)
	m.setPhase("update_blocked", message)
	return errUpdateBlocked
}

// verifyUpdate is called while the server runs. The update counts as done
// once the new server answers with the packaged version, setup is complete
// and module and device state are still readable.
func (m *Manager) verifyUpdate(serverVersion string, setupComplete bool) {
	m.mu.Lock()
	u := m.state.Update
	m.mu.Unlock()
	if u == nil || u.State != "pending" || !setupComplete || !strings.HasPrefix(serverVersion, u.To.Jellyfin+".") {
		return
	}
	if err := checkModuleState(m.hubDirectory(), m.State().Active); err != nil {
		m.failUpdate(u, err.Error())
		return
	}
	now := time.Now().UTC()
	verified := *u
	verified.State, verified.Finished, verified.Message = "verified", &now, "Update geprüft. Der Datenstand vor dem Update bleibt als Sicherung erhalten."
	if err := m.writeDataVersion(u.To); err != nil {
		return
	}
	if err := m.writeUpdate(&verified); err != nil {
		return
	}
	m.setUpdate(&verified)
	m.pruneSnapshots(2)
}

func (m *Manager) failUpdate(u *UpdateState, reason string) {
	now := time.Now().UTC()
	failed := *u
	failed.State, failed.Finished = "failed", &now
	failed.Message = "Das Update konnte nicht abgeschlossen werden (" + reason + "). Der Datenstand vor dem Update ist gesichert; " +
		"die vorherige Mutti-Version bietet die Wiederherstellung an."
	_ = m.writeUpdate(&failed)
	m.setUpdate(&failed)
}

// checkModuleState makes sure module and device state still parse.
func checkModuleState(hubDir, active string) error {
	for _, path := range []string{filepath.Join(hubDir, "hub.json"), filepath.Join(active, "connect", "connect.json")} {
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		var v any
		if err != nil || json.Unmarshal(b, &v) != nil {
			return errors.New("Modul- oder Gerätedaten sind nicht mehr lesbar")
		}
	}
	return nil
}

// instanceEntries is what a snapshot keeps of an instance. Without an import
// the instance is the Mutti root itself, so only these entries are copied;
// caches, logs, backups and other instances are not part of the data stage.
var instanceEntries = []string{"config", "data", "connect", "connect-settings.json", "setup-new", "report.json"}

// snapshotExcluded are rebuilt or re-downloaded and not needed for recovery.
func snapshotExcluded(source, rel string) bool {
	rel = filepath.ToSlash(rel)
	switch source {
	case "instance":
		return rel == "data/transcodes" || strings.HasPrefix(rel, "data/transcodes/")
	case "hub":
		return rel == "ai/models" || rel == "ai/home" || rel == "tmp" || strings.HasPrefix(rel, "ai/models/") || strings.HasPrefix(rel, "ai/home/") || strings.HasPrefix(rel, "tmp/")
	}
	return false
}

// snapshotSources lists (live path, path inside the snapshot) pairs.
func (m *Manager) snapshotSources(active string) [][2]string {
	var out [][2]string
	for _, e := range instanceEntries {
		out = append(out, [2]string{filepath.Join(active, e), "instance/" + e})
	}
	return append(out, [2]string{m.hubDirectory(), "hub"})
}

// freeBytes is replaceable in tests.
var freeBytes = func(path string) (uint64, error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return 0, err
	}
	return uint64(s.Bavail) * uint64(s.Bsize), nil
}

// createSnapshot copies the stopped instance and the shared module data into
// a new snapshot directory, hashes every file and only then makes it visible.
func (m *Manager) createSnapshot(active string, version dataVersion) (string, error) {
	rel, err := filepath.Rel(m.Options.Root, active)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("Die aktive Instanz liegt nicht im Mutti-Datenordner.")
	}
	sources := m.snapshotSources(active)
	kind := func(to string) string { k, _, _ := strings.Cut(to, "/"); return k }
	var need int64
	for _, s := range sources {
		_ = filepath.WalkDir(s[0], func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			r, _ := filepath.Rel(filepath.Dir(s[0]), path)
			if d.IsDir() && snapshotExcluded(kind(s[1]), relWithin(s, r)) {
				return filepath.SkipDir
			}
			if info, e := d.Info(); e == nil && info.Mode().IsRegular() {
				need += info.Size()
			}
			return nil
		})
	}
	if err = os.MkdirAll(m.snapshotDir(), 0700); err != nil {
		return "", err
	}
	// APFS clones cost almost nothing, but other file systems need the space.
	if free, err := freeBytes(m.snapshotDir()); err != nil || free < uint64(need)+uint64(need)/10+64<<20 {
		return "", errors.New("Für die Sicherung ist nicht genug freier Speicher vorhanden.")
	}
	id := randomID()
	tmp := filepath.Join(m.snapshotDir(), ".incomplete-"+id)
	_ = os.RemoveAll(tmp)
	defer os.RemoveAll(tmp)
	manifest := snapshotManifest{Schema: 1, ID: id, Created: time.Now().UTC(), Version: version, Active: rel, Hashes: map[string]string{}}
	for _, s := range sources {
		if _, err := os.Lstat(s[0]); os.IsNotExist(err) {
			continue
		}
		base := strings.TrimPrefix(strings.TrimPrefix(s[1], "instance/"), "hub")
		k := kind(s[1])
		if err := copyTree(s[0], filepath.Join(tmp, filepath.FromSlash(s[1])), func(r string) bool {
			return snapshotExcluded(k, filepath.ToSlash(filepath.Join(base, r)))
		}); err != nil {
			return "", errors.New("Kopieren fehlgeschlagen.")
		}
	}
	err = filepath.WalkDir(tmp, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		r, _ := filepath.Rel(tmp, path)
		h, n, e := fileHash(path)
		if e != nil {
			return e
		}
		manifest.Hashes[filepath.ToSlash(r)] = h
		manifest.Bytes += n
		return nil
	})
	if err != nil {
		return "", errors.New("Die Sicherung konnte nicht geprüft werden.")
	}
	b, _ := json.Marshal(manifest)
	if err = privateWrite(filepath.Join(tmp, "snapshot.json"), b); err != nil {
		return "", err
	}
	if err = os.Rename(tmp, filepath.Join(m.snapshotDir(), id)); err != nil {
		return "", err
	}
	return id, nil
}

// copyTree copies regular files, directories and symlinks (as links). On
// macOS cp -c clones files on APFS.
func copyTree(from, to string, skip func(rel string) bool) error {
	if info, err := os.Lstat(from); err == nil && info.Mode().IsRegular() {
		if err := os.MkdirAll(filepath.Dir(to), 0700); err != nil {
			return err
		}
		return copyFile(from, to)
	}
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		if rel != "." && skip(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(to, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0700)
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case d.Type().IsRegular():
			return copyFile(path, target)
		}
		return nil // sockets and other special files are not data
	})
}

// relWithin maps a path relative to the source's parent to the path the
// exclusion rules use (relative to the instance or hub root).
func relWithin(s [2]string, relToParent string) string {
	rel := filepath.ToSlash(relToParent)
	if strings.HasPrefix(s[1], "instance/") {
		return rel
	}
	_, rest, _ := strings.Cut(rel, "/")
	return rest
}

func copyFile(from, to string) error {
	if runtime.GOOS == "darwin" {
		if exec.Command("/bin/cp", "-c", "-p", from, to).Run() == nil {
			return nil
		}
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if e := out.Sync(); err == nil {
		err = e
	}
	if e := out.Close(); err == nil {
		err = e
	}
	return err
}

func (m *Manager) readSnapshot(id string) (*snapshotManifest, string, error) {
	if !validID(id) {
		return nil, "", errors.New("Ungültige Sicherung.")
	}
	dir := filepath.Join(m.snapshotDir(), id)
	b, err := os.ReadFile(filepath.Join(dir, "snapshot.json"))
	var s snapshotManifest
	if err != nil || json.Unmarshal(b, &s) != nil || s.Schema != 1 || s.ID != id {
		return nil, "", errors.New("Die Sicherung vor dem Update ist unvollständig.")
	}
	base, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, "", errors.New("Die Sicherung vor dem Update ist unvollständig.")
	}
	for rel, want := range s.Hashes {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		// No symlinked file or parent inside the snapshot.
		if resolved, err := filepath.EvalSymlinks(path); err != nil || resolved != filepath.Join(base, filepath.FromSlash(rel)) {
			return nil, "", errors.New("Unsicherer Pfad in der Sicherung.")
		}
		if h, _, err := fileHash(path); err != nil || h != want {
			return nil, "", errors.New("Die Prüfsumme der Sicherung stimmt nicht. Es wurde nichts verändert.")
		}
	}
	return &s, dir, nil
}

// findSnapshot returns the newest snapshot of a given data stage.
func (m *Manager) findSnapshot(v dataVersion) string {
	entries, _ := os.ReadDir(m.snapshotDir())
	best, newest := "", time.Time{}
	for _, e := range entries {
		if !e.IsDir() || !validID(e.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(m.snapshotDir(), e.Name(), "snapshot.json"))
		var s snapshotManifest
		if err == nil && json.Unmarshal(b, &s) == nil && s.Version.same(v) && s.Created.After(newest) {
			best, newest = e.Name(), s.Created
		}
	}
	return best
}

// removeIncompleteSnapshots deletes copies an interrupted start left behind;
// they were never published and no state refers to them.
func (m *Manager) removeIncompleteSnapshots() {
	entries, _ := os.ReadDir(m.snapshotDir())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".incomplete-") {
			_ = os.RemoveAll(filepath.Join(m.snapshotDir(), e.Name()))
		}
	}
}

// pruneSnapshots keeps the newest n complete snapshots and removes
// interrupted ones.
func (m *Manager) pruneSnapshots(keep int) {
	entries, _ := os.ReadDir(m.snapshotDir())
	type snap struct {
		id      string
		created time.Time
	}
	var all []snap
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".incomplete-") {
			_ = os.RemoveAll(filepath.Join(m.snapshotDir(), e.Name()))
			continue
		}
		b, err := os.ReadFile(filepath.Join(m.snapshotDir(), e.Name(), "snapshot.json"))
		var s snapshotManifest
		if err == nil && json.Unmarshal(b, &s) == nil {
			all = append(all, snap{e.Name(), s.Created})
		}
	}
	slices.SortFunc(all, func(a, b snap) int { return b.created.Compare(a.created) })
	for i := keep; i < len(all); i++ {
		_ = os.RemoveAll(filepath.Join(m.snapshotDir(), all[i].id))
	}
}

// rollbackUpdate restores a verified pre-update snapshot while the server is
// stopped. The replaced data is kept in a dated folder, never deleted.
func (m *Manager) rollbackUpdate(id string) error {
	current, ok := m.Options.packageVersion()
	if !ok {
		return errors.New("Paketversion unbekannt.")
	}
	s, dir, err := m.readSnapshot(id)
	if err != nil {
		return err
	}
	if !s.Version.same(current) {
		return fmt.Errorf("Diese Sicherung gehört zu %s; bitte mit genau dieser Version wiederherstellen.", s.Version.label())
	}
	if m.child.running() {
		return errors.New("Bitte zuerst den Server beenden.")
	}
	active := filepath.Join(m.Options.Root, filepath.FromSlash(s.Active))
	keep := filepath.Join(m.Options.Root, "before-rollback-"+time.Now().UTC().Format("20060102-150405"))
	if err = os.MkdirAll(keep, 0700); err != nil {
		return err
	}
	for _, s := range m.snapshotSources(active) {
		live, saved := s[0], filepath.Join(dir, filepath.FromSlash(s[1]))
		_, savedErr := os.Lstat(saved)
		if _, err := os.Lstat(live); err == nil {
			// Everything in the data stage is replaced; what the snapshot does
			// not contain is moved aside as well, so no newer file remains.
			target := filepath.Join(keep, filepath.FromSlash(s[1]))
			if err = os.MkdirAll(filepath.Dir(target), 0700); err == nil {
				err = os.Rename(live, target)
			}
			if err != nil {
				return errors.New("Der aktuelle Datenstand konnte nicht beiseitegelegt werden; der bisher verschobene Teil liegt in " + keep + ".")
			}
		}
		if savedErr != nil {
			continue
		}
		if err = copyTree(saved, live, func(string) bool { return false }); err != nil {
			return errors.New("Die Wiederherstellung ist unvollständig; der vorherige Stand liegt in " + keep + ".")
		}
	}
	// Access withdrawn after the snapshot stays withdrawn.
	revoked, err := keepRevocations(filepath.Join(keep, "instance", "connect", "connect.json"), filepath.Join(active, "connect", "connect.json"),
		filepath.Join(keep, "hub", "hub.json"), filepath.Join(m.hubDirectory(), "hub.json"))
	if err != nil {
		return errors.New("Entzogene Zugriffe konnten nicht übernommen werden; der vorherige Stand liegt in " + keep + ".")
	}
	// Downloaded models are not part of a snapshot; keep the installed ones.
	if models := filepath.Join(keep, "hub", "ai", "models"); dirExists(models) {
		_ = os.MkdirAll(filepath.Join(m.hubDirectory(), "ai"), 0700)
		_ = os.Rename(models, filepath.Join(m.hubDirectory(), "ai", "models"))
	}
	if err = m.writeDataVersion(s.Version); err != nil {
		return err
	}
	now := time.Now().UTC()
	message := "Datenstand vor dem Update wiederhergestellt. Der zuvor verwendete Stand liegt in " + filepath.Base(keep) + "."
	if revoked > 0 {
		message += fmt.Sprintf(" %d nach der Sicherung entzogene Geräte oder Freigaben bleiben entzogen; danach gekoppelte Geräte bitte neu koppeln.", revoked)
	}
	update := &UpdateState{State: "rolled_back", From: s.Version, To: current, Snapshot: id, Started: s.Created, Finished: &now, Message: message}
	if err = m.writeUpdate(update); err != nil {
		return err
	}
	m.setUpdate(update)
	return nil
}

// keepRevocations applies withdrawals made after the snapshot to the
// restored state: a paired device, a module grant, an enabled module or a
// profile link survives a rollback only if it also exists in the state being
// replaced. A missing or unreadable replaced state counts as withdrawn (fail
// closed). Jellyfin's own users and sessions return to the snapshot; remote
// access only works through a paired device. Returns the number withdrawn.
func keepRevocations(previousConnect, restoredConnect, previousHub, restoredHub string) (int, error) {
	n := 0
	err := editJSON(restoredConnect, func(restored map[string]any) {
		current := map[string]any{}
		_ = readJSONMap(previousConnect, &current)
		now, _ := current["devices"].(map[string]any)
		devices, _ := restored["devices"].(map[string]any)
		for pin := range devices {
			if _, ok := now[pin]; !ok {
				delete(devices, pin)
				n++
			}
		}
	})
	if err != nil {
		return n, err
	}
	err = editJSON(restoredHub, func(restored map[string]any) {
		current := map[string]any{}
		_ = readJSONMap(previousHub, &current)
		nowModules, _ := current["modules"].(map[string]any)
		modules, _ := restored["modules"].(map[string]any)
		for name, raw := range modules {
			module, _ := raw.(map[string]any)
			now, _ := nowModules[name].(map[string]any)
			if module == nil {
				continue
			}
			if module["enabled"] == true && now["enabled"] != true {
				module["enabled"] = false
				n++
			}
			nowGrants, _ := now["grants"].(map[string]any)
			grants, _ := module["grants"].(map[string]any)
			for user, allowed := range grants {
				if allowed == true && nowGrants[user] != true {
					grants[user] = false
					n++
				}
			}
			nowLinks, _ := now["links"].(map[string]any)
			links, _ := module["links"].(map[string]any)
			for profile := range links {
				if _, ok := nowLinks[profile]; !ok {
					delete(links, profile)
					n++
				}
			}
		}
	})
	return n, err
}

func readJSONMap(path string, out *map[string]any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// editJSON rewrites a private JSON file only if edit changed it. A missing
// file has nothing to restrict.
func editJSON(path string, edit func(map[string]any)) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	v := map[string]any{}
	if err = json.Unmarshal(b, &v); err != nil {
		return err
	}
	before, _ := json.Marshal(v)
	edit(v)
	after, _ := json.Marshal(v)
	if string(before) == string(after) {
		return nil
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, out, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
