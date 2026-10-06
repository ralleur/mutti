// SPDX-License-Identifier: MPL-2.0
package connect

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
)

type Device struct {
	Pin     string          `json:"pin"`
	Name    string          `json:"name"`
	UserID  string          `json:"userId"`
	Token   string          `json:"token"`
	User    json.RawMessage `json:"user"`
	Created time.Time       `json:"created"`
}
type serverState struct {
	Identity Identity          `json:"identity"`
	Owner    string            `json:"owner"`
	Devices  map[string]Device `json:"devices"`
}
type pending struct {
	Pin     string    `json:"pin"`
	Name    string    `json:"name"`
	Expires time.Time `json:"expires"`
}
type invite struct {
	Hash    string
	Expires time.Time
	Pin     string
}
type Server struct {
	mu                               sync.Mutex
	state                            serverState
	path                             string
	Broker, STUN, Target, TargetHost string
	invites                          map[string]*invite
	pending                          map[string]pending
	sessions                         map[string]map[*yamux.Session]bool
	active                           chan struct{}
	targetHTTP                       *http.Client
	policies                         map[string]string
}

func NewServer(directory, broker, stun, target, host string) (*Server, error) {
	s := &Server{path: filepath.Join(directory, "connect.json"), Broker: strings.TrimSuffix(broker, "/"), STUN: stun, Target: target, TargetHost: host, invites: map[string]*invite{}, pending: map[string]pending{}, sessions: map[string]map[*yamux.Session]bool{}, active: make(chan struct{}, 32)}
	b, e := os.ReadFile(s.path)
	if os.IsNotExist(e) {
		s.state.Identity, e = NewIdentity()
		s.state.Owner = randomID()
		s.state.Devices = map[string]Device{}
		if e == nil {
			e = savePrivate(s.path, s.state)
		}
	} else if e == nil {
		e = json.Unmarshal(b, &s.state)
	}
	if e != nil {
		return nil, e
	}
	if !validID(s.state.Identity.Pin()) || !validID(s.state.Owner) || s.state.Devices == nil {
		return nil, errors.New("invalid server identity store")
	}
	i := Invitation{Version: 1, Broker: broker, STUN: stun, Pin: s.state.Identity.Pin(), Room: digest(s.state.Owner)}
	if e = i.Validate(); e != nil {
		return nil, e
	}
	t, e := url.Parse(target)
	if e != nil || t.Scheme != "http" || !net.ParseIP(t.Hostname()).IsLoopback() || t.User != nil || t.Path != "" {
		return nil, errors.New("Jellyfin target must be fixed loopback HTTP origin")
	}
	s.targetHTTP = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect rejected") }}
	return s, nil
}
func (s *Server) NewInvitation() (Invitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.prune(now)
	if len(s.invites) >= 8 {
		return Invitation{}, errors.New("Bitte bestehende QR-Codes ablaufen lassen.")
	}
	secret := randomID()
	expires := now.Add(5 * time.Minute)
	s.invites[digest(secret)] = &invite{Hash: digest(secret), Expires: expires}
	return Invitation{1, s.Broker, digest(s.state.Owner), s.state.Identity.Pin(), secret, s.STUN, expires.Unix()}, nil
}
func (s *Server) prune(now time.Time) {
	for k, v := range s.invites {
		if now.After(v.Expires) {
			delete(s.invites, k)
		}
	}
	for k, v := range s.pending {
		if now.After(v.Expires) {
			delete(s.pending, k)
		}
	}
}
func (s *Server) Run(ctx context.Context) {
	for ctx.Err() == nil {
		var o Offer
		status, e := signal(ctx, s.Broker, "/v1/poll/"+digest(s.state.Owner), s.state.Owner, nil, &o)
		if e != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		if status != 200 {
			continue
		}
		select {
		case s.active <- struct{}{}:
			go func() { defer func() { <-s.active }(); s.accept(ctx, o) }()
		default:
		}
	}
}
func (s *Server) accept(parent context.Context, o Offer) {
	ctx, cancel := context.WithTimeout(parent, 40*time.Second)
	defer cancel()
	p, e := newPeer(s.STUN)
	if e != nil {
		return
	}
	defer p.Close()
	if e = p.remote(o.SDP); e != nil {
		return
	}
	answer, e := p.local(ctx, false)
	if e != nil {
		return
	}
	if _, e = signal(ctx, s.Broker, "/v1/answer/"+o.ID, "", answer, nil); e != nil {
		return
	}
	session, pin, e := p.secure(ctx, s.state.Identity, "", true)
	if e != nil {
		return
	}
	defer session.Close()
	s.mu.Lock()
	if s.sessions[pin] == nil {
		s.sessions[pin] = map[*yamux.Session]bool{}
	}
	s.sessions[pin][session] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.sessions[pin], session)
		if len(s.sessions[pin]) == 0 {
			delete(s.sessions, pin)
		}
		s.mu.Unlock()
	}()
	life, cancelLife := context.WithCancel(parent)
	defer cancelLife()
	go s.guardSession(life, pin, session)
	go func() {
		select {
		case <-session.CloseChan():
		case <-life.Done():
			_ = session.Close()
		}
	}()
	// An unapproved connection has a short lifetime even if its TLS handshake is valid.
	go func() {
		select {
		case <-time.After(6 * time.Minute):
			s.mu.Lock()
			_, ok := s.state.Devices[pin]
			s.mu.Unlock()
			if !ok {
				_ = session.Close()
			}
		case <-life.Done():
		}
	}()
	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.remoteRequest(pin, w, r) }), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 45 * time.Second, MaxHeaderBytes: 32768}
	_ = httpServer.Serve(&boundedListener{Listener: session, slots: make(chan struct{}, 16)})
}
func (s *Server) remoteRequest(pin string, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	if r.URL.IsAbs() || r.Method == "CONNECT" || r.Host != "mutti.internal" || r.Header.Get("Origin") != "" {
		http.Error(w, "forbidden", 403)
		return
	}
	if r.URL.Path == "/_mutti/pair" {
		s.pair(pin, w, r)
		return
	}
	s.mu.Lock()
	device, approved := s.state.Devices[pin]
	s.mu.Unlock()
	if !approved {
		http.Error(w, "Gerät ist nicht freigegeben oder wurde gesperrt.", 403)
		return
	}
	var currentUser jellyUser
	if s.jf(r.Context(), "GET", "/Users/Me", device.Token, nil, &currentUser) != nil || currentUser.ID != device.UserID || currentUser.Policy.IsAdministrator || currentUser.Policy.IsDisabled {
		http.Error(w, "Wiedergabeprofil nicht verfügbar.", 403)
		return
	}
	s.observePolicy(pin, currentUser.policySnapshot)
	if r.URL.Path == "/Users/AuthenticateWithQuickConnect" && r.Method == "POST" {
		// Network identity has already been proved. Issue only this device's profile;
		// the upstream Jellyfin bearer token never leaves the server sidecar.
		jsonReply(w, 200, map[string]any{"AccessToken": "mutti-device-bound", "User": device.User, "ServerId": s.serverID()})
		return
	}
	lower := strings.ToLower(r.URL.Path)
	if strings.HasPrefix(lower, "/_mutti") || strings.HasPrefix(lower, "/mutti") || strings.HasPrefix(lower, "/quickconnect") || strings.Contains(lower, "authenticate") || strings.HasPrefix(lower, "/users/new") || strings.HasPrefix(lower, "/startup") || strings.HasPrefix(lower, "/web") || strings.HasPrefix(lower, "/system/shutdown") || strings.HasPrefix(lower, "/system/restart") {
		http.Error(w, "forbidden", 403)
		return
	}
	target, _ := url.Parse(s.Target)
	proxy := &httputil.ReverseProxy{ErrorLog: log.New(io.Discard, "", 0), Rewrite: func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		pr.Out.Host = s.TargetHost
		pr.Out.Header.Del("Origin")
		pr.Out.Header.Del("Forwarded")
		pr.Out.Header.Del("X-Forwarded-For")
		pr.Out.Header.Del("X-Forwarded-Host")
		pr.Out.Header.Del("X-Forwarded-Proto")
		pr.Out.Header.Del("Authorization")
		pr.Out.Header.Del("Cookie")
		pr.Out.Header.Set("Accept-Encoding", "identity")
		pr.Out.Header.Del("X-Emby-Authorization")
		pr.Out.Header.Set("Authorization", `MediaBrowser Client="kurtz via Mutti", Device="Paired device", DeviceId="`+device.Pin+`", Version="0.1", Token="`+device.Token+`"`)
		pr.Out.Header.Del("X-Emby-Token")
		pr.Out.Header.Del("X-MediaBrowser-Token")
		q := pr.Out.URL.Query()
		for k := range q {
			switch strings.ToLower(k) {
			case "api_key", "apikey", "token", "access_token":
				q.Del(k)
			}
		}
		q.Set("ApiKey", device.Token)
		pr.Out.URL.RawQuery = q.Encode()
	}, Transport: s.targetHTTP.Transport, FlushInterval: -1, ModifyResponse: func(res *http.Response) error {
		if res.StatusCode >= 300 && res.StatusCode < 400 {
			return errors.New("redirect blocked")
		}
		res.Header.Del("Set-Cookie")
		return rewriteResponse(res, "http://mutti.internal", device.Token)
	}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, "Mutti-Mediendienst nicht erreichbar.", 502)
	}}
	proxy.ServeHTTP(w, r)
}
func (s *Server) pair(pin string, w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var req struct {
		Secret string `json:"secret"`
		Name   string `json:"name"`
	}
	if decodeBody(w, r, &req) != nil || len(req.Name) > 80 || len(req.Name) == 0 || !validID(req.Secret) {
		http.Error(w, "invalid pairing", 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(time.Now())
	// Repeated polling by the same key is idempotent, but possession of a consumed
	// invite never authorizes a different key.
	if _, ok := s.state.Devices[pin]; ok {
		jsonReply(w, 200, map[string]string{"state": "approved", "pin": pin})
		return
	}
	inv := s.invites[digest(req.Secret)]
	if inv == nil || (inv.Pin != "" && inv.Pin != pin) {
		http.Error(w, "QR-Code abgelaufen oder bereits verwendet.", 410)
		return
	}
	inv.Pin = pin
	s.pending[pin] = pending{pin, req.Name, inv.Expires}
	jsonReply(w, 202, map[string]string{"state": "pending", "pin": pin})
}
func (s *Server) jf(ctx context.Context, method, path, token string, body io.Reader, out any) error {
	return s.jfDevice(ctx, method, path, token, "mutti-connect", body, out)
}
func (s *Server) jfDevice(ctx context.Context, method, path, token, deviceID string, body io.Reader, out any) error {
	req, e := http.NewRequestWithContext(ctx, method, s.Target+path, body)
	if e != nil {
		return e
	}
	req.Host = s.TargetHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Emby-Token", token)
	authorization := `MediaBrowser Client="Mutti Connect", Device="Mutti", DeviceId="` + deviceID + `", Version="0.1"`
	if token != "" {
		authorization += `, Token="` + token + `"`
	}
	req.Header.Set("Authorization", authorization)
	res, e := s.targetHTTP.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return errors.New("Jellyfin hat die Anfrage abgelehnt.")
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(out)
	}
	return nil
}
func (s *Server) serverID() string {
	var data struct {
		ID string `json:"Id"`
	}
	_ = s.jf(context.Background(), "GET", "/System/Info/Public", "", nil, &data)
	return data.ID
}

// Bound HTTP streams per authenticated transport, including unapproved peers.
type boundedListener struct {
	net.Listener
	slots chan struct{}
}

func (l *boundedListener) Accept() (net.Conn, error) {
	for {
		conn, e := l.Listener.Accept()
		if e != nil {
			return nil, e
		}
		select {
		case l.slots <- struct{}{}:
			return &boundedConn{Conn: conn, release: func() { <-l.slots }}, nil
		default:
			_ = conn.Close()
		}
	}
}

type boundedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *boundedConn) Close() error { e := c.Conn.Close(); c.once.Do(c.release); return e }

// Existing streams must end when the upstream session or playback profile is
// revoked too. Per-request checks alone cannot terminate an ongoing film.
func (s *Server) guardSession(ctx context.Context, pin string, session *yamux.Session) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-session.CloseChan():
			return
		case <-ticker.C:
		}
		s.mu.Lock()
		device, approved := s.state.Devices[pin]
		s.mu.Unlock()
		if !approved {
			continue
		} // Unapproved pairing has its own bounded lifetime.
		check, cancel := context.WithTimeout(ctx, 3*time.Second)
		var user jellyUser
		err := s.jf(check, "GET", "/Users/Me", device.Token, nil, &user)
		cancel()
		if err == nil {
			s.observePolicy(pin, user.policySnapshot)
		}
		if err != nil || user.ID != device.UserID || user.Policy.IsDisabled || user.Policy.IsAdministrator {
			_ = session.Close()
			return
		}
	}
}

func (s *Server) observePolicy(pin, policy string) {
	s.mu.Lock()
	if s.policies == nil {
		s.policies = map[string]string{}
	}
	previous, exists := s.policies[pin]
	s.policies[pin] = policy
	var closing []*yamux.Session
	if exists && previous != policy {
		// End old streams on any rights change. A new connection receives the
		// freshly checked policy and remains usable if its profile is active.
		for session := range s.sessions[pin] {
			closing = append(closing, session)
		}
	}
	s.mu.Unlock()
	for _, session := range closing {
		_ = session.Close()
	}
}
