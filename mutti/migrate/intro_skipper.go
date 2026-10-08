// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const introID = "c83d86bb-a1e0-4c35-a113-e2101cf4ee6b"
const introVersion = "12.0.4.0"
const introDLLHash = "f5c10f3453d48bfa210c6c487c67fc3ee7ce4c96c51121b2a6608f8a923399e8"

var introFiles = map[string]bool{"IntroSkipper.xml": true, "introskipper-v2.db": true, "introskipper-cache.db": true, "introskipper.db": true}

func samePluginID(a, b string) bool {
	return strings.EqualFold(strings.ReplaceAll(a, "-", ""), strings.ReplaceAll(b, "-", ""))
}

type PluginInfo struct {
	Id, Name, Version, Status string
	CanUninstall              bool
}

func qualifyPlugins(plugins []PluginInfo) (bool, error) {
	intro := false
	for _, p := range plugins {
		if samePluginID(p.Id, introID) {
			if p.Version != introVersion {
				return false, fmt.Errorf("Intro Skipper %s benötigt noch eine geprüfte Datenmigration. Der bisherige Datenstand bleibt erhalten.", p.Version)
			}
			intro = true
			continue
		}
		if p.CanUninstall && p.Status != "Disabled" && p.Name != "Mutti Export" {
			return false, fmt.Errorf("Das zusätzliche Plugin „%s“ benötigt eine geprüfte Übernahme. Der Import wurde vor der Änderung gestoppt.", p.Name)
		}
	}
	return intro, nil
}
func (o Options) installIntro(root string) error {
	if o.IntroSkipper == "" {
		return nil
	} // Non-packaged upstream test fixtures may omit it.
	dll, e := os.ReadFile(filepath.Join(o.IntroSkipper, "IntroSkipper.dll"))
	if e != nil {
		return errors.New("Das mitgelieferte Intro Skipper fehlt. Bitte das vollständige Mutti-Paket verwenden.")
	}
	h := sha256.Sum256(dll)
	if hex.EncodeToString(h[:]) != introDLLHash {
		return errors.New("Die Intro-Skipper-Prüfsumme stimmt nicht. Bitte das vollständige Mutti-Paket verwenden.")
	}
	dir := filepath.Join(root, "data", "plugins", "Intro Skipper_"+introVersion)
	if e = privateWrite(filepath.Join(dir, "IntroSkipper.dll"), dll); e != nil {
		return e
	}
	meta, _ := json.Marshal(map[string]any{"name": "Intro Skipper", "guid": introID, "version": introVersion, "targetAbi": "12.1.0.0", "status": "Active", "autoUpdate": false})
	return privateWrite(filepath.Join(dir, "meta.json"), meta)
}

type introSnapshot struct {
	Directory string
	Hashes    map[string]string
}

func readIntroSnapshot(directory string) (introSnapshot, error) {
	s := introSnapshot{Directory: directory}
	b, e := os.ReadFile(filepath.Join(directory, "snapshot.json"))
	if e != nil {
		return s, errors.New("Die Intro-Skipper-Datensicherung fehlt. Bei Fernimport bitte den aktuellen Mutti-Umzugshelfer verwenden.")
	}
	if len(b) > 16384 || json.Unmarshal(b, &s.Hashes) != nil || s.Hashes == nil {
		return s, errors.New("Ungültige Intro-Skipper-Sicherung.")
	}
	for name, h := range s.Hashes {
		digest, e := hex.DecodeString(h)
		if !introFiles[name] || e != nil || len(digest) != 32 {
			return s, errors.New("Unbekannte Daten in der Intro-Skipper-Sicherung.")
		}
		st, e := os.Lstat(filepath.Join(directory, name))
		if e != nil || !st.Mode().IsRegular() || st.Size() > 20<<30 {
			return s, errors.New("Die Intro-Skipper-Sicherung ist unvollständig.")
		}
	}
	return s, nil
}
func (o Options) snapshotIntro(ctx context.Context, programData, output string) (introSnapshot, error) {
	programData, e := filepath.EvalSymlinks(programData)
	if e != nil {
		return introSnapshot{}, e
	}
	c := exec.CommandContext(ctx, o.Server, "--mutti-intro-snapshot", programData, output)
	c.Env = cleanServerEnvironment(o.TargetOrigin, false)
	if e = c.Run(); e != nil {
		return introSnapshot{}, errors.New("Intro Skipper konnte nicht konsistent gesichert werden. Bitte Dateizugriff prüfen und laufende Analysen abwarten.")
	}
	return readIntroSnapshot(output)
}
func (o Options) sourceIntro(ctx context.Context, s *Source, archive, output string) (introSnapshot, error) {
	if s.Local && locallyReadable(s.Info.ProgramDataPath) {
		return o.snapshotIntro(ctx, s.Info.ProgramDataPath, output)
	}
	z, e := zip.OpenReader(archive)
	if e != nil {
		return introSnapshot{}, e
	}
	defer z.Close()
	if e = validateArchive(z); e != nil {
		return introSnapshot{}, e
	}
	if e = os.Mkdir(output, 0700); e != nil {
		return introSnapshot{}, e
	}
	for _, entry := range z.File {
		if !strings.HasPrefix(entry.Name, "Mutti/IntroSkipper/") {
			continue
		}
		name := strings.TrimPrefix(entry.Name, "Mutti/IntroSkipper/")
		if name != "snapshot.json" && !introFiles[name] {
			return introSnapshot{}, errors.New("Unbekannte Plugin-Sicherungsdatei.")
		}
		r, e := entry.Open()
		if e != nil {
			return introSnapshot{}, e
		}
		f, e := os.OpenFile(filepath.Join(output, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			r.Close()
			return introSnapshot{}, e
		}
		_, e = io.Copy(f, r)
		r.Close()
		closeErr := f.Close()
		if e != nil {
			return introSnapshot{}, e
		}
		if closeErr != nil {
			return introSnapshot{}, closeErr
		}
	}
	return readIntroSnapshot(output)
}
func restoreIntro(s introSnapshot, instance string, mappings map[string]string) (map[string]string, error) {
	expected := map[string]string{}
	for name, hash := range s.Hashes {
		source := filepath.Join(s.Directory, name)
		dest := filepath.Join(instance, "data", "data", "introskipper", name)
		if name == "IntroSkipper.xml" {
			dest = filepath.Join(instance, "data", "plugins", "configurations", name)
			b, e := os.ReadFile(source)
			if e != nil {
				return nil, e
			}
			// Remap media exclusions without changing detection/preferences.
			b, e = rewriteXML(b, mappings, nil)
			if e != nil {
				return nil, e
			}
			if e = privateWrite(dest, b); e != nil {
				return nil, e
			}
			h := sha256.Sum256(b)
			expected[name] = hex.EncodeToString(h[:])
			continue
		}
		if e := os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return nil, e
		}
		// The bootstrap process is stopped; discard only its generated sidecars.
		for _, suffix := range []string{"-wal", "-shm"} {
			if e := os.Remove(dest + suffix); e != nil && !os.IsNotExist(e) {
				return nil, e
			}
		}
		in, e := os.Open(source)
		if e != nil {
			return nil, e
		}
		out, e := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if e != nil {
			in.Close()
			return nil, e
		}
		_, e = io.Copy(out, in)
		in.Close()
		closeErr := out.Close()
		if e != nil {
			return nil, e
		}
		if closeErr != nil {
			return nil, closeErr
		}
		expected[name] = hash
	}
	return expected, nil
}
func compareIntro(expected, actual map[string]string, exact bool) error {
	if exact && len(expected) != len(actual) {
		return errors.New("Intro-Skipper-Daten wurden während des Umzugs geändert. Bitte laufende Analysen abwarten und erneut versuchen.")
	}
	for name, hash := range expected {
		if actual[name] != hash {
			return fmt.Errorf("Die Intro-Skipper-Datenprüfung für %s ist fehlgeschlagen. Es wurde nicht umgeschaltet.", name)
		}
	}
	return nil
}
func verifyIntroLoaded(ctx context.Context, api *API) error {
	var plugins []PluginInfo
	if e := api.call(ctx, "GET", "/Plugins", nil, &plugins); e != nil {
		return e
	}
	for _, p := range plugins {
		if samePluginID(p.Id, introID) && p.Version == introVersion && p.Status == "Active" {
			// The plugin must be usable, not merely listed as installed.
			deadline, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			for {
				var support struct {
					Sections []struct {
						Entries []struct{ Label, Value string }
					}
				}
				if e := api.call(deadline, "GET", "/IntroSkipper/SupportBundle/Json", nil, &support); e != nil {
					return e
				}
				for _, section := range support.Sections {
					for _, entry := range section.Entries {
						if entry.Label == "FFmpeg" && entry.Value == "okay" {
							return nil
						}
					}
				}
				select {
				case <-deadline.Done():
					return errors.New("Intro Skipper kann die mitgelieferte Audioanalyse noch nicht verwenden. Es wurde nicht umgeschaltet.")
				case <-time.After(250 * time.Millisecond):
				}
			}
		}
	}
	return errors.New("Das mitgelieferte Intro Skipper konnte nicht geladen werden. Der bisherige Datenstand bleibt aktiv.")
}
