// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A receipt is durable before changing the source. A retry can therefore finish
// a restart interrupted by cancellation/app shutdown even if the mode is NoLock.
// It contains no login/session credentials and is never exposed by /api/state.
type preparationReceipt struct {
	ServerID, Address, Format, OriginalSHA, PreparedSHA, Backup string
}

func digestBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func preparationDir(root, id, address string) string {
	return filepath.Join(root, "source-preparation", digestBytes([]byte(id+"\n"+address)))
}
func savePreparation(root, id, address, format string, original, prepared []byte) error {
	dir := preparationDir(root, id, address)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	backup := "database-original-" + randomID() + "." + format
	if e := privateWrite(filepath.Join(dir, backup), original); e != nil {
		return e
	}
	receipt := preparationReceipt{id, address, format, digestBytes(original), digestBytes(prepared), backup}
	b, _ := json.Marshal(receipt)
	return privateWrite(filepath.Join(dir, "pending.json"), b)
}
func pendingPreparation(root, id, address, format string, current []byte) bool {
	b, e := os.ReadFile(filepath.Join(preparationDir(root, id, address), "pending.json"))
	var r preparationReceipt
	return e == nil && json.Unmarshal(b, &r) == nil && r.ServerID == id && r.Address == address && r.Format == format && r.PreparedSHA == digestBytes(current)
}
func preparationConflict(root, id, address, format string, current []byte) bool {
	b, e := os.ReadFile(filepath.Join(preparationDir(root, id, address), "pending.json"))
	var receipt preparationReceipt
	if e != nil || json.Unmarshal(b, &receipt) != nil || receipt.ServerID != id || receipt.Address != address || receipt.Format != format {
		return false
	}
	hash := digestBytes(current)
	return hash != receipt.PreparedSHA && hash != receipt.OriginalSHA
}
func finishPreparation(root, id, address string) error {
	e := os.Remove(filepath.Join(preparationDir(root, id, address), "pending.json"))
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
func pessimistic(mode json.RawMessage) bool {
	s := strings.Trim(string(mode), "\" ")
	return strings.EqualFold(s, "Pessimistic") || s == "1"
}
func readDatabaseConfig(ctx context.Context, a *API) (map[string]json.RawMessage, []byte, error) {
	var config map[string]json.RawMessage
	if e := a.call(ctx, "GET", "/System/Configuration/database", nil, &config); e != nil {
		return nil, nil, e
	}
	if config == nil {
		return nil, nil, errors.New("Jellyfin hat keine gültigen Datenbankeinstellungen geliefert.")
	}
	b, e := json.Marshal(config)
	return config, b, e
}
func sameSource(ctx context.Context, a *API, id string) (PublicInfo, error) {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var info PublicInfo
	e := a.call(c, "GET", "/System/Info/Public", nil, &info)
	if e != nil {
		return info, e
	}
	if info.Id != id {
		return info, errors.New("An dieser Adresse antwortet ein anderer Jellyfin-Server. Der Import wurde angehalten.")
	}
	return info, nil
}
func (m *Manager) prepareSource(ctx context.Context, s *Source, input SourceInput) error {
	// Do not change either a nested target or an already active Mutti instance.
	if s.Local {
		source, _ := filepath.EvalSymlinks(s.Info.ProgramDataPath)
		target, _ := filepath.EvalSymlinks(m.Options.Root)
		if source != "" && target != "" && (source == target || within(target, source) || within(source, target)) {
			return errors.New("Die Quelle und Mutti müssen getrennte Datenordner verwenden.")
		}
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	config, original, e := readDatabaseConfig(c, s.API)
	cancel()
	if e != nil {
		return fmt.Errorf("Jellyfins Importvorbereitung konnte nicht geprüft werden: %w", e)
	}
	if preparationConflict(m.Options.Root, s.Info.Id, s.API.Base.String(), "json", original) {
		return errors.New("Die Jellyfin-Einstellungen wurden seit der Vorbereitung verändert. Der Import wurde angehalten; es wird nichts überschrieben.")
	}
	needsChange := pessimistic(config["LockingBehavior"])
	pending := pendingPreparation(m.Options.Root, s.Info.Id, s.API.Base.String(), "json", original)
	if needsChange && string(config["DatabaseType"]) != `"Jellyfin-SQLite"` {
		return errors.New("Diese Datenbank kann Mutti noch nicht automatisch vorbereiten. Es wurde nichts verändert.")
	}
	if !needsChange && !pending {
		return nil
	}
	if !input.PrepareSource {
		return errors.New("Bitte den Import mit automatischer Vorbereitung starten. Jellyfin muss dafür kurz neu gestartet werden.")
	}
	if _, e = sameSource(ctx, s.API, s.Info.Id); e != nil {
		return e
	}
	m.setStep("checking", 1, "Jellyfin wird für den Umzug vorbereitet …")
	if needsChange {
		// Preserve every unknown/provider-specific property instead of replacing the
		// configuration with a partial object.
		config["LockingBehavior"] = json.RawMessage(`"NoLock"`)
		prepared, _ := json.Marshal(config)
		if e = savePreparation(m.Options.Root, s.Info.Id, s.API.Base.String(), "json", original, prepared); e != nil {
			return errors.New("Die bisherigen Jellyfin-Einstellungen konnten nicht gesichert werden. Es wurde nichts verändert.")
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		c, cancel = context.WithTimeout(ctx, 10*time.Second)
		_, current, readErr := readDatabaseConfig(c, s.API)
		cancel()
		if readErr != nil || !bytes.Equal(current, original) {
			return errors.New("Die Jellyfin-Einstellungen haben sich während der Vorbereitung geändert. Es wurde nichts überschrieben.")
		}
	}
	// Once the setting is changed, finish the bounded restart even when import is
	// cancelled. A crash/forced exit leaves the durable pending receipt for retry.
	recovery, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer stop()
	if needsChange {
		c, cancel = context.WithTimeout(recovery, 10*time.Second)
		e = s.API.call(c, "POST", "/System/Configuration/database", config, nil)
		cancel()
		if e != nil {
			return fmt.Errorf("Jellyfin konnte nicht automatisch vorbereitet werden. Die bisherigen Einstellungen sind gesichert: %w", e)
		}
	}
	m.setStep("checking", 1, "Jellyfin wird neu gestartet. Mutti wartet auf deinen Server …")
	if e = restartSourceAPI(recovery, s); e != nil {
		return e
	}
	if e = finishPreparation(m.Options.Root, s.Info.Id, s.API.Base.String()); e != nil {
		return e
	}
	m.setStep("checking", 1, "Jellyfin ist wieder bereit. Der Umzug wird fortgesetzt …")
	return ctx.Err()
}
func restartSourceAPI(ctx context.Context, s *Source) error {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	e := s.API.call(c, "POST", "/System/Restart", nil, nil)
	cancel()
	if e != nil {
		return fmt.Errorf("Jellyfin konnte nicht automatisch neu gestartet werden. Die Vorbereitung bleibt für den nächsten Versuch gespeichert: %w", e)
	}
	// Observe a shutdown/startup interval; reading the newly saved configuration
	// alone is not proof that the running process uses the new locking behavior.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	gone := false
	for {
		select {
		case <-ctx.Done():
			return errors.New("Jellyfins Neustart konnte nicht bestätigt werden. Deine Daten bleiben erhalten; Mutti hat keine Sicherung gestartet. Beim nächsten Versuch wird die Vorbereitung erneut geprüft.")
		case <-ticker.C:
		}
		c, cancel = context.WithTimeout(ctx, time.Second)
		var info PublicInfo
		e = s.API.call(c, "GET", "/System/Info/Public", nil, &info)
		cancel()
		if e != nil || !info.StartupWizardCompleted {
			gone = true
			continue
		}
		if info.Id != s.Info.Id {
			return errors.New("Nach dem Neustart antwortet ein anderer Server. Es wurde keine Sicherung gestartet.")
		}
		if !gone {
			continue
		}
		// Existing Jellyfin tokens survive a restart. Do not resubmit the password
		// repeatedly against a server still starting up.
		c, cancel = context.WithTimeout(ctx, 3*time.Second)
		config, _, err := readDatabaseConfig(c, s.API)
		cancel()
		if err != nil {
			continue
		}
		var mode string
		if json.Unmarshal(config["LockingBehavior"], &mode) != nil || mode != "NoLock" {
			return errors.New("Jellyfin hat die vorbereitete Einstellung nicht übernommen. Es wurde keine Sicherung gestartet.")
		}
		return nil
	}
}
func (m *Manager) openImportSource(ctx context.Context, input SourceInput, targetID string) (*Source, error) {
	if input.PrepareSource && input.nativeOwner {
		if e := m.prepareLocalSource(ctx, input, targetID); e != nil {
			return nil, e
		}
	}
	return OpenSource(ctx, input, targetID, func(ctx context.Context, s *Source) error { return m.prepareSource(ctx, s, input) })
}
