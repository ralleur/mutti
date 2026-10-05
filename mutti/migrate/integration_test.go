// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealMigration(t *testing.T) {
	if os.Getenv("MUTTI_FRESH_IMPORT_SMOKE") != "1" {
		t.Skip("opt-in synthetic server test")
	}
	work, e := os.MkdirTemp("", "mutti-import-smoke-")
	if e != nil {
		t.Fatal(e)
	}
	t.Log("Synthetic artifacts:", work)
	repo := os.Getenv("MUTTI_IMPORT_REPO")
	if repo == "" {
		t.Fatal("repo required")
	}
	ff := filepath.Join(repo, "build/macos/osx-arm64/Mutti.app/Contents/Resources/ffmpeg/ffmpeg")
	sourcePort, _ := freePort()
	targetPort, _ := freePort()
	o := Options{Server: filepath.Join(repo, "build/server/osx-arm64/jellyfin"), Web: filepath.Join(repo, "../mutti-web/dist"), FFmpeg: ff, Bind: "127.0.0.1", Root: filepath.Join(work, "target"), Backend: fmt.Sprintf("http://127.0.0.1:%d", targetPort), TargetOrigin: fmt.Sprintf("http://127.0.0.1:%d", targetPort)}
	if exe := os.Getenv("MUTTI_IMPORT_SERVER"); exe != "" {
		o.Server = exe
	}
	if web := os.Getenv("MUTTI_IMPORT_WEB"); web != "" {
		o.Web = web
	}
	if ffmpeg := os.Getenv("MUTTI_IMPORT_FFMPEG"); ffmpeg != "" {
		o.FFmpeg = ffmpeg
		ff = ffmpeg
	}
	withIntro := os.Getenv("MUTTI_TEST_INTRO_SKIPPER") == "1"
	sourceWithIntro := withIntro && os.Getenv("MUTTI_TEST_SOURCE_WITHOUT_INTRO") != "1"
	if withIntro {
		o.IntroSkipper = filepath.Join(repo, "build/intro-skipper")
		if path := os.Getenv("MUTTI_INTRO_BUNDLE"); path != "" {
			o.IntroSkipper = path
		}
	}
	media := filepath.Join(work, "media")
	_ = os.Mkdir(media, 0700)
	cmd := exec.Command(ff, "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=10", "-t", "12", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-an", filepath.Join(media, "Synthetic Film (2026).mp4"))
	if e := cmd.Run(); e != nil {
		t.Fatal(e)
	}
	if os.Getenv("MUTTI_TEST_EXPORT") == "1" {
		pluginDir := filepath.Join(work, "source", "data", "plugins", "Mutti Export_0.1.1.0")
		_ = os.MkdirAll(pluginDir, 0700)
		data, e := os.ReadFile(filepath.Join(repo, "build/export/Mutti.Export.dll"))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(pluginDir, "Mutti.Export.dll"), data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	sourceOptions := o
	if !sourceWithIntro {
		sourceOptions.IntroSkipper = ""
	}
	source, e := sourceOptions.startServer(filepath.Join(work, "source"), sourcePort, fmt.Sprintf("127.0.0.1:%d", sourcePort), "", false)
	if e != nil {
		t.Fatal(e)
	}
	defer source.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	a, _ := NewAPI(fmt.Sprintf("http://127.0.0.1:%d", sourcePort))
	info, e := waitServer(ctx, source, a)
	if e != nil || info.StartupWizardCompleted {
		t.Fatal("fresh server required", e)
	}
	call := func(method, path string, body, out any) {
		t.Helper()
		if e := a.call(ctx, method, path, body, out); e != nil {
			t.Fatalf("%s %s: %v", method, path, e)
		}
	}
	// The setup endpoint becomes ready after the lightweight public info endpoint.
	for {
		if e := a.call(ctx, "GET", "/Startup/User", nil, nil); e == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}
	password := randomID()
	viewerPassword := randomID()
	call("POST", "/Startup/Configuration", map[string]string{"ServerName": "Import Fixture", "UICulture": "de-DE", "MetadataCountryCode": "DE", "PreferredMetadataLanguage": "de"}, nil)
	call("POST", "/Startup/User", map[string]string{"Name": "Import Owner", "Password": password}, nil)
	call("POST", "/Startup/RemoteAccess", map[string]bool{"EnableRemoteAccess": false}, nil)
	if _, e = login(ctx, a, "Import Owner", password); e != nil {
		t.Fatal(e)
	}
	if sourceWithIntro {
		if e = verifyIntroLoaded(ctx, a); e != nil {
			t.Fatal(e)
		}
		var config map[string]any
		call("GET", "/Plugins/"+introID+"/Configuration", nil, &config)
		config["AutoDetectIntros"] = false
		config["MinimumIntroDuration"] = 27
		config["SkipFirstEpisode"] = true
		call("POST", "/Plugins/"+introID+"/Configuration", config, nil)
	}
	var viewer User
	call("POST", "/Users/New", map[string]string{"Name": "Import Viewer", "Password": viewerPassword}, &viewer)
	call("POST", "/Library/VirtualFolders?name=Meine+Filme&collectionType=movies&paths="+url.QueryEscape(media)+"&refreshLibrary=true", map[string]any{"LibraryOptions": map[string]any{"EnableRealtimeMonitor": false, "EnableInternetProviders": false, "SaveLocalMetadata": false, "TypeOptions": []any{map[string]any{"Type": "Movie", "MetadataFetchers": []string{}, "ImageFetchers": []string{}}}}}, nil)
	call("POST", "/Startup/Complete", nil, nil)
	var items struct{ Items []struct{ Id string } }
	for i := 0; i < 90; i++ {
		call("GET", "/Items?Recursive=true&IncludeItemTypes=Movie", nil, &items)
		if len(items.Items) == 1 {
			break
		}
		time.Sleep(time.Second)
	}
	if len(items.Items) != 1 {
		t.Fatal("media not scanned")
	}
	item := items.Items[0].Id
	call("POST", "/UserItems/"+item+"/UserData?userId="+viewer.Id, map[string]any{"IsFavorite": true, "Played": true, "PlayCount": 3, "PlaybackPositionTicks": int64(40000000), "LastPlayedDate": "2026-10-01T12:00:00Z"}, nil)
	call("POST", "/Playlists", map[string]any{"Name": "Meine Merkliste", "Ids": []string{item}, "UserId": viewer.Id, "MediaType": "Video", "IsPublic": true}, nil)
	// A generated profile image exercises private persisted ownership fields.
	portrait := filepath.Join(work, "portrait.png")
	if e = exec.Command(ff, "-f", "lavfi", "-i", "color=c=yellow:s=32x32", "-frames:v", "1", portrait).Run(); e != nil {
		t.Fatal(e)
	}
	imageData, e := os.ReadFile(portrait)
	if e != nil {
		t.Fatal(e)
	}
	upload, e := http.NewRequestWithContext(ctx, "POST", a.Base.String()+"/UserImage?userId="+viewer.Id, strings.NewReader(base64.StdEncoding.EncodeToString(imageData)))
	if e != nil {
		t.Fatal(e)
	}
	upload.Header.Set("Content-Type", "image/png")
	upload.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Client="Mutti Import", Device="Mutti", DeviceId="%s", Version="0.1.0", Token="%s"`, a.Device, a.Token))
	response, e := a.Client.Do(upload)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatal("profile image upload", response.StatusCode)
	}
	// Include preferences with private identity setters and owned home sections.
	call("POST", "/DisplayPreferences/mutti-fixture?userId="+viewer.Id+"&client=emby", map[string]any{"Id": "mutti-fixture", "ShowBackdrop": true, "ShowSidebar": true, "ScrollDirection": "Vertical", "SortBy": "Name", "SortOrder": "Descending", "CustomPrefs": map[string]string{"homesection0": "Resume", "homesection1": "LatestMedia", "skipForwardLength": "45000", "dashboardTheme": "dark", "fixture": "preserved"}}, nil)
	// Finish pending library writes before asking Jellyfin for its consistent backup.
	for i := 0; i < 60; i++ {
		var tasks []struct{ Key, State string }
		call("GET", "/ScheduledTasks", nil, &tasks)
		busy := false
		for _, task := range tasks {
			if task.Key == "RefreshLibrary" && (task.State == "Running" || task.State == "Cancelling") {
				busy = true
			}
		}
		if !busy {
			break
		}
		time.Sleep(time.Second)
	}
	if sourceWithIntro {
		call("POST", "/Episode/"+item+"/Segments", map[string]any{"Type": "Introduction", "Start": 2.0, "End": 5.0}, nil)
	}
	if os.Getenv("MUTTI_TEST_EXPORT") == "1" {
		s, e := OpenSource(ctx, SourceInput{Address: a.Base.String(), Username: "Import Owner", Password: password}, "")
		if e != nil {
			t.Fatal(e)
		}
		dir := filepath.Join(work, "download")
		_ = os.Mkdir(dir, 0700)
		archive, e := remoteBackup(ctx, s, dir)
		s.API.logout()
		if e != nil {
			t.Fatal("remote helper", e)
		}
		if _, e = ReadAudit(archive); e != nil {
			t.Fatal("remote archive", e)
		}
		if sourceWithIntro {
			local, err := o.snapshotIntro(ctx, s.Info.ProgramDataPath, filepath.Join(work, "helper-local"))
			if err != nil {
				t.Fatal(err)
			}
			s.Local = false // Exercise the remote archive extraction branch.
			remote, err := o.sourceIntro(ctx, s, archive, filepath.Join(work, "helper-remote"))
			if err != nil {
				t.Fatal(err)
			}
			if err = compareIntro(local.Hashes, remote.Hashes, true); err != nil {
				t.Fatal(err)
			}
			t.Log("PASS: export helper carries complete Intro Skipper snapshot")
		}
		// A download ticket is single-use and bound to the issuing device.
		s, e = OpenSource(ctx, SourceInput{Address: a.Base.String(), Username: "Import Owner", Password: password}, "")
		if e != nil {
			t.Fatal(e)
		}
		var ticket struct{ Id, Secret string }
		if e = s.API.call(ctx, "POST", "/MuttiExport/Begin", map[string]string{"Recipient": s.API.Device}, &ticket); e != nil {
			t.Fatal(e)
		}
		foreign, _ := NewAPI(a.Base.String())
		if _, e = login(ctx, foreign, "Import Owner", password); e != nil {
			t.Fatal(e)
		}
		denied, e := foreign.request(ctx, "POST", "/MuttiExport/Download", ticket)
		if e != nil {
			t.Fatal(e)
		}
		denied.Body.Close()
		foreign.logout()
		if denied.StatusCode != 404 {
			t.Fatal("foreign device accepted", denied.StatusCode)
		}
		for attempt := 0; attempt < 2; attempt++ {
			res, e := s.API.request(ctx, "POST", "/MuttiExport/Download", ticket)
			if e != nil {
				t.Fatal(e)
			}
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
			expected := 200
			if attempt == 1 {
				expected = 404
			}
			if res.StatusCode != expected {
				t.Fatal("ticket reuse", res.StatusCode)
			}
		}
		s.API.logout()
		t.Log("PASS: authenticated helper download, foreign-device rejection and single-use ticket")
	}
	m, e := NewManager(o)
	if e != nil {
		t.Fatal(e)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { defer close(done); done <- m.Run(runCtx) }()
	defer func() { stop(); <-done }()
	for i := 0; i < 90 && !m.State().Ready; i++ {
		time.Sleep(time.Second)
	}
	if !m.State().Ready {
		t.Fatal("target not ready")
	}
	input := SourceInput{Address: a.Base.String(), Username: "Import Owner", Password: password}
	if os.Getenv("MUTTI_TEST_CONFIGURED_TARGET") == "1" {
		// Reproduce the preview whose wizard is complete but whose target login
		// is not known to the importing user. Never pass it to the import.
		for {
			if e = m.api.call(ctx, "GET", "/Startup/User", nil, nil); e == nil {
				break
			}
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
			time.Sleep(200 * time.Millisecond)
		}
		if e = m.api.call(ctx, "POST", "/Startup/User", map[string]string{"Name": "Previous Preview", "Password": randomID()}, nil); e != nil {
			t.Fatal(e)
		}
		if e = m.api.call(ctx, "POST", "/Startup/Complete", nil, nil); e != nil {
			t.Fatal(e)
		}
		input.Replace = true
		input.nativeOwner = true // HTTP capability checks are exercised separately.
		t.Log("Configured target: importing with source credentials and native approval only")
	}
	if e = m.StartImport(ctx, input); e != nil {
		t.Fatal(e)
	}
	phase := ""
	for {
		s := m.State()
		if s.Phase != phase {
			t.Log(s.Phase, s.Message)
			phase = s.Phase
		}
		if s.Phase == "error" {
			t.Fatal(s.Message)
		}
		if s.Progress.Step < 1 || s.Progress.Step > 7 || s.Progress.StartedAt.IsZero() {
			t.Fatal("missing progress", s.Progress)
		}
		if s.Phase == "complete" {
			if s.Progress.Step != 7 || s.Progress.FinishedAt.IsZero() {
				t.Fatal("unfinished progress", s.Progress)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}
	if withIntro {
		owner, _ := NewAPI(o.Backend)
		if _, e = login(ctx, owner, "Import Owner", password); e != nil {
			t.Fatal(e)
		}
		if e = verifyIntroLoaded(ctx, owner); e != nil {
			t.Fatal(e)
		}
		if sourceWithIntro {
			var config struct {
				MinimumIntroDuration int
				SkipFirstEpisode     bool
			}
			if e = owner.call(ctx, "GET", "/Plugins/"+introID+"/Configuration", nil, &config); e != nil {
				t.Fatal(e)
			}
			if config.MinimumIntroDuration != 27 || !config.SkipFirstEpisode {
				t.Fatal("Intro Skipper settings lost")
			}
			var segments []struct{ Start, End float64 }
			if e = owner.call(ctx, "GET", "/Episode/"+item+"/Segments", nil, &segments); e != nil {
				t.Fatal(e)
			}
			if len(segments) != 1 || segments[0].Start != 2 || segments[0].End != 5 {
				t.Fatalf("Intro Skipper segments changed: %+v", segments)
			}
			t.Log("PASS: bundled Intro Skipper active, custom settings and existing segment preserved")
		} else {
			t.Log("PASS: Intro Skipper included even when absent on source")
		}
		owner.logout()
	}
	var imported struct {
		AccessToken string
		User        User
	}
	if e = m.api.call(ctx, "POST", "/Users/AuthenticateByName", map[string]string{"Username": "Import Viewer", "Pw": viewerPassword}, &imported); e != nil {
		t.Fatal("viewer password not preserved", e)
	}
	v, _ := NewAPI(o.Backend)
	v.Token = imported.AccessToken
	var data struct {
		IsFavorite, Played    bool
		PlaybackPositionTicks int64
		PlayCount             int
	}
	if e = v.call(ctx, "GET", "/UserItems/"+item+"/UserData?userId="+viewer.Id, nil, &data); e != nil {
		t.Fatal(e)
	}
	if !data.IsFavorite || !data.Played || data.PlayCount != 3 || data.PlaybackPositionTicks != 40000000 {
		t.Fatalf("user data changed: %+v", data)
	}
	var preferences struct {
		ShowSidebar bool
		CustomPrefs map[string]string
	}
	if e = v.call(ctx, "GET", "/DisplayPreferences/mutti-fixture?userId="+viewer.Id+"&client=emby", nil, &preferences); e != nil {
		t.Fatal(e)
	}
	if !preferences.ShowSidebar || preferences.CustomPrefs["fixture"] != "preserved" || preferences.CustomPrefs["skipForwardLength"] != "45000" {
		t.Fatal("display preferences lost")
	}
	imageResponse, e := v.request(ctx, "GET", "/UserImage?userId="+viewer.Id, nil)
	if e != nil {
		t.Fatal(e)
	}
	imageResponse.Body.Close()
	if imageResponse.StatusCode != 200 {
		t.Fatal("profile image not retained", imageResponse.StatusCode)
	}
	if !source.running() {
		t.Fatal("source was stopped")
	}
	if imported.User.Id != viewer.Id || imported.User.Policy.IsAdministrator {
		t.Fatal("user identity/rights changed")
	}
	if _, e = os.Stat(filepath.Join(o.Root, "data")); e != nil {
		t.Fatal("previous target missing")
	}
	active := m.State().Active
	stop()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	restarted, e := NewManager(o)
	if e != nil {
		t.Fatal(e)
	}
	if restarted.State().Active != active {
		t.Fatal("active instance did not survive restart")
	}
	restartCtx, stopRestart := context.WithCancel(ctx)
	restartDone := make(chan error, 1)
	go func() { restartDone <- restarted.Run(restartCtx) }()
	defer func() { stopRestart(); <-restartDone }()
	for i := 0; i < 90 && !restarted.State().Ready; i++ {
		time.Sleep(time.Second)
	}
	if !restarted.State().SetupComplete {
		t.Fatal("import was not retained across restart")
	}
	if _, e = login(ctx, restarted.api, "Import Owner", password); e != nil {
		t.Fatal("owner login after restart", e)
	}
	t.Log("PASS: existing logins, library identity, playlist, favorites, watch state, resume, source retained and restart persistence")
}
