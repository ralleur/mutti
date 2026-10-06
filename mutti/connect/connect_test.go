// SPDX-License-Identifier: MPL-2.0
package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/pion/webrtc/v4"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDirectPairProxyAndRevoke(t *testing.T) {
	for _, mode := range []string{"device", "profile", "libraries", "reconnect"} {
		t.Run(mode, func(t *testing.T) { testDirectPairProxyAndRevoke(t, mode) })
	}
}
func testDirectPairProxyAndRevoke(t *testing.T, mode string) {
	var disabled, loggedOut atomic.Bool
	t.Setenv("MUTTI_ICE_INTERFACES", "lo0,lo")
	payload := bytes.Repeat([]byte("mutti-synthetic-media\n"), 32000)
	jf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/System/Info/Public":
			jsonReply(w, 200, map[string]string{"Id": "test-server", "ServerName": "Mutti", "Version": "12.1"})
		case "/Users/profile":
			jsonReply(w, 200, map[string]any{"Id": "profile", "Name": "Family", "Policy": map[string]bool{"IsAdministrator": false}})
		case "/QuickConnect/Initiate":
			jsonReply(w, 200, map[string]string{"Code": "123456", "Secret": "test-quick-secret"})
		case "/QuickConnect/Authorize":
			jsonReply(w, 200, true)
		case "/Sessions/Logout":
			if strings.Contains(r.Header.Get("Authorization"), `Token="private-upstream-token"`) {
				loggedOut.Store(true)
			}
			w.WriteHeader(204)
		case "/Users/AuthenticateWithQuickConnect":
			jsonReply(w, 200, map[string]any{"AccessToken": "private-upstream-token", "User": map[string]string{"Id": "profile", "Name": "Family"}})
		case "/Users/Me":
			if !strings.Contains(r.Header.Get("Authorization"), `Token="private-upstream-token"`) {
				t.Error("wrong upstream token")
			}
			jsonReply(w, 200, map[string]any{"Id": "profile", "Name": "Family", "Policy": map[string]bool{"IsDisabled": mode == "profile" && disabled.Load(), "EnableAllFolders": !disabled.Load()}})
		case "/video":
			if !strings.Contains(r.Header.Get("Authorization"), `Token="private-upstream-token"`) {
				t.Error("missing bound token")
			}
			http.ServeContent(w, r, "clip.mp4", time.Time{}, bytes.NewReader(payload))
		case "/hls/master.m3u8":
			if r.Header.Get("Accept-Encoding") != "identity" {
				t.Error("metadata rewriting requires uncompressed upstream responses")
			}
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = io.WriteString(w, "#EXTM3U\n/hls/part.ts?api_key=private-upstream-token\n")
		case "/slow":
			w.WriteHeader(200)
			f := w.(http.Flusher)
			for i := 0; i < 100; i++ {
				_, e := w.Write(payload[:1024])
				if e != nil {
					return
				}
				f.Flush()
				select {
				case <-r.Context().Done():
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer jf.Close()
	target, _ := url.Parse(jf.URL)
	broker := httptest.NewServer(NewBroker())
	defer broker.Close()
	server, e := NewServer(t.TempDir(), broker.URL, "", jf.URL, target.Host)
	if e != nil {
		t.Fatal(e)
	}
	var hubSeen atomic.Value
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		hubSeen.Store(r.URL.Path + "|" + r.Header.Get("Authorization") + "|" + r.Header.Get("X-Mutti-Device") + "|" + r.Header.Get("X-Mutti-Hub-Peer") + "|" + strconv.Itoa(len(body)))
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: done\ndata: {}\n\n")
	}))
	defer hub.Close()
	peer := strings.Repeat("p", 64)
	if e = server.SetHub("http://example.com", peer); e == nil {
		t.Fatal("non-loopback module service accepted")
	}
	if e = server.SetHub(hub.URL, peer); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Run(ctx)
	invitation, e := server.NewInvitation()
	if e != nil {
		t.Fatal(e)
	}
	credentials, e := NewCredentials(invitation.URL())
	if e != nil {
		t.Fatal(e)
	}
	client, e := NewClient(credentials)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	base, e := client.Gateway()
	if e != nil {
		t.Fatal(e)
	}
	time.Sleep(100 * time.Millisecond)
	res, e := http.Get(base + "/video")
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("unapproved status %d", res.StatusCode)
	}
	paired := make(chan error, 1)
	go func() { paired <- client.Pair(ctx, "Test player", nil) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		server.mu.Lock()
		n := len(server.pending)
		server.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no pending pair")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The one-use invitation cannot be taken by another key after being claimed.
	secondID, _ := NewIdentity()
	req := httptest.NewRequest("POST", "http://mutti.internal/_mutti/pair", strings.NewReader(`{"secret":"`+invitation.Secret+`","name":"attacker"}`))
	req.Host = "mutti.internal"
	rec := httptest.NewRecorder()
	server.pair(secondID.Pin(), rec, req)
	if rec.Code != 410 {
		t.Fatalf("replay accepted %d", rec.Code)
	}
	if e = server.approve(ctx, credentials.Identity.Pin(), "profile", "owner"); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-paired:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pair never approved")
	}
	pair, err := client.peer.PC.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil || pair == nil || pair.Local.Typ == webrtc.ICECandidateTypeRelay || pair.Remote.Typ == webrtc.ICECandidateTypeRelay {
		t.Fatal("expected a direct ICE pair")
	}
	req2, _ := http.NewRequest("GET", base+"/video", nil)
	req2.Header.Set("Range", "bytes=128-65535")
	res, e = http.DefaultClient.Do(req2)
	if e != nil {
		t.Fatal(e)
	}
	got, e := io.ReadAll(res.Body)
	res.Body.Close()
	if e != nil || res.StatusCode != 206 || !bytes.Equal(got, payload[128:65536]) {
		t.Fatalf("range mismatch status=%d size=%d err=%v", res.StatusCode, len(got), e)
	}
	res, e = http.Post(base+"/Users/AuthenticateWithQuickConnect", "application/json", strings.NewReader(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	got, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if bytes.Contains(got, []byte("private-upstream-token")) || !bytes.Contains(got, []byte("mutti-device-bound")) {
		t.Fatal("token escaped or profile missing")
	}
	// Module requests carry the device-bound profile session, never client identity.
	hubReq, _ := http.NewRequest("POST", base+"/mutti/hub/v1/photos/assets", bytes.NewReader(payload[:300000]))
	hubReq.Header.Set("Authorization", `MediaBrowser Token="forged"`)
	hubReq.Header.Set("X-Mutti-Device", "forged")
	hubReq.Header.Set("X-Mutti-Hub-Peer", "forged")
	res, e = http.DefaultClient.Do(hubReq)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	seen, _ := hubSeen.Load().(string)
	want := "/mutti/hub/v1/photos/assets|" + `MediaBrowser Client="kurtz via Mutti", Device="Paired device", DeviceId="` + credentials.Identity.Pin() + `", Version="0.1", Token="private-upstream-token"|` + credentials.Identity.Pin() + "|" + peer + "|300000"
	if res.StatusCode != 200 || seen != want {
		t.Fatalf("hub forwarding %d %q", res.StatusCode, seen)
	}
	hubSeen.Store("")
	res, e = http.Get(base + "/mutti/hub/v1/admin/state")
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 || hubSeen.Load() != "" {
		t.Fatalf("owner module route reachable from device: %d", res.StatusCode)
	}
	res, e = http.Get(base + "/hls/master.m3u8")
	if e != nil {
		t.Fatal(e)
	}
	got, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if !bytes.Contains(got, []byte(base+"/hls/part.ts?api_key=mutti-device-bound")) {
		t.Fatalf("HLS not rewritten: %s", got)
	}
	// Browser origins, DNS-rebinding hosts and missing capability are rejected.
	for _, kind := range []string{"origin", "host", "path"} {
		req, _ := http.NewRequest("GET", base+"/video", nil)
		switch kind {
		case "origin":
			req.Header.Set("Origin", "https://evil.example")
		case "host":
			req.Host = "evil.example"
		case "path":
			req.URL.Path = "/video"
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatalf("boundary %s status %d", kind, res.StatusCode)
		}
	}
	if mode == "reconnect" {
		// Drop the transport without the HTTP/yamux graceful client teardown,
		// then reconnect the same saved device while old sessions may remain.
		client.peer.Close()
		returning, err := NewClient(client.Credentials())
		if err != nil {
			t.Fatal(err)
		}
		defer returning.Close()
		base, e = returning.Gateway()
		if e != nil {
			t.Fatal(e)
		}
	}
	res, e = http.Get(base + "/slow")
	if e != nil {
		t.Fatal(e)
	}
	one := make([]byte, 1024)
	if _, e = io.ReadFull(res.Body, one); e != nil {
		t.Fatal(e)
	}
	if mode == "profile" || mode == "libraries" {
		disabled.Store(true)
	} else {
		revoked := make(chan error, 1)
		go func() { revoked <- server.Revoke(credentials.Identity.Pin()) }()
		select {
		case e = <-revoked:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("revocation waits for a stale transport")
		}
	}
	ended := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, res.Body); res.Body.Close(); close(ended) }()
	select {
	case <-ended:
	case <-time.After(6 * time.Second):
		t.Fatal("revoked stream remains open")
	}
	// Reconnect after server-side revocation also fails authorization.
	client.Close()
	fresh, e := NewClient(client.Credentials())
	if e != nil {
		t.Fatal(e)
	}
	defer fresh.Close()
	base, e = fresh.Gateway()
	if e != nil {
		t.Fatal(e)
	}
	res, e = http.Get(base + "/Users/Me")
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	expected := 403
	if mode == "libraries" {
		expected = 200
	}
	if res.StatusCode != expected {
		t.Fatalf("reconnect after policy/revoke %d", res.StatusCode)
	}
	if revokedDevice := mode != "profile" && mode != "libraries"; loggedOut.Load() != revokedDevice {
		t.Fatalf("upstream session logout %v for mode %s", loggedOut.Load(), mode)
	}
}
func TestInvitationAndServerIdentity(t *testing.T) {
	id, _ := NewIdentity()
	i := Invitation{1, "http://127.0.0.1:9999", randomID(), id.Pin(), randomID(), "", time.Now().Add(time.Minute).Unix()}
	if _, e := ParseInvitation(i.URL()); e != nil {
		t.Fatal(e)
	}
	cases := []Invitation{i, i, i, i}
	cases[0].Expires = time.Now().Add(-time.Second).Unix()
	cases[1].Broker = "http://public.example"
	cases[2].STUN = "turn:relay.example"
	cases[3].Pin = "wrong"
	for _, bad := range cases {
		if _, e := ParseInvitation(bad.URL()); e == nil {
			t.Fatal("invalid invitation accepted")
		}
	}
}
func TestWrongPinFailsBeforePairing(t *testing.T) {
	t.Setenv("MUTTI_ICE_INTERFACES", "lo0,lo")
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted peer reached Jellyfin") }))
	defer target.Close()
	u, _ := url.Parse(target.URL)
	broker := httptest.NewServer(NewBroker())
	defer broker.Close()
	s, e := NewServer(t.TempDir(), broker.URL, "", target.URL, u.Host)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go s.Run(ctx)
	i, _ := s.NewInvitation()
	i.Pin = randomID()
	c, e := NewCredentials(i.URL())
	if e != nil {
		t.Fatal(e)
	}
	client, e := NewClient(c)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	time.Sleep(50 * time.Millisecond)
	if e = client.Pair(ctx, "wrong", nil); e == nil {
		t.Fatal("wrong server accepted")
	}
}
func TestBrokerBoundaries(t *testing.T) {
	b := NewBroker()
	for _, body := range []string{`{"type":"offer","sdp":"a=candidate:1 typ relay"}`, strings.Repeat("a", 70000)} {
		r := httptest.NewRequest("POST", "/v1/offer/"+randomID(), strings.NewReader(body))
		w := httptest.NewRecorder()
		b.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/v1/poll/"+randomID(), nil)
	r.Header.Set("Authorization", "Bearer "+randomID())
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("mailbox owner spoofing")
	}
}
func TestStoredDevicePersistsWithoutInvite(t *testing.T) {
	path := t.TempDir()
	s, e := NewServer(path, "http://127.0.0.1:9999", "", "http://127.0.0.1:1", "127.0.0.1:1")
	if e != nil {
		t.Fatal(e)
	}
	s.state.Devices["test"] = Device{Pin: "test"}
	if e = savePrivate(s.path, s.state); e != nil {
		t.Fatal(e)
	}
	again, e := NewServer(path, "http://127.0.0.1:9999", "", "http://127.0.0.1:1", "127.0.0.1:1")
	if e != nil {
		t.Fatal(e)
	}
	if again.state.Identity.Pin() != s.state.Identity.Pin() || len(again.state.Devices) != 1 || len(again.invites) != 0 {
		t.Fatal("state lost")
	}
	_, _ = json.Marshal(again.state)
}

func TestExpiredInviteAndProfileEscalation(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, 200, map[string]any{"Id": "admin", "Policy": map[string]bool{"IsAdministrator": true}})
	}))
	defer target.Close()
	u, _ := url.Parse(target.URL)
	s, e := NewServer(t.TempDir(), "http://127.0.0.1:9999", "", target.URL, u.Host)
	if e != nil {
		t.Fatal(e)
	}
	i, _ := s.NewInvitation()
	s.invites[digest(i.Secret)].Expires = time.Now().Add(-time.Second)
	id, _ := NewIdentity()
	r := httptest.NewRequest("POST", "http://mutti.internal/_mutti/pair", strings.NewReader(`{"secret":"`+i.Secret+`","name":"expired"}`))
	w := httptest.NewRecorder()
	s.pair(id.Pin(), w, r)
	if w.Code != 410 {
		t.Fatal("expired invitation accepted")
	}
	s.state.Devices[id.Pin()] = Device{Pin: id.Pin(), Token: "bound-token"}
	r = httptest.NewRequest("GET", "http://mutti.internal/Items", nil)
	r.URL.Scheme = ""
	r.URL.Host = ""
	w = httptest.NewRecorder()
	s.remoteRequest(id.Pin(), w, r)
	if w.Code != 403 {
		t.Fatal("promoted administrator profile retained remote playback grant")
	}
}

func TestSDKMetadataPathsRemainRelative(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://mutti.internal/Items/test/PlaybackInfo", nil)
	response := &http.Response{Header: http.Header{"Content-Type": []string{"application/json"}}, Request: req, Body: io.NopCloser(strings.NewReader(`{"MediaSources":[{"TranscodingUrl":"/Videos/test/master.m3u8?ApiKey=private-token","DirectStreamUrl":"http://mutti.internal/Videos/test/stream?ApiKey=private-token"}]}`))}
	if e := rewriteResponse(response, "http://127.0.0.1:9999/capability", "private-token"); e != nil {
		t.Fatal(e)
	}
	data, _ := io.ReadAll(response.Body)
	if bytes.Contains(data, []byte("capability")) || bytes.Contains(data, []byte("mutti.internal")) || bytes.Contains(data, []byte("private-token")) || !bytes.Contains(data, []byte(`"TranscodingUrl":"/Videos/test/master.m3u8?ApiKey=mutti-device-bound"`)) {
		t.Fatalf("SDK URL contract violated: %s", data)
	}
}
