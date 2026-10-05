// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Only used by the explicit synthetic migration test. Registers a unique user
// service pointing at that test's private data and removes it again afterwards.
func launchSyntheticSource(t *testing.T, previous *process, api *API, info PublicInfo) func() {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Fatal("launchd fixture requires macOS")
	}
	label := "org.ralleur.mutti-import-test-" + randomID()
	home, _ := os.UserHomeDir()
	plist := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
	service := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	env := map[string]string{}
	for _, entry := range previous.cmd.Env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 && (parts[0] == "PATH" || strings.HasPrefix(parts[0], "MUTTI_") || strings.HasPrefix(parts[0], "DOTNET_")) {
			env[parts[0]] = parts[1]
		}
	}
	var logPath string
	for i, arg := range previous.cmd.Args {
		if arg == "--logdir" && i+1 < len(previous.cmd.Args) {
			logPath = filepath.Join(previous.cmd.Args[i+1], "launchd-test.log")
		}
	}
	payload := map[string]any{"Label": label, "ProgramArguments": previous.cmd.Args, "KeepAlive": true, "RunAtLoad": true, "EnvironmentVariables": env, "StandardOutPath": logPath, "StandardErrorPath": logPath}
	data, _ := json.Marshal(payload)
	previous.stop()
	// privateWrite uses exclusive random temp files; no existing LaunchAgent name
	// is reused by this fixture.
	if e := privateWrite(plist, data); e != nil {
		t.Fatal(e)
	}
	if out, e := exec.Command("/usr/bin/plutil", "-convert", "xml1", plist).CombinedOutput(); e != nil {
		t.Fatal("fixture plist", e, string(out))
	}
	os.Chmod(plist, 0600)
	cleanup := func() { exec.Command("/bin/launchctl", "bootout", service).Run(); os.Remove(plist) }
	t.Cleanup(cleanup)
	if out, e := exec.Command("/bin/launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), plist).CombinedOutput(); e != nil {
		t.Fatal("fixture bootstrap", e, string(out))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		got, e := sameSource(ctx, api, info.Id)
		if e == nil && got.StartupWizardCompleted {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture source did not start")
		case <-time.After(200 * time.Millisecond):
		}
	}
	// A real asynchronous backup in Pessimistic mode reproduces the owner stall.
	// Only the synthetic database is involved. Its timed-out request must not
	// cause a second backup before the service has restarted in NoLock mode.
	blocked, cancelBlocked := context.WithTimeout(context.Background(), 3*time.Second)
	var result struct{ Path string }
	err := api.call(blocked, "POST", "/Backup/Create", map[string]bool{"Database": true}, &result)
	cancelBlocked()
	if err != nil {
		t.Log("Synthetic source backup is blocked; native preparation must recover its user service")
	} else {
		t.Log("Synthetic source uses Pessimistic; native preparation must restart before import")
	}
	return cleanup
}
