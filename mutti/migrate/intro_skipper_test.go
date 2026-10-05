// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCuratedPluginIdentity(t *testing.T) {
	for _, id := range []string{introID, strings.ReplaceAll(introID, "-", "")} {
		ok, e := qualifyPlugins([]PluginInfo{{Id: id, Name: "Intro Skipper", Version: introVersion, Status: "Active", CanUninstall: true}})
		if !ok || e != nil {
			t.Fatal("supported plugin not recognized", e)
		}
	}
	for _, p := range []PluginInfo{
		{Id: "other", Name: "Intro Skipper", Version: introVersion, Status: "Active", CanUninstall: true},
		{Id: introID, Name: "Intro Skipper", Version: "99.0.0.0", Status: "Active", CanUninstall: true},
		{Id: "other", Name: "Unknown plugin", Status: "Active", CanUninstall: true},
	} {
		if _, e := qualifyPlugins([]PluginInfo{p}); e == nil {
			t.Fatal("unqualified plugin accepted")
		}
	}
	if yes, e := qualifyPlugins(nil); yes || e != nil {
		t.Fatal("source without plugin rejected")
	}
}
func TestIntroBinaryAndDataChecks(t *testing.T) {
	bundle := t.TempDir()
	_ = os.WriteFile(filepath.Join(bundle, "IntroSkipper.dll"), []byte("untrusted executable"), 0600)
	if e := (Options{IntroSkipper: bundle}).installIntro(t.TempDir()); e == nil {
		t.Fatal("tampered bundle accepted")
	}
	if compareIntro(map[string]string{"introskipper-v2.db": "original"}, map[string]string{"introskipper-v2.db": "changed"}, false) == nil {
		t.Fatal("changed segments accepted")
	}
	if compareIntro(map[string]string{}, map[string]string{"introskipper-v2.db": "new"}, true) == nil {
		t.Fatal("concurrent source creation missed")
	}
	snapshot := t.TempDir()
	_ = os.WriteFile(filepath.Join(snapshot, "snapshot.json"), []byte(`{"../outside":"`+strings.Repeat("a", 64)+`"}`), 0600)
	if _, e := readIntroSnapshot(snapshot); e == nil {
		t.Fatal("snapshot traversal accepted")
	}
}

func TestFreshEncodingHasPinnedFFmpegBeforePluginStarts(t *testing.T) {
	b, e := rewriteXML([]byte("<EncodingOptions></EncodingOptions>"), nil, map[string]string{"EncoderAppPath": "/bundle/ffmpeg", "EncoderAppPathDisplay": "/bundle/ffmpeg"})
	if e != nil || !strings.Contains(string(b), "<EncoderAppPathDisplay>/bundle/ffmpeg</EncoderAppPathDisplay>") {
		t.Fatal("fresh config missing plugin FFmpeg path", e)
	}
	updated, e := rewriteXML(b, nil, map[string]string{"EncoderAppPath": "/new/ffmpeg", "EncoderAppPathDisplay": "/new/ffmpeg"})
	if e != nil || strings.Count(string(updated), "<EncoderAppPathDisplay>") != 1 || strings.Contains(string(updated), "/bundle/") {
		t.Fatal("existing config duplicated or stale", e)
	}
}
