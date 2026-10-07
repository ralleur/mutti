// SPDX-License-Identifier: MPL-2.0
package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/pion/webrtc/v4"
	"golang.org/x/time/rate"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Offer struct {
	ID  string                    `json:"id"`
	SDP webrtc.SessionDescription `json:"sdp"`
}
type exchange struct {
	offer   Offer
	answer  chan webrtc.SessionDescription
	expires time.Time
}
type mailbox struct {
	queue   chan Offer
	expires time.Time
	limiter *rate.Limiter
}
type Broker struct {
	mu       sync.Mutex
	rooms    map[string]*mailbox
	sessions map[string]*exchange
	limiter  *rate.Limiter
}

func NewBroker() *Broker {
	return &Broker{rooms: map[string]*mailbox{}, sessions: map[string]*exchange{}, limiter: rate.NewLimiter(30, 60)}
}
func jsonReply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func decodeBody(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return errors.New("trailing data")
	}
	return nil
}
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Header.Get("Origin") != "" {
		http.Error(w, "forbidden", 403)
		return
	}
	if r.URL.Path == "/health" && r.Method == "GET" {
		jsonReply(w, 200, map[string]bool{"relay": false})
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "v1" || !validID(parts[2]) {
		http.NotFound(w, r)
		return
	}
	id := parts[2]
	now := time.Now()
	b.mu.Lock()
	for k, v := range b.rooms {
		if now.After(v.expires) {
			delete(b.rooms, k)
		}
	}
	for k, v := range b.sessions {
		if now.After(v.expires) {
			delete(b.sessions, k)
		}
	}
	allowed := b.limiter.Allow()
	b.mu.Unlock()
	if !allowed {
		http.Error(w, "busy", 429)
		return
	}
	switch parts[1] {
	case "poll":
		owner := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if r.Method != "POST" || !validID(owner) || digest(owner) != id {
			http.Error(w, "forbidden", 403)
			return
		}
		b.mu.Lock()
		room := b.rooms[id]
		if room == nil {
			if len(b.rooms) >= 1024 {
				b.mu.Unlock()
				http.Error(w, "busy", 503)
				return
			}
			room = &mailbox{queue: make(chan Offer, 8), limiter: rate.NewLimiter(0.5, 4)}
			b.rooms[id] = room
		}
		room.expires = now.Add(time.Minute)
		b.mu.Unlock()
		select {
		case o := <-room.queue:
			jsonReply(w, 200, o)
		case <-time.After(20 * time.Second):
			w.WriteHeader(204)
		case <-r.Context().Done():
		}
	case "offer":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var s webrtc.SessionDescription
		if decodeBody(w, r, &s) != nil || s.Type != webrtc.SDPTypeOffer || len(s.SDP) > 60000 || strings.Contains(s.SDP, " typ relay") {
			http.Error(w, "invalid offer", 400)
			return
		}
		b.mu.Lock()
		room := b.rooms[id]
		if room == nil || len(b.sessions) >= 2048 {
			b.mu.Unlock()
			http.Error(w, say(r, "Mutti ist offline."), 503)
			return
		}
		if !room.limiter.Allow() {
			b.mu.Unlock()
			http.Error(w, "busy", 429)
			return
		}
		sessionID := randomID()
		ex := &exchange{offer: Offer{sessionID, s}, answer: make(chan webrtc.SessionDescription, 1), expires: now.Add(time.Minute)}
		b.sessions[sessionID] = ex
		select {
		case room.queue <- ex.offer:
		default:
			delete(b.sessions, sessionID)
			b.mu.Unlock()
			http.Error(w, "busy", 429)
			return
		}
		b.mu.Unlock()
		defer func() { b.mu.Lock(); delete(b.sessions, sessionID); b.mu.Unlock() }()
		select {
		case answer := <-ex.answer:
			jsonReply(w, 200, answer)
		case <-time.After(35 * time.Second):
			http.Error(w, say(r, "Mutti antwortet nicht."), 504)
		case <-r.Context().Done():
		}
	case "answer":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var s webrtc.SessionDescription
		if decodeBody(w, r, &s) != nil || s.Type != webrtc.SDPTypeAnswer || len(s.SDP) > 60000 || strings.Contains(s.SDP, " typ relay") {
			http.Error(w, "invalid answer", 400)
			return
		}
		// Session IDs are unguessable and only disclosed over the owner's poll.
		b.mu.Lock()
		ex := b.sessions[id]
		b.mu.Unlock()
		if ex == nil {
			http.NotFound(w, r)
			return
		}
		select {
		case ex.answer <- s:
			w.WriteHeader(204)
		default:
			http.Error(w, "already answered", 409)
		}
	default:
		http.NotFound(w, r)
	}
}

var signalHTTP = &http.Client{Timeout: 40 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 8 * time.Second}).DialContext, TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 38 * time.Second, MaxConnsPerHost: 8}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect rejected") }}

func signal(ctx context.Context, base, path, auth string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, e := json.Marshal(in)
		if e != nil {
			return 0, e
		}
		body = bytes.NewReader(b)
	}
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimSuffix(base, "/")+path, body)
	if e != nil {
		return 0, e
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	res, e := signalHTTP.Do(req)
	if e != nil {
		return 0, errors.New("Der Vermittlungsdienst ist nicht erreichbar.")
	}
	defer res.Body.Close()
	if res.StatusCode == 204 {
		return 204, nil
	}
	if res.StatusCode != 200 {
		return res.StatusCode, errors.New("Mutti ist nicht erreichbar oder der Verbindungsdienst ist ausgelastet.")
	}
	if out != nil {
		e = json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(out)
	}
	return res.StatusCode, e
}
