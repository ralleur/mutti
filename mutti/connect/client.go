// SPDX-License-Identifier: MPL-2.0
package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/pion/webrtc/v4"
)

type Credentials struct {
	Invitation Invitation `json:"invitation"`
	Identity   Identity   `json:"identity"`
}

func NewCredentials(raw string) (Credentials, error) {
	i, e := ParseInvitation(raw)
	if e != nil {
		return Credentials{}, e
	}
	identity, e := NewIdentity()
	return Credentials{i, identity}, e
}

type Client struct {
	mu          sync.Mutex
	credentials Credentials
	session     *yamux.Session
	peer        *Peer
	closed      bool
	http        *http.Client
	transport   *http.Transport
	gateway     *http.Server
	listener    net.Listener
	URL         string
	cancel      context.CancelFunc
	root        context.Context
}

func NewClient(c Credentials) (*Client, error) {
	if e := c.Invitation.Validate(); e != nil {
		return nil, e
	}
	if !validID(c.Identity.Pin()) {
		return nil, errors.New("Ungültiger Geräteschlüssel.")
	}
	root, cancel := context.WithCancel(context.Background())
	cl := &Client{credentials: c, root: root, cancel: cancel}
	cl.transport = &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "mutti.internal:80" {
			return nil, errors.New("fixed target only")
		}
		session, e := cl.connect(ctx)
		if e != nil {
			return nil, e
		}
		return session.Open()
	}, MaxConnsPerHost: 16, ResponseHeaderTimeout: 40 * time.Second, IdleConnTimeout: 40 * time.Second, DisableCompression: true}
	cl.http = &http.Client{Transport: cl.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect rejected") }}
	return cl, nil
}
func (c *Client) connect(parent context.Context) (*yamux.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, net.ErrClosed
	}
	if c.session != nil && !c.session.IsClosed() {
		return c.session, nil
	}
	if c.peer != nil {
		c.peer.Close()
	}
	ctx, cancel := context.WithTimeout(parent, 40*time.Second)
	defer cancel()
	stop := context.AfterFunc(c.root, cancel)
	defer stop()
	p, e := newPeer(c.credentials.Invitation.STUN)
	if e != nil {
		return nil, e
	}
	success := false
	defer func() {
		if !success {
			p.Close()
		}
	}()
	offer, e := p.local(ctx, true)
	if e != nil {
		return nil, e
	}
	var answer webrtc.SessionDescription
	i := c.credentials.Invitation
	if _, e = signal(ctx, i.Broker, "/v1/offer/"+i.Room, "", offer, &answer); e != nil {
		return nil, e
	}
	if e = p.remote(answer); e != nil {
		return nil, e
	}
	session, _, e := p.secure(ctx, c.credentials.Identity, i.Pin, false)
	if e != nil {
		return nil, errors.New("Keine sichere direkte Verbindung zu Mutti. Prüfe, ob beide Netze UDP-Verbindungen zulassen.")
	}
	c.session = session
	c.peer = p
	success = true
	return session, nil
}
func (c *Client) Pair(ctx context.Context, name string, progress func(string)) error {
	deadline := time.Unix(c.credentials.Invitation.Expires, 0)
	if deadline.Before(time.Now()) {
		return errors.New("QR-Code abgelaufen.")
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"secret": c.credentials.Invitation.Secret, "name": name})
	for {
		req, e := http.NewRequestWithContext(ctx, "POST", "http://mutti.internal/_mutti/pair", bytes.NewReader(body))
		if e != nil {
			return e
		}
		req.Header.Set("Content-Type", "application/json")
		response, e := c.http.Do(req)
		if e != nil {
			return errors.New("Mutti ist nicht direkt erreichbar. Prüfe Verbindung und Vermittlungsdienst.")
		}
		var reply struct {
			State string `json:"state"`
			Pin   string `json:"pin"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&reply)
		response.Body.Close()
		if response.StatusCode == 200 && reply.State == "approved" && reply.Pin == c.credentials.Identity.Pin() {
			return nil
		}
		if response.StatusCode != 202 {
			return errors.New("Kopplung abgelaufen oder abgelehnt. Bitte neuen QR-Code erstellen.")
		}
		if progress != nil {
			progress("Bitte das Gerät in Mutti freigeben. Fingerabdruck: " + c.credentials.Identity.Pin())
		}
		select {
		case <-ctx.Done():
			return errors.New("Kopplung abgelaufen oder abgebrochen.")
		case <-time.After(2 * time.Second):
		}
	}
}
func (c *Client) Credentials() Credentials {
	copy := c.credentials
	copy.Invitation.Secret = ""
	copy.Invitation.Expires = 0
	return copy
}
func (c *Client) Gateway() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return "", net.ErrClosed
	}
	if c.listener != nil {
		return c.URL, nil
	}
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return "", e
	}
	capability := randomID()
	prefix := "/" + capability
	c.URL = "http://" + listener.Addr().String() + prefix
	c.listener = listener
	proxy := &httputil.ReverseProxy{ErrorLog: log.New(io.Discard, "", 0), Rewrite: func(pr *httputil.ProxyRequest) {
		pr.Out.URL.Scheme = "http"
		pr.Out.URL.Host = "mutti.internal"
		pr.Out.Host = "mutti.internal"
		pr.Out.URL.Path = strings.TrimPrefix(pr.In.URL.Path, prefix)
		if pr.Out.URL.Path == "" {
			pr.Out.URL.Path = "/"
		}
		pr.Out.URL.RawPath = ""
		pr.Out.Header.Del("Forwarded")
		pr.Out.Header.Del("X-Forwarded-For")
		pr.Out.Header.Del("X-Forwarded-Host")
		pr.Out.Header.Del("X-Forwarded-Proto")
		pr.Out.Header.Del("Origin")
		pr.Out.Header.Del("Cookie")
		pr.Out.Header.Set("Accept-Encoding", "identity")
	}, Transport: c.transport, FlushInterval: -1, ModifyResponse: func(r *http.Response) error {
		if r.StatusCode >= 300 && r.StatusCode < 400 {
			return errors.New("redirect blocked")
		}
		return rewriteResponse(r, c.URL, "")
	}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, "Mutti ist nicht direkt erreichbar. Erneut versuchen oder Netzwerk prüfen.", 502)
	}}
	c.gateway = &http.Server{ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32768, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != listener.Addr().String() || r.URL.IsAbs() || r.Method == "CONNECT" || len(r.Header.Values("Origin")) > 0 || !strings.HasPrefix(r.URL.Path, prefix+"/") || strings.Contains(r.URL.Path, "..") || strings.Contains(r.URL.Path, "\\") {
			http.Error(w, "forbidden", 403)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		proxy.ServeHTTP(w, r)
	})}
	go func() { _ = c.gateway.Serve(listener) }()
	return c.URL, nil
}
func (c *Client) Close() {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.gateway != nil {
		_ = c.gateway.Close()
	}
	if c.session != nil {
		_ = c.session.Close()
	}
	if c.peer != nil {
		c.peer.Close()
	}
	c.transport.CloseIdleConnections()
}

// Rewrites only URL fields and HLS references, never arbitrary library paths.
// JSON/HLS cannot leak an upstream bearer token or escape the local capability.
func rewriteResponse(r *http.Response, base, token string) error {
	content := strings.ToLower(r.Header.Get("Content-Type"))
	isJSON := strings.Contains(content, "json")
	isHLS := strings.Contains(content, "mpegurl") || strings.HasSuffix(r.Request.URL.Path, ".m3u8")
	if !isJSON && !isHLS {
		return nil
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 8<<20+1))
	r.Body.Close()
	if e != nil {
		return e
	}
	if len(b) > 8<<20 {
		return errors.New("metadata too large")
	}
	if token != "" {
		b = bytes.ReplaceAll(b, []byte(token), []byte("mutti-device-bound"))
	}
	rewriteURL := func(raw string) (string, error) {
		u, e := url.Parse(raw)
		if e != nil {
			return "", e
		}
		if u.IsAbs() {
			if u.Host != "mutti.internal" {
				return "", errors.New("external media URL blocked")
			}
			return base + u.RequestURI(), nil
		}
		if strings.HasPrefix(raw, "/") {
			return base + raw, nil
		}
		return raw, nil
	}
	if isJSON {
		var value any
		if e = json.Unmarshal(b, &value); e != nil {
			return e
		}
		var walk func(any)
		walk = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				for k, v := range x {
					if s, ok := v.(string); ok && strings.HasSuffix(strings.ToLower(k), "url") {
						// Jellyfin SDK appends JSON URL paths to its base path. Keep these
						// relative; adding the gateway prefix here would duplicate it.
						if u, err := url.Parse(s); err == nil && u.IsAbs() {
							origin := u.Scheme + "://" + u.Host
							if origin == "http://mutti.internal" || origin == r.Request.URL.Scheme+"://"+r.Request.URL.Host || origin == r.Request.URL.Scheme+"://"+r.Request.Host {
								x[k] = u.RequestURI()
							}
						}
					} else {
						walk(v)
					}
				}
			case []any:
				for _, v := range x {
					walk(v)
				}
			}
		}
		walk(value)
		b, e = json.Marshal(value)
		if e != nil {
			return e
		}
	} else {
		lines := strings.Split(string(b), "\n")
		for idx, line := range lines {
			line = strings.TrimSuffix(line, "\r")
			if line != "" && !strings.HasPrefix(line, "#") {
				lines[idx], e = rewriteURL(line)
				if e != nil {
					return e
				}
			} else if start := strings.Index(line, "URI=\""); start >= 0 {
				start += 5
				end := strings.Index(line[start:], "\"")
				if end < 0 {
					return errors.New("invalid HLS URI")
				}
				updated, err := rewriteURL(line[start : start+end])
				if err != nil {
					return err
				}
				lines[idx] = line[:start] + updated + line[start+end:]
			}
		}
		b = []byte(strings.Join(lines, "\n"))
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	r.ContentLength = int64(len(b))
	r.Header.Del("Content-Length")
	r.Header.Del("ETag")
	return nil
}
