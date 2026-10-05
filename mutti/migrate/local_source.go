// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type launchAgent struct {
	Label, Program       string
	ProgramArguments     []string
	EnvironmentVariables map[string]string
	KeepAlive            json.RawMessage
}
type localSource struct {
	PID                                      int
	Service, Plist, Config, Data, Executable string
}

var serviceLabel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)
var servicePID = regexp.MustCompile(`(?m)^\s*pid = ([0-9]+)\s*$`)
var lockingElement = regexp.MustCompile(`(?i)<LockingBehavior>\s*Pessimistic\s*</LockingBehavior>`)

func macCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, name, args...)
	// These OS commands only emit bounded process/service/plist metadata. Avoid
	// propagating output, which may contain paths/environment, in user errors.
	return cmd.Output()
}
func readOwnedFile(path string, limit int64) ([]byte, error) {
	real, e := filepath.EvalSymlinks(path)
	if e != nil || real != filepath.Clean(path) {
		return nil, errors.New("Dateipfad konnte nicht eindeutig geprüft werden.")
	}
	st, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	owner, ok := st.Sys().(*syscall.Stat_t)
	if !ok || int(owner.Uid) != os.Getuid() || !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 || st.Size() > limit {
		return nil, errors.New("Diese Jellyfin-Datei gehört nicht ausschließlich zum angemeldeten Benutzer.")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(st, opened) {
		return nil, errors.New("Die Datei hat sich während der Prüfung geändert.")
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, errors.New("Datei ist zu groß.")
	}
	return b, e
}
func explicitDirectory(a launchAgent, flag, env string) string {
	value := a.EnvironmentVariables[env]
	for i := 1; i < len(a.ProgramArguments); i++ {
		if a.ProgramArguments[i] == flag && i+1 < len(a.ProgramArguments) {
			value = a.ProgramArguments[i+1]
			i++
		}
		if strings.HasPrefix(a.ProgramArguments[i], flag+"=") {
			value = strings.TrimPrefix(a.ProgramArguments[i], flag+"=")
		}
	}
	if !filepath.IsAbs(value) {
		return ""
	}
	return filepath.Clean(value)
}
func qualifyAgent(a launchAgent, exe string) (string, string, bool) {
	if !serviceLabel.MatchString(a.Label) || string(a.KeepAlive) != "true" || len(a.ProgramArguments) == 0 || a.ProgramArguments[0] != exe || a.Program != "" && a.Program != exe || strings.ToLower(filepath.Base(exe)) != "jellyfin" {
		return "", "", false
	}
	data := explicitDirectory(a, "--datadir", "JELLYFIN_DATA_DIR")
	config := explicitDirectory(a, "--configdir", "JELLYFIN_CONFIG_DIR")
	return data, config, data != "" && config != ""
}
func launchPID(ctx context.Context, service string) (int, error) {
	out, e := macCommand(ctx, "/bin/launchctl", "print", service)
	if e != nil {
		return 0, e
	}
	match := servicePID.FindSubmatch(out)
	if len(match) != 2 {
		return 0, nil
	}
	return strconv.Atoi(string(match[1]))
}
func inspectLocalSource(ctx context.Context, a *API, info PublicInfo, targetRoot string) (*localSource, error) {
	ip := net.ParseIP(a.Base.Hostname())
	if runtime.GOOS != "darwin" || a.Base.Scheme != "http" || a.Base.Path != "" || !(a.Base.Hostname() == "localhost" || ip != nil && ip.IsLoopback()) {
		return nil, nil
	}
	port := a.Base.Port()
	if port == "" {
		return nil, nil
	}
	out, e := macCommand(ctx, "/usr/sbin/lsof", "-nP", "-a", "-iTCP:"+port, "-sTCP:LISTEN", "-Fp")
	if e != nil {
		return nil, nil
	}
	pids := map[int]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "p") {
			n, _ := strconv.Atoi(line[1:])
			if n > 0 {
				pids[n] = true
			}
		}
	}
	if len(pids) != 1 {
		return nil, nil
	}
	var pid int
	for n := range pids {
		pid = n
	}
	out, e = macCommand(ctx, "/bin/ps", "-p", strconv.Itoa(pid), "-o", "uid=,comm=")
	if e != nil {
		return nil, nil
	}
	line := strings.TrimSpace(string(out))
	parts := strings.Fields(line)
	if len(parts) < 2 || parts[0] != strconv.Itoa(os.Getuid()) {
		return nil, nil
	}
	exe := strings.TrimSpace(strings.TrimPrefix(line, parts[0]))
	if strings.ToLower(filepath.Base(exe)) != "jellyfin" {
		return nil, nil
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return nil, nil
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	entries, _ := os.ReadDir(dir)
	var found *localSource
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".plist") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if _, e = readOwnedFile(path, 1<<20); e != nil {
			continue
		}
		out, e = macCommand(ctx, "/usr/bin/plutil", "-convert", "json", "-o", "-", path)
		var agent launchAgent
		if e != nil || json.Unmarshal(out, &agent) != nil {
			continue
		}
		data, config, ok := qualifyAgent(agent, exe)
		if !ok {
			continue
		}
		root, _ := filepath.EvalSymlinks(targetRoot)
		if root != "" && (data == root || within(root, data) || within(data, root) || config == root || within(root, config)) {
			continue
		}
		device, e := readOwnedFile(filepath.Join(data, "data", "device.txt"), 4096)
		if e != nil || strings.TrimSpace(strings.TrimPrefix(string(device), "\ufeff")) != info.Id {
			continue
		}
		configPath := filepath.Join(config, "database.xml")
		if _, e = readOwnedFile(configPath, 1<<20); e != nil {
			continue
		}
		service := fmt.Sprintf("gui/%d/%s", os.Getuid(), agent.Label)
		active, e := launchPID(ctx, service)
		if e != nil || active != pid {
			continue
		}
		// Require the loaded service to reference the same on-disk plist.
		out, e = macCommand(ctx, "/bin/launchctl", "print", service)
		if e != nil || !strings.Contains(string(out), "path = "+path+"\n") {
			continue
		}
		if found != nil {
			return nil, errors.New("Der lokale Jellyfin-Dienst ist nicht eindeutig zugeordnet. Es wurde nichts verändert.")
		}
		found = &localSource{pid, service, path, configPath, data, exe}
	}
	return found, nil
}
func prepareDatabaseXML(original []byte) ([]byte, bool, error) {
	var config struct {
		XMLName                       xml.Name
		LockingBehavior, DatabaseType string
	}
	d := xml.NewDecoder(bytes.NewReader(original))
	if e := d.Decode(&config); e != nil {
		return nil, false, e
	}
	if config.DatabaseType != "Jellyfin-SQLite" {
		return nil, false, nil
	}
	if config.LockingBehavior != "Pessimistic" {
		return nil, false, nil
	}
	if len(lockingElement.FindAllIndex(original, -1)) != 1 {
		return nil, false, errors.New("Die Jellyfin-Einstellung ist nicht eindeutig. Es wurde nichts verändert.")
	}
	return lockingElement.ReplaceAll(original, []byte("<LockingBehavior>NoLock</LockingBehavior>")), true, nil
}
func restartLocalService(ctx context.Context, source *localSource, grace time.Duration) error {
	current, e := launchPID(ctx, source.Service)
	if e != nil || current != source.PID {
		return errors.New("Der Jellyfin-Dienst hat sich geändert. Der Neustart wurde angehalten.")
	}
	if _, e = macCommand(ctx, "/bin/launchctl", "kill", "SIGTERM", source.Service); e != nil {
		return errors.New("Der lokale Jellyfin-Dienst konnte nicht beendet werden.")
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		current, e = launchPID(ctx, source.Service)
		if e == nil && current > 0 && current != source.PID {
			return nil
		}
	}
	current, e = launchPID(ctx, source.Service)
	if e != nil {
		return errors.New("Der lokale Jellyfin-Dienst ist nicht mehr verfügbar.")
	}
	if current > 0 && current != source.PID {
		return nil
	}
	// KeepAlive is required during qualification. A blocked process may never
	// finish its database shutdown; launchd replaces only the bound user service.
	args := []string{"kickstart"}
	if current == source.PID {
		args = append(args, "-k")
	}
	args = append(args, source.Service)
	// kickstart can outlive its command timeout while launchd is already
	// replacing the blocked service. Verify the new PID instead of reporting an
	// ambiguous command result as failure or sending another restart.
	_, _ = macCommand(ctx, "/bin/launchctl", args...)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		current, e = launchPID(ctx, source.Service)
		if e == nil && current > 0 && current != source.PID {
			return nil
		}
	}
}
func (m *Manager) prepareLocalSource(ctx context.Context, input SourceInput, targetID string) error {
	if runtime.GOOS != "darwin" || m.Options.Container || !input.nativeOwner || !input.PrepareSource {
		return nil
	}
	a, e := NewAPI(input.Address)
	if e != nil {
		return e
	}
	defer a.Client.CloseIdleConnections()
	c, cancel := context.WithTimeout(ctx, 4*time.Second)
	var info PublicInfo
	e = a.call(c, "GET", "/System/Info/Public", nil, &info)
	cancel()
	// Unavailable/nonlocal/unmanaged servers continue through the authenticated API.
	if e != nil {
		return nil
	}
	if info.Id == "" || info.Id == targetID || !info.StartupWizardCompleted || !strings.HasPrefix(info.Version, "12.1.") {
		return nil
	}
	source, e := inspectLocalSource(ctx, a, info, m.Options.Root)
	if e != nil {
		return e
	}
	if source == nil {
		return nil
	}
	original, e := readOwnedFile(source.Config, 1<<20)
	if e != nil {
		return e
	}
	if preparationConflict(m.Options.Root, info.Id, a.Base.String(), "xml", original) {
		return errors.New("Die Jellyfin-Einstellungen wurden seit der Vorbereitung verändert. Es wird nichts überschrieben.")
	}
	prepared, change, e := prepareDatabaseXML(original)
	if e != nil {
		return e
	}
	pending := pendingPreparation(m.Options.Root, info.Id, a.Base.String(), "xml", original)
	if !change && !pending {
		return nil
	}
	m.setStep("checking", 1, "Jellyfin wird für den Umzug vorbereitet …")
	if change {
		if e = savePreparation(m.Options.Root, info.Id, a.Base.String(), "xml", original, prepared); e != nil {
			return errors.New("Die bisherigen Jellyfin-Einstellungen konnten nicht gesichert werden. Es wurde nichts verändert.")
		}
		current, e := readOwnedFile(source.Config, 1<<20)
		if e != nil || !bytes.Equal(current, original) {
			return errors.New("Die Jellyfin-Einstellungen haben sich geändert. Die Vorbereitung wurde angehalten.")
		}
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	// Re-bind the service immediately before the change and restart, without
	// trusting a PID/path supplied by a web page or a stored receipt.
	current, e := inspectLocalSource(ctx, a, info, m.Options.Root)
	if e != nil || current == nil || *current != *source {
		return errors.New("Der lokale Jellyfin-Dienst hat sich geändert. Es wurde nichts verändert.")
	}
	recovery, stop := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
	defer stop()
	if change {
		if e = privateWrite(source.Config, prepared); e != nil {
			return e
		}
	}
	m.setStep("checking", 1, "Jellyfin wird neu gestartet. Mutti wartet auf deinen Server …")
	if e = restartLocalService(recovery, source, 8*time.Second); e != nil {
		return fmt.Errorf("%w Die bisherigen Einstellungen sind gesichert; es wurde keine weitere Sicherung gestartet.", e)
	}
	for {
		if recovery.Err() != nil {
			return errors.New("Jellyfin ist nach dem Neustart noch nicht bereit. Die bisherigen Einstellungen sind gesichert; bitte später erneut versuchen.")
		}
		c, cancel = context.WithTimeout(recovery, 2*time.Second)
		var ready PublicInfo
		e = a.call(c, "GET", "/System/Info/Public", nil, &ready)
		cancel()
		if e == nil && ready.StartupWizardCompleted {
			if ready.Id != info.Id {
				return errors.New("Nach dem Neustart antwortet ein anderer Server. Der Import wurde angehalten.")
			}
			break
		}
		select {
		case <-recovery.Done():
		case <-time.After(300 * time.Millisecond):
		}
	}
	currentConfig, e := readOwnedFile(source.Config, 1<<20)
	if e != nil {
		return e
	}
	var config struct{ LockingBehavior string }
	if xml.Unmarshal(currentConfig, &config) != nil || config.LockingBehavior != "NoLock" {
		return errors.New("Jellyfin hat die Vorbereitung nicht übernommen. Der Import wurde angehalten.")
	}
	if e = finishPreparation(m.Options.Root, info.Id, a.Base.String()); e != nil {
		return e
	}
	m.setStep("checking", 1, "Jellyfin ist wieder bereit. Der Umzug wird fortgesetzt …")
	return ctx.Err()
}
