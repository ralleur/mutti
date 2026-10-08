// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Module identifiers are stable contract values shared with mutti-web and kurtz.
const (
	ModuleAI        = "ai"
	ModulePhotos    = "photos"
	ModuleDocuments = "documents"
)

var moduleIDs = []string{ModuleAI, ModulePhotos, ModuleDocuments}

// Link maps one Jellyfin profile to its own account in a service. Secret is a
// profile-scoped API key or token, never an administrator credential.
type Link struct {
	Account string    `json:"account"`
	Secret  string    `json:"secret"`
	KeyID   string    `json:"keyId,omitempty"`
	Remote  string    `json:"remoteUser,omitempty"`
	Created time.Time `json:"created"`
}

type Engine struct {
	// "managed" runs this package's own network-restricted engine; "external"
	// uses an owner-provided engine on loopback or a private network.
	Mode string `json:"mode"`
	URL  string `json:"url,omitempty"`
}

type ModuleConfig struct {
	Enabled    bool             `json:"enabled"`
	Grants     map[string]bool  `json:"grants"`
	ServiceURL string           `json:"serviceUrl,omitempty"`
	Links      map[string]*Link `json:"links,omitempty"`
	Engine     *Engine          `json:"engine,omitempty"`
	Model      string           `json:"model,omitempty"`
	// Instance identifies the backend behind ServiceURL. Pointing the module
	// at another service creates a new instance, so earlier content IDs never
	// resolve to objects of a replaced backend.
	Instance string `json:"instance,omitempty"`
}

type Config struct {
	Version int                      `json:"version"`
	Modules map[string]*ModuleConfig `json:"modules"`
	// MuttiID names this installation in content references; MediaInstance
	// is the identity of its own media library.
	MuttiID       string `json:"muttiId,omitempty"`
	MediaInstance string `json:"mediaInstance,omitempty"`
}

// Store persists owner configuration privately and atomically.
type Store struct {
	mu     sync.Mutex
	path   string
	config Config
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "hub.json")}
	b, err := os.ReadFile(s.path)
	switch {
	case os.IsNotExist(err):
		s.config = Config{Version: 1, Modules: map[string]*ModuleConfig{}}
	case err != nil:
		return nil, err
	default:
		if err = json.Unmarshal(b, &s.config); err != nil || s.config.Version != 1 {
			return nil, errors.New("invalid hub configuration")
		}
	}
	normalize(&s.config)
	// Content identities are created once and persisted before first use.
	if missingIdentity(s.config) {
		if err := s.Update(func(c *Config) error { assignIdentity(c); return nil }); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func missingIdentity(c Config) bool {
	if c.MuttiID == "" || c.MediaInstance == "" {
		return true
	}
	for _, m := range c.Modules {
		if m.ServiceURL != "" && m.Instance == "" {
			return true
		}
	}
	return false
}

func assignIdentity(c *Config) {
	if c.MuttiID == "" {
		c.MuttiID = randomID()
	}
	if c.MediaInstance == "" {
		c.MediaInstance = randomID()
	}
	for _, m := range c.Modules {
		if m.ServiceURL != "" && m.Instance == "" {
			m.Instance = randomID()
		}
	}
}

// normalize guarantees non-nil maps; JSON round trips drop empty ones.
func normalize(c *Config) {
	if c.Modules == nil {
		c.Modules = map[string]*ModuleConfig{}
	}
	for _, id := range moduleIDs {
		m := c.Modules[id]
		if m == nil {
			m = &ModuleConfig{}
			c.Modules[id] = m
		}
		if m.Grants == nil {
			m.Grants = map[string]bool{}
		}
		if m.Links == nil {
			m.Links = map[string]*Link{}
		}
		if id == ModuleAI && m.Engine == nil {
			m.Engine = &Engine{Mode: "managed"}
		}
	}
}

// Read returns a deep copy so callers can never mutate state without Update.
func (s *Store) Read() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneConfig(s.config)
}

// Update applies fn to a copy and persists it before it becomes visible.
func (s *Store) Update(fn func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneConfig(s.config)
	if err := fn(&next); err != nil {
		return err
	}
	if err := writePrivateJSON(s.path, next); err != nil {
		return err
	}
	s.config = next
	return nil
}

func cloneConfig(c Config) Config {
	b, _ := json.Marshal(c)
	var out Config
	_ = json.Unmarshal(b, &out)
	normalize(&out)
	return out
}

func writePrivateJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writePrivate(path, b)
}

func writePrivate(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".hub-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	return err
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
