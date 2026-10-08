// SPDX-License-Identifier: MPL-2.0
package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt-in, destructive ONLY to the reserved fresh synthetic smoke instance.
func TestRealJellyfin(t *testing.T) {
	if os.Getenv("MUTTI_FRESH_CONNECT_SMOKE") != "1" {
		t.Skip("requires fresh reserved smoke server")
	}
	t.Setenv("MUTTI_ICE_INTERFACES", "lo0,lo")
	target := os.Getenv("MUTTI_CONNECT_SMOKE_URL")
	if target == "" {
		target = "http://127.0.0.1:18598"
	}
	if target != "http://127.0.0.1:18598" && target != "http://127.0.0.1:18597" {
		t.Fatal("reserved fresh smoke ports only")
	}
	targetURL, _ := url.Parse(target)
	broker := httptest.NewServer(NewBroker())
	defer broker.Close()
	s, e := NewServer(t.TempDir(), broker.URL, "", target, targetURL.Host)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	call := func(method, path, token string, body any, out any) {
		t.Helper()
		var b io.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			b = bytes.NewReader(data)
		}
		if e := s.jf(ctx, method, path, token, b, out); e != nil {
			t.Fatalf("%s %s: %v", method, strings.Split(path, "?")[0], e)
		}
	}
	var info struct {
		StartupWizardCompleted *bool `json:"StartupWizardCompleted"`
	}
	for attempt := 0; attempt < 60; attempt++ {
		_ = s.jf(ctx, "GET", "/System/Info/Public", "", nil, &info)
		if info.StartupWizardCompleted != nil && s.jf(ctx, "GET", "/Startup/User", "", nil, nil) == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if info.StartupWizardCompleted == nil || *info.StartupWizardCompleted {
		t.Fatal("refusing to modify an existing instance")
	}
	call("GET", "/Startup/User", "", nil, nil)
	call("POST", "/Startup/Configuration", "", map[string]string{"ServerName": "Mutti", "UICulture": "en-US", "MetadataCountryCode": "US", "PreferredMetadataLanguage": "en"}, nil)
	password := randomID()
	call("POST", "/Startup/User", "", map[string]string{"Name": "Connect smoke owner", "Password": password}, nil)
	call("POST", "/Startup/RemoteAccess", "", map[string]bool{"EnableRemoteAccess": false}, nil)
	var auth struct {
		AccessToken string `json:"AccessToken"`
	}
	call("POST", "/Users/AuthenticateByName", "", map[string]string{"Username": "Connect smoke owner", "Pw": password}, &auth)
	var user jellyUser
	call("POST", "/Users/New", auth.AccessToken, map[string]string{"Name": "Connect smoke profile", "Password": randomID()}, &user)
	media := os.Getenv("MUTTI_SMOKE_MEDIA")
	sample := os.Getenv("MUTTI_SMOKE_SAMPLE")
	if media == "" || sample == "" {
		t.Fatal("synthetic sample paths required")
	}
	library := "/Library/VirtualFolders?name=Connect+smoke&collectionType=movies&paths=" + url.QueryEscape(media) + "&refreshLibrary=true"
	call("POST", library, auth.AccessToken, map[string]any{"LibraryOptions": map[string]any{"EnableRealtimeMonitor": false, "EnableInternetProviders": false, "SaveLocalMetadata": false, "TypeOptions": []any{map[string]any{"Type": "Movie", "MetadataFetchers": []string{}, "ImageFetchers": []string{}}}}}, nil)
	call("POST", "/Startup/Complete", "", nil, nil)
	var items struct {
		Items []struct {
			ID string `json:"Id"`
		} `json:"Items"`
	}
	for attempt := 0; attempt < 60; attempt++ {
		call("GET", "/Items?Recursive=true&IncludeItemTypes=Movie", auth.AccessToken, nil, &items)
		if len(items.Items) > 0 {
			break
		}
		time.Sleep(time.Second)
	}
	if len(items.Items) != 1 {
		t.Fatal("sample not indexed")
	}
	admin := os.Getenv("MUTTI_CONNECT_SMOKE_ADMIN")
	adminCall := func(path string, body any, out any) {
		t.Helper()
		if admin != "http://127.0.0.1:18594" {
			t.Fatal("reserved smoke admin only")
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, "POST", admin+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
		r, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Body.Close()
		if r.StatusCode < 200 || r.StatusCode >= 300 {
			t.Fatalf("admin %s status %d", path, r.StatusCode)
		}
		if out != nil {
			if e = json.NewDecoder(r.Body).Decode(out); e != nil {
				t.Fatal(e)
			}
		}
	}
	var i Invitation
	if admin == "" {
		go s.Run(ctx)
		i, _ = s.NewInvitation()
	} else {
		var invitation struct {
			URL string `json:"url"`
		}
		adminCall("/invite", map[string]string{}, &invitation)
		i, e = ParseInvitation(invitation.URL)
		if e != nil {
			t.Fatal(e)
		}
	}
	credentials, _ := NewCredentials(i.URL())
	c, e := NewClient(credentials)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	time.Sleep(100 * time.Millisecond)
	paired := make(chan error, 1)
	go func() { paired <- c.Pair(ctx, "Real Jellyfin smoke player", nil) }()
	for attempts := 0; attempts < 300; attempts++ {
		count := 0
		if admin == "" {
			s.mu.Lock()
			count = len(s.pending)
			s.mu.Unlock()
		} else {
			var state struct {
				Pending []pending `json:"pending"`
			}
			adminCall("/state", map[string]string{}, &state)
			count = len(state.Pending)
		}
		if count == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if admin == "" {
		if e = s.approve(ctx, credentials.Identity.Pin(), user.ID, auth.AccessToken); e != nil {
			t.Fatal(e)
		}
	} else {
		adminCall("/approve", map[string]string{"pin": credentials.Identity.Pin(), "userId": user.ID}, nil)
	}
	if e = <-paired; e != nil {
		t.Fatal(e)
	}
	base, _ := c.Gateway()
	r, e := http.Post(base+"/Users/AuthenticateWithQuickConnect", "application/json", strings.NewReader(`{"Secret":"mutti-device-bound"}`))
	if e != nil {
		t.Fatal(e)
	}
	var player struct {
		AccessToken string    `json:"AccessToken"`
		User        jellyUser `json:"User"`
	}
	_ = json.NewDecoder(r.Body).Decode(&player)
	r.Body.Close()
	if r.StatusCode != 200 || player.AccessToken != "mutti-device-bound" || player.User.Policy.IsAdministrator || player.User.ID != user.ID {
		t.Fatalf("wrong paired profile: %d", r.StatusCode)
	}
	t.Log("PASS device-bound pairing to real playback profile")
	r, e = http.Get(base + "/System/Info/Storage")
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 401 && r.StatusCode != 403 {
		t.Fatalf("admin API exposed: %d", r.StatusCode)
	}
	t.Log("PASS administration denied")
	request, _ := http.NewRequest("GET", base+"/Videos/"+items.Items[0].ID+"/stream?static=true", nil)
	request.Header.Set("Range", "bytes=0-4095")
	r, e = http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	data, e := io.ReadAll(r.Body)
	r.Body.Close()
	expected, e2 := os.ReadFile(sample)
	if e != nil || e2 != nil || r.StatusCode != 206 || !bytes.Equal(data, expected[:4096]) {
		t.Fatalf("real media mismatch: %d, %v", r.StatusCode, e)
	}
	t.Log("PASS encrypted real video Range 206 byte exact")
	// Force an HLS profile and follow master -> media playlist -> actual segment.
	profile := map[string]any{"DeviceProfile": map[string]any{"Name": "Mutti HLS smoke", "MaxStreamingBitrate": 2000000, "DirectPlayProfiles": []any{}, "TranscodingProfiles": []any{map[string]any{"Type": "Video", "Context": "Streaming", "Protocol": "hls", "Container": "ts", "VideoCodec": "h264", "AudioCodec": "aac"}}}}
	profileBody, _ := json.Marshal(profile)
	playbackURL := base + "/Items/" + items.Items[0].ID + "/PlaybackInfo?UserId=" + user.ID + "&IsPlayback=true&EnableDirectPlay=false&EnableDirectStream=false&EnableTranscoding=true&AutoOpenLiveStream=true"
	r, e = http.Post(playbackURL, "application/json", bytes.NewReader(profileBody))
	if e != nil {
		t.Fatal(e)
	}
	var playback struct {
		MediaSources []struct {
			TranscodingURL string `json:"TranscodingUrl"`
		} `json:"MediaSources"`
	}
	e = json.NewDecoder(r.Body).Decode(&playback)
	r.Body.Close()
	if e != nil || r.StatusCode != 200 || len(playback.MediaSources) == 0 || playback.MediaSources[0].TranscodingURL == "" {
		t.Fatalf("HLS profile failed status %d", r.StatusCode)
	}
	path := playback.MediaSources[0].TranscodingURL
	if strings.Contains(path, "127.0.0.1") || strings.Contains(path, "mutti.internal") {
		t.Fatal("SDK metadata URL must stay relative")
	}
	nextURL, _ := url.Parse(base + "/" + strings.TrimPrefix(path, "/"))
	gotSegment := false
	for step := 0; step < 4; step++ {
		r, e = http.Get(nextURL.String())
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		r.Body.Close()
		if e != nil || r.StatusCode != 200 {
			t.Fatalf("HLS step %d: status %d", step, r.StatusCode)
		}
		if !bytes.HasPrefix(body, []byte("#EXTM3U")) {
			if len(body) < 188 {
				t.Fatal("empty HLS segment")
			}
			gotSegment = true
			break
		}
		found := false
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			u, e := url.Parse(line)
			if e != nil {
				t.Fatal(e)
			}
			nextURL = nextURL.ResolveReference(u)
			if !strings.HasPrefix(nextURL.String(), base+"/") {
				t.Fatal("HLS escaped local capability")
			}
			found = true
			break
		}
		if !found {
			t.Fatal("HLS playlist has no media")
		}
	}
	if !gotSegment {
		t.Fatal("no HLS media segment")
	}
	t.Log("PASS real HLS transcode, rewritten playlists and media segment")
	// Existing WebSocket upgrades must traverse the same device-bound channel.
	ws := base + "/socket?api_key=mutti-device-bound&deviceId=connect-smoke"
	req, _ := http.NewRequest("GET", ws, nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	r, e = http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 101 {
		t.Fatalf("websocket failed: %d", r.StatusCode)
	}
	t.Log("PASS Jellyfin WebSocket upgrade")
	if admin == "" {
		if e = s.Revoke(credentials.Identity.Pin()); e != nil {
			t.Fatal(e)
		}
	} else {
		adminCall("/revoke", map[string]string{"pin": credentials.Identity.Pin()}, nil)
	}
	c.Close()
	again, e := NewClient(c.Credentials())
	if e != nil {
		t.Fatal(e)
	}
	defer again.Close()
	base, _ = again.Gateway()
	r, e = http.Get(base + "/Users/Me")
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatal("revoked peer allowed")
	}
	t.Log("PASS revoked device denied after reconnect")
}
