// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func randomID() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}

var noRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

type API struct {
	Base                *url.URL
	Host, Token, Device string
	Client              *http.Client
}

func NewAPI(address string) (*API, error) {
	u, e := url.Parse(strings.TrimRight(strings.TrimSpace(address), "/"))
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("Bitte eine gültige Serveradresse ohne Zugangsdaten eingeben.")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
		return nil, errors.New("Für entfernte Server ist eine HTTPS-Adresse erforderlich. Lokal ist http://127.0.0.1 erlaubt.")
	}
	if strings.Contains(u.Path, "..") || strings.Contains(u.Path, "\\") {
		return nil, errors.New("Ungültiger Serverpfad.")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &API{Base: u, Device: "mutti-import-" + randomID(), Client: &http.Client{Transport: transport, Timeout: 30 * time.Minute, CheckRedirect: noRedirect}}, nil
}
func (a *API) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		reader = bytes.NewReader(b)
	}
	u := *a.Base
	relative, e := url.Parse(path)
	if e != nil {
		return nil, e
	}
	u.Path = strings.TrimRight(u.Path, "/") + relative.Path
	u.RawQuery = relative.RawQuery
	req, e := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	auth := fmt.Sprintf(`MediaBrowser Client="Mutti Import", Device="Mutti", DeviceId="%s", Version="0.1.0"`, a.Device)
	if a.Token != "" {
		auth += `, Token="` + a.Token + `"`
	}
	req.Header.Set("Authorization", auth)
	if a.Host != "" {
		req.Host = a.Host
	}
	return a.Client.Do(req)
}
func (a *API) call(ctx context.Context, method, path string, body, out any) error {
	r, e := a.request(ctx, method, path, body)
	if e != nil {
		return errors.New("Der Server ist nicht erreichbar. Adresse und Verbindung prüfen.")
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return fmt.Errorf("Der Server hat die Anfrage abgelehnt (HTTP %d).", r.StatusCode)
	}
	if out == nil {
		_, e = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))
		return e
	}
	return json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(out)
}

type PublicInfo struct {
	Id, ServerName, Version string
	StartupWizardCompleted  bool
}
type SystemInfo struct {
	PublicInfo
	ProgramDataPath, CachePath, InternalMetadataPath string
}
type User struct {
	Id, Name    string
	Policy      struct{ IsAdministrator, IsDisabled bool }
	HasPassword bool
}
type Library struct {
	Name, ItemId   string
	Locations      []string
	LibraryOptions json.RawMessage
}
type Source struct {
	API                *API
	Info               SystemInfo
	Users              []User
	Libraries          []Library
	Local              bool
	Username, Password string
}
type SourceInput struct {
	Address, Username, Password, TargetUsername, TargetPassword string
	Mappings                                                    map[string]string
	Replace                                                     bool
	Archive                                                     string
}

func login(ctx context.Context, a *API, name, password string) (User, error) {
	var auth struct {
		AccessToken string
		User        User
	}
	if e := a.call(ctx, "POST", "/Users/AuthenticateByName", map[string]string{"Username": name, "Pw": password}, &auth); e != nil {
		return User{}, errors.New("Anmeldung fehlgeschlagen. Bitte den Administratorzugang dieser Instanz verwenden.")
	}
	a.Token = auth.AccessToken
	if !auth.User.Policy.IsAdministrator || auth.User.Policy.IsDisabled || a.Token == "" {
		a.logout()
		return User{}, errors.New("Für die Übernahme ist ein aktiver Administratorzugang erforderlich.")
	}
	return auth.User, nil
}
func (a *API) logout() {
	if a.Token == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = a.call(ctx, "POST", "/Sessions/Logout", nil, nil)
	a.Token = ""
	a.Client.CloseIdleConnections()
}
func OpenSource(ctx context.Context, input SourceInput, targetID string) (*Source, error) {
	a, e := NewAPI(input.Address)
	if e != nil {
		return nil, e
	}
	s := &Source{API: a, Username: input.Username, Password: input.Password}
	if _, e = login(ctx, a, input.Username, input.Password); e != nil {
		return nil, e
	}
	fail := func(err error) (*Source, error) { a.logout(); return nil, err }
	if e = a.call(ctx, "GET", "/System/Info", nil, &s.Info); e != nil {
		return fail(e)
	}
	if !s.Info.StartupWizardCompleted || s.Info.Id == "" || s.Info.Id == targetID {
		return fail(errors.New("Bitte einen fertig eingerichteten anderen Jellyfin-Server auswählen."))
	}
	if !strings.HasPrefix(s.Info.Version, "12.1.") {
		return fail(errors.New("Dieser Teststand übernimmt Jellyfin 12.1. Für diese Serverversion ist die vollständige Übernahme noch nicht geprüft."))
	}
	ip := net.ParseIP(a.Base.Hostname())
	s.Local = a.Base.Hostname() == "localhost" || ip != nil && ip.IsLoopback()
	if e = a.call(ctx, "GET", "/Users", nil, &s.Users); e != nil {
		return fail(e)
	}
	if e = a.call(ctx, "GET", "/Library/VirtualFolders", nil, &s.Libraries); e != nil {
		return fail(e)
	}
	return s, nil
}
func (s *Source) Backup(ctx context.Context) (string, error) {
	var result struct {
		Path          string
		ServerVersion string
	}
	if e := s.API.call(ctx, "POST", "/Backup/Create", map[string]bool{"Metadata": true, "Subtitles": true, "Trickplay": true}, &result); e != nil {
		return "", fmt.Errorf("Sicherung konnte nicht erstellt werden. Prüfe freien Speicher und warte auf laufende Bibliotheksscans: %w", e)
	}
	root, e := filepath.EvalSymlinks(s.Info.ProgramDataPath)
	if e != nil || !filepath.IsAbs(root) {
		return "", errors.New("Der Jellyfin-Datenordner ist lokal nicht lesbar.")
	}
	archive, e := filepath.EvalSymlinks(result.Path)
	if e != nil {
		return "", errors.New("Die fertige Sicherung ist auf diesem Rechner nicht lesbar. Bei Docker muss das Quellvolume lesbar eingebunden sein.")
	}
	expected := filepath.Join(root, "data", "backups")
	if !within(expected, archive) || !strings.HasSuffix(strings.ToLower(archive), ".zip") {
		return "", errors.New("Der Sicherungspfad gehört nicht zum ausgewählten Jellyfin-Datenordner.")
	}
	st, e := os.Stat(archive)
	if e != nil || !st.Mode().IsRegular() {
		return "", errors.New("Die Sicherung ist keine lesbare reguläre Datei.")
	}
	// This archive was just created for the authorized transfer and contains
	// account hashes. Restrict it when the source process shares our owner.
	if e = os.Chmod(archive, 0600); e != nil {
		return "", errors.New("Die neue Sicherung konnte nicht auf private Dateirechte begrenzt werden.")
	}
	return archive, nil
}
func within(root, path string) bool {
	r, e := filepath.Rel(root, path)
	return e == nil && r != "." && r != ".." && !strings.HasPrefix(r, ".."+string(os.PathSeparator)) && !filepath.IsAbs(r)
}
func Discover(ctx context.Context) []map[string]string {
	ports := map[string]bool{"8096": true, "8097": true}
	if runtime.GOOS == "darwin" {
		out, _ := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-a", "-c", "jellyfin", "-iTCP", "-sTCP:LISTEN", "-Fn").Output()
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "n") {
				_, p, e := net.SplitHostPort(strings.TrimPrefix(line, "n"))
				if e == nil {
					ports[p] = true
				}
			}
		}
	}
	result := []map[string]string{}
	for port := range ports {
		if p, e := strconv.Atoi(port); e != nil || p < 1 || p > 65535 {
			continue
		}
		a, _ := NewAPI("http://127.0.0.1:" + port)
		c, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		var info PublicInfo
		e := a.call(c, "GET", "/System/Info/Public", nil, &info)
		cancel()
		a.Client.CloseIdleConnections()
		if e == nil && info.Id != "" && info.StartupWizardCompleted {
			result = append(result, map[string]string{"id": info.Id, "address": a.Base.String(), "name": info.ServerName, "version": info.Version})
		}
	}
	return result
}
