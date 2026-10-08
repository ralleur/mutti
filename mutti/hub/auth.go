// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Identity is always derived from Jellyfin for each request; no client field
// can name another user.
type Identity struct {
	UserID  string
	Name    string
	Admin   bool
	Device  string
	token   string
	Policy  json.RawMessage
	checked time.Time
}

type jellyfin struct {
	base, host string
	client     *http.Client
	mu         sync.Mutex
	cache      map[string]Identity
}

var errUnauthorized = errors.New("Anmeldung abgelaufen oder Zugriff entfernt.")

func newJellyfin(base, host string) (*jellyfin, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || !net.ParseIP(u.Hostname()).IsLoopback() {
		return nil, errors.New("Jellyfin target must be a fixed loopback origin")
	}
	if host == "" {
		host = u.Host
	}
	return &jellyfin{base: strings.TrimSuffix(base, "/"), host: host, client: guardedClient(30 * time.Second), cache: map[string]Identity{}}, nil
}

var tokenPattern = regexp.MustCompile(`(?i)token="([^"]+)"`)

func requestToken(r *http.Request) string {
	if t := r.Header.Get("X-Emby-Token"); t != "" {
		return t
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if m := tokenPattern.FindStringSubmatch(auth); m != nil {
		return m[1]
	}
	return ""
}

func tokenKey(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// identify validates the token against Jellyfin. Cached results are reused
// for a few seconds only, so profile disabling and logout take effect quickly.
func (j *jellyfin) identify(ctx context.Context, token string, maxAge time.Duration) (Identity, error) {
	if token == "" || len(token) > 256 {
		return Identity{}, errUnauthorized
	}
	key := tokenKey(token)
	j.mu.Lock()
	cached, ok := j.cache[key]
	j.mu.Unlock()
	if ok && time.Since(cached.checked) < maxAge {
		return cached, nil
	}
	var user struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Policy struct {
			IsAdministrator bool `json:"IsAdministrator"`
			IsDisabled      bool `json:"IsDisabled"`
		} `json:"Policy"`
		RawPolicy json.RawMessage `json:"-"`
	}
	var raw map[string]json.RawMessage
	if err := j.call(ctx, "GET", "/Users/Me", token, nil, &raw); err != nil {
		j.forget(key)
		return Identity{}, errUnauthorized
	}
	b, _ := json.Marshal(raw)
	if json.Unmarshal(b, &user) != nil || user.ID == "" || user.Policy.IsDisabled {
		j.forget(key)
		return Identity{}, errUnauthorized
	}
	id := Identity{UserID: normalizeID(user.ID), Name: user.Name, Admin: user.Policy.IsAdministrator, token: token, Policy: raw["Policy"], checked: time.Now()}
	j.mu.Lock()
	if len(j.cache) > 512 {
		j.cache = map[string]Identity{}
	}
	j.cache[key] = id
	j.mu.Unlock()
	return id, nil
}

func (j *jellyfin) forget(key string) {
	j.mu.Lock()
	delete(j.cache, key)
	j.mu.Unlock()
}

// normalizeID makes dashed and undashed Jellyfin GUID spellings comparable.
func normalizeID(id string) string { return strings.ToLower(strings.ReplaceAll(id, "-", "")) }

func (j *jellyfin) call(ctx context.Context, method, path, token string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, j.base+path, body)
	if err != nil {
		return err
	}
	req.Host = j.host
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Client="Mutti Hub", Device="Mutti", DeviceId="mutti-hub", Version="0.1", Token="%s"`, token))
	res, err := j.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return errUnauthorized
	}
	if res.StatusCode == 404 || res.StatusCode == 400 {
		return errNotFound
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("jellyfin status %d", res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out)
}

// peerDevice accepts the device header only from the local Connect sidecar,
// which proves itself with the per-run secret shared by the manager.
func peerDevice(r *http.Request, secret string) string {
	if secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Mutti-Hub-Peer")), []byte(secret)) != 1 {
		return ""
	}
	device := r.Header.Get("X-Mutti-Device")
	if len(device) > 128 {
		return ""
	}
	return device
}
