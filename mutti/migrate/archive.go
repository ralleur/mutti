// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Audit struct {
	Tables   map[string][]string `json:"tables"`
	Files    map[string]string   `json:"files"`
	Counts   map[string]int      `json:"counts"`
	Settings map[string]string   `json:"settings"`
}

func newAudit() Audit {
	return Audit{map[string][]string{}, map[string]string{}, map[string]int{}, map[string]string{}}
}
func safeEntry(name string) bool {
	return name != "" && !strings.Contains(name, "\\") && !strings.Contains(name, "\x00") && !strings.HasPrefix(name, "/") && path.Clean(name) == strings.TrimSuffix(name, "/") && name != ".." && !strings.HasPrefix(name, "../") && !strings.Contains(strings.Split(name, "/")[0], ":")
}
func validateArchive(z *zip.ReadCloser) error {
	if len(z.File) > 500000 {
		return errors.New("Die Sicherung enthält zu viele Dateien.")
	}
	seen := map[string]bool{}
	var total uint64
	for _, f := range z.File {
		if !safeEntry(f.Name) || f.Mode()&os.ModeSymlink != 0 || seen[strings.ToLower(f.Name)] {
			return errors.New("Die Sicherung enthält unsichere oder doppelte Dateipfade.")
		}
		seen[strings.ToLower(f.Name)] = true
		if f.UncompressedSize64 > 20<<30 || total > (100<<30)-f.UncompressedSize64 {
			return errors.New("Die Sicherung überschreitet die Größe dieses Teststands.")
		}
		total += f.UncompressedSize64
	}
	f, e := z.Open("manifest.json")
	if e != nil {
		return errors.New("Keine Jellyfin-Sicherung: manifest.json fehlt.")
	}
	defer f.Close()
	var m struct {
		ServerVersion, BackupEngineVersion string
		Options                            struct{ Database bool }
		DatabaseTables                     []string
	}
	if json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&m) != nil || !strings.HasPrefix(m.ServerVersion, "12.1.") || m.BackupEngineVersion != "0.2.0" || !m.Options.Database {
		return errors.New("Diese Sicherung ist nicht mit dem geprüften Jellyfin-12.1-Import kompatibel.")
	}
	requiredEntries := []string{"Config/system.xml"}
	for _, table := range strings.Fields("AccessSchedules ActivityLogs ApiKeys Devices DeviceOptions DisplayPreferences ImageInfos ItemDisplayPreferences CustomItemDisplayPreferences Permissions Preferences Users TrickplayInfos MediaSegments UserData AncestorIds AttachmentStreamInfos BaseItems Chapters ItemValues ItemValuesMap MediaStreamInfos Peoples PeopleBaseItemMap LinkedChildren BaseItemProviders BaseItemImageInfos BaseItemMetadataFields BaseItemTrailerTypes KeyframeData HistoryRow") {
		requiredEntries = append(requiredEntries, "Database/"+table+".json")
	}
	for _, required := range requiredEntries {
		if !seen[strings.ToLower(required)] {
			return fmt.Errorf("Die Sicherung ist unvollständig: %s fehlt.", required)
		}
	}
	return nil
}
func replacePath(s string, mappings map[string]string) string {
	normalized := map[string]string{}
	for k, v := range mappings {
		normalized[strings.TrimRight(k, "/\\")] = v
	}
	mappings = normalized
	keys := make([]string, 0, len(mappings))
	for k := range mappings {
		if k != "" {
			keys = append(keys, strings.TrimRight(k, "/\\"))
		}
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, old := range keys {
		if s == old || strings.HasPrefix(s, old+"/") || strings.HasPrefix(s, old+"\\") {
			return filepath.ToSlash(mappings[old]) + strings.ReplaceAll(s[len(old):], "\\", "/")
		}
	}
	return s
}
func rewriteValue(v any, mappings map[string]string) any {
	switch t := v.(type) {
	case string:
		return replacePath(t, mappings)
	case []any:
		for i, x := range t {
			t[i] = rewriteValue(x, mappings)
		}
	case map[string]any:
		for k, x := range t {
			t[k] = rewriteValue(x, mappings)
		}
	}
	return v
}

var volatileTables = map[string]bool{"ActivityLogs": true, "Devices": true, "DeviceOptions": true, "ApiKeys": true}

func canonical(v any, table string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, a := range x {
			// Restore regenerates EF concurrency counters and the surrogate row
			// keys of user permissions/preferences. Compare their semantic
			// (UserId, Kind, Value) tuples, retaining every permission value.
			if (table == "Users" && k == "RowVersion") || ((table == "Permissions" || table == "Preferences") && k == "Id") {
				delete(x, k)
				continue
			}
			if table == "Users" && (k == "LastLoginDate" || k == "LastActivityDate") {
				delete(x, k)
				continue
			}
			x[k] = canonical(a, table)
		}
	case []any:
		for i, a := range x {
			x[i] = canonical(a, table)
		}
	case string:
		if t, e := time.Parse(time.RFC3339Nano, x); e == nil {
			return t.UTC().Format(time.RFC3339Nano)
		}
	}
	return v
}
func hashJSON(v any, table string) string {
	b, _ := json.Marshal(canonical(v, table))
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func transformJSON(r io.Reader, w io.Writer, mappings map[string]string, table string, a *Audit, clearSessions bool) error {
	d := json.NewDecoder(r)
	d.UseNumber()
	token, e := d.Token()
	if e != nil || token != json.Delim('[') {
		return errors.New("Ungültige Datenbanktabelle im Archiv.")
	}
	if _, e = io.WriteString(w, "["); e != nil {
		return e
	}
	a.Tables[table] = []string{}
	a.Counts[table] = 0
	first := true
	for d.More() {
		var row any
		if e = d.Decode(&row); e != nil {
			return e
		}
		row = rewriteValue(row, mappings)
		if table == "Users" {
			if obj, ok := row.(map[string]any); ok {
				if provider, ok := obj["AuthenticationProviderId"].(string); ok && provider != "" && provider != "Jellyfin.Server.Implementations.Users.DefaultAuthenticationProvider" {
					return errors.New("Ein Benutzer verwendet einen externen Anmeldedienst. Dessen Übernahme muss zuerst eingerichtet werden.")
				}
			}
		}
		if clearSessions && (table == "Devices" || table == "DeviceOptions" || table == "ApiKeys") {
			continue
		}
		if !first {
			_, e = io.WriteString(w, ",")
			if e != nil {
				return e
			}
		}
		first = false
		b, e := json.Marshal(row)
		if e != nil {
			return e
		}
		if _, e = w.Write(b); e != nil {
			return e
		}
		a.Counts[table]++
		if !volatileTables[table] {
			a.Tables[table] = append(a.Tables[table], hashJSON(row, table))
		}
	}
	if _, e = d.Token(); e != nil {
		return e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("Unerwartete Daten nach der Datenbanktabelle.")
	}
	_, e = io.WriteString(w, "]")
	sort.Strings(a.Tables[table])
	return e
}
func rewriteXML(data []byte, mappings map[string]string, overrides map[string]string) ([]byte, error) {
	d := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	var b bytes.Buffer
	e := xml.NewEncoder(&b)
	depth := 0
	skip := 0
	seenOverrides := map[string]bool{}
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.Directive:
			return nil, errors.New("XML-Direktiven sind in einer Importdatei nicht erlaubt.")
		case xml.StartElement:
			depth++
			if skip > 0 {
				continue
			}
			if v, ok := overrides[t.Name.Local]; ok {
				seenOverrides[t.Name.Local] = true
				if err = e.EncodeToken(t); err != nil {
					return nil, err
				}
				if err = e.EncodeToken(xml.CharData(v)); err != nil {
					return nil, err
				}
				if err = e.EncodeToken(t.End()); err != nil {
					return nil, err
				}
				skip = depth
				continue
			}
		case xml.EndElement:
			if depth == 1 && skip == 0 {
				keys := []string{}
				for key := range overrides {
					if !seenOverrides[key] {
						keys = append(keys, key)
					}
				}
				sort.Strings(keys)
				for _, key := range keys {
					if err = e.EncodeElement(overrides[key], xml.StartElement{Name: xml.Name{Local: key}}); err != nil {
						return nil, err
					}
				}
			}
			if skip > 0 {
				if depth == skip {
					skip = 0
				}
				depth--
				continue
			}
			depth--
		case xml.CharData:
			if skip == 0 {
				token = xml.CharData(replacePath(string(t), mappings))
			}
		}
		if skip == 0 {
			if err = e.EncodeToken(token); err != nil {
				return nil, err
			}
		}
	}
	if err := e.Flush(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func networkXML(port int, bind string) []byte {
	return []byte(fmt.Sprintf(`<NetworkConfiguration><InternalHttpPort>%d</InternalHttpPort><PublicHttpPort>%d</PublicHttpPort><EnableRemoteAccess>false</EnableRemoteAccess><AutoDiscovery>false</AutoDiscovery><EnableIPv6>false</EnableIPv6><BaseUrl></BaseUrl><LocalNetworkAddresses><string>%s</string></LocalNetworkAddresses></NetworkConfiguration>`, port, port, bind))
}

// importPathMappings is shared by archive rewriting and library verification.
// Jellyfin expands virtual paths in its API, so internal collections need the
// same relocation as persisted database rows, XML and shortcut files.
func importPathMappings(source SystemInfo, instance string, mediaMappings map[string]string) map[string]string {
	mappings := map[string]string{}
	for k, v := range mediaMappings {
		mappings[k] = v
	}
	if source.ProgramDataPath != "" {
		mappings[strings.TrimRight(source.ProgramDataPath, "/\\")] = filepath.Join(instance, "data")
	}
	if source.InternalMetadataPath != "" {
		mappings[strings.TrimRight(source.InternalMetadataPath, "/\\")] = filepath.Join(instance, "data", "metadata")
	}
	if source.CachePath != "" {
		mappings[strings.TrimRight(source.CachePath, "/\\")] = filepath.Join(instance, "cache")
	}
	return mappings
}
func TransformArchive(input, output, instance string, port int, source SystemInfo, mediaMappings map[string]string, ffmpegPaths ...string) (Audit, error) {
	a := newAudit()
	z, e := zip.OpenReader(input)
	if e != nil {
		return a, errors.New("Die Sicherung konnte nicht geöffnet werden.")
	}
	defer z.Close()
	if e = validateArchive(z); e != nil {
		return a, e
	}
	mappings := importPathMappings(source, instance, mediaMappings)
	out, e := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return a, e
	}
	defer out.Close()
	w := zip.NewWriter(out)
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Curated plugin payload is restored and audited separately, never executed from the source.
		if strings.HasPrefix(f.Name, "Mutti/IntroSkipper/") {
			name := strings.TrimPrefix(f.Name, "Mutti/IntroSkipper/")
			if name != "snapshot.json" && !introFiles[name] {
				return a, errors.New("Unbekannte Plugin-Sicherungsdatei.")
			}
			continue
		}
		// Platform/database/log destinations are controlled by Mutti. No imported
		// connection string, log sink, plugin executable or network listener runs.
		if f.Name == "Config/database.xml" || f.Name == "Config/logging.json" || f.Name == "Config/network.xml" {
			continue
		}
		if !(f.Name == "manifest.json" || strings.HasPrefix(f.Name, "Database/") || strings.HasPrefix(f.Name, "Config/") || strings.HasPrefix(f.Name, "Root/") || strings.HasPrefix(f.Name, "Data/")) {
			return a, errors.New("Unbekannter Archivbereich.")
		}
		name := strings.ToLower(f.Name)
		if strings.HasSuffix(name, ".dll") || strings.HasSuffix(name, ".so") || strings.HasSuffix(name, ".dylib") || strings.Contains(name, "/plugins/") {
			return a, errors.New("Plugin-Programme werden nicht ungeprüft importiert.")
		}
		r, err := f.Open()
		if err != nil {
			return a, err
		}
		writer, err := w.Create(f.Name)
		if err != nil {
			r.Close()
			return a, err
		}
		digest := sha256.New()
		writer = io.MultiWriter(writer, digest)
		switch {
		case strings.HasPrefix(f.Name, "Database/"):
			table := strings.TrimSuffix(path.Base(f.Name), ".json")
			err = transformJSON(r, writer, mappings, table, &a, true)
		case strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".mblink"):
			var data []byte
			data, err = io.ReadAll(io.LimitReader(r, (16<<20)+1))
			if len(data) > 16<<20 {
				return a, errors.New("Eine Konfigurationsdatei ist zu groß.")
			}
			if err == nil {
				if strings.HasSuffix(name, ".mblink") {
					data = []byte(replacePath(string(data), mappings))
				} else {
					overrides := map[string]string{}
					if f.Name == "Config/system.xml" {
						overrides = map[string]string{"IsStartupWizardCompleted": "true", "CachePath": filepath.Join(instance, "cache"), "MetadataPath": filepath.Join(instance, "data", "metadata"), "EnableAutoUpdate": "false", "EnableAutomaticRestart": "false"}
					}
					if f.Name == "Config/encoding.xml" {
						ffmpeg := ""
						if len(ffmpegPaths) > 0 {
							ffmpeg = ffmpegPaths[0]
						}
						overrides = map[string]string{"EncoderAppPath": ffmpeg, "EncoderAppPathDisplay": ffmpeg, "TranscodingTempPath": filepath.Join(instance, "cache", "transcodes"), "HardwareAccelerationType": "none"}
					}
					data, err = rewriteXML(data, mappings, overrides)
				}
				if err == nil {
					_, err = writer.Write(data)
				}
			}
		default:
			_, err = io.Copy(writer, r)
		}
		r.Close()
		if err != nil {
			return a, err
		}
		if strings.HasPrefix(f.Name, "Config/") && !strings.HasPrefix(f.Name, "Config/ScheduledTasks/") {
			a.Settings[f.Name] = hex.EncodeToString(digest.Sum(nil))
		}
		if strings.HasPrefix(f.Name, "Root/") || (strings.HasPrefix(f.Name, "Data/") && !strings.HasPrefix(f.Name, "Data/ScheduledTasks/")) {
			a.Files[f.Name] = hex.EncodeToString(digest.Sum(nil))
		}
	}
	nw, e := w.Create("Config/network.xml")
	if e != nil {
		return a, e
	}
	if _, e = nw.Write(networkXML(port, "127.0.0.1")); e != nil {
		return a, e
	}
	if e = w.Close(); e != nil {
		return a, e
	}
	if e = out.Sync(); e != nil {
		return a, e
	}
	return a, nil
}
func ReadAudit(archive string) (Audit, error) {
	a := newAudit()
	z, e := zip.OpenReader(archive)
	if e != nil {
		return a, e
	}
	defer z.Close()
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, "Database/") && strings.HasSuffix(f.Name, ".json") {
			r, e := f.Open()
			if e != nil {
				return a, e
			}
			table := strings.TrimSuffix(path.Base(f.Name), ".json")
			e = transformJSON(r, io.Discard, nil, table, &a, false)
			r.Close()
			if e != nil {
				return a, e
			}
		}
	}
	return a, nil
}
func CompareAudit(expected, actual Audit) error {
	for table, hashes := range expected.Tables {
		if volatileTables[table] || table == "HistoryRow" {
			continue
		}
		other, exists := actual.Tables[table]
		if !exists {
			return fmt.Errorf("Die Tabelle %s fehlt nach der Wiederherstellung.", table)
		}
		if len(hashes) != len(other) {
			return fmt.Errorf("Die Prüfung der Tabelle %s ist fehlgeschlagen: Anzahl stimmt nicht überein.", table)
		}
		for i, h := range hashes {
			if h != other[i] {
				return fmt.Errorf("Die Prüfung der Tabelle %s ist fehlgeschlagen: Inhalte stimmen nicht überein.", table)
			}
		}
	}
	return nil
}

// Verify the restored non-database payload as well as the semantic database.
func VerifyFiles(a Audit, instance string) error {
	for name, expected := range a.Files {
		var dest string
		switch {
		case strings.HasPrefix(name, "Root/"):
			dest = filepath.Join(instance, "data", "root", strings.TrimPrefix(name, "Root/"))
		case strings.HasPrefix(name, "Data/metadata/"):
			dest = filepath.Join(instance, "data", "metadata", strings.TrimPrefix(name, "Data/metadata/"))
		case strings.HasPrefix(name, "Data/metadata-default/"):
			dest = filepath.Join(instance, "data", "metadata", strings.TrimPrefix(name, "Data/metadata-default/"))
		case strings.HasPrefix(name, "Data/"):
			dest = filepath.Join(instance, "data", "data", strings.TrimPrefix(name, "Data/"))
		default:
			return errors.New("Unbekannter Prüfpfad.")
		}
		resolved, e := filepath.EvalSymlinks(dest)
		root, _ := filepath.EvalSymlinks(instance)
		if e != nil || !within(root, resolved) {
			return errors.New("Eine übernommene Datei fehlt oder liegt außerhalb der Importinstanz.")
		}
		f, e := os.Open(dest)
		if e != nil {
			return e
		}
		h := sha256.New()
		_, e = io.Copy(h, f)
		f.Close()
		if e != nil || hex.EncodeToString(h.Sum(nil)) != expected {
			return fmt.Errorf("Die übernommene Datei %s stimmt nicht mit der Sicherung überein.", name)
		}
	}
	return nil
}
