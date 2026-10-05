// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Options struct {
	Root, Server, Web, FFmpeg, Connect, Listen, Origin, Backend, TargetOrigin, ConnectListen, ConnectOrigin, Bind string
	Container                                                                                                     bool
	IntroSkipper                                                                                                  string
	NativeOwnerToken                                                                                              string `json:"-"` // Private parent pipe, never an HTTP configuration option.
}
type process struct {
	cmd  *exec.Cmd
	done chan struct{}
	once sync.Once
	log  *os.File
}

func (p *process) stop() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		select {
		case <-p.done:
			return
		default:
		}
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-p.done:
		case <-time.After(10 * time.Second):
			_ = p.cmd.Process.Kill()
			<-p.done
		}
		_ = p.log.Close()
	})
}
func (p *process) running() bool {
	if p == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}
func startProcess(exe string, args, env []string, logPath string) (*process, error) {
	if e := os.MkdirAll(filepath.Dir(logPath), 0700); e != nil {
		return nil, e
	}
	log, e := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if e != nil {
		return nil, e
	}
	c := exec.Command(exe, args...)
	c.Env = env
	c.Stdout = log
	c.Stderr = log
	if e = c.Start(); e != nil {
		log.Close()
		return nil, e
	}
	p := &process{cmd: c, done: make(chan struct{}), log: log}
	go func() { _ = c.Wait(); log.Close(); close(p.done) }()
	return p, nil
}
func privateWrite(path string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".mutti-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e == nil {
		e = os.Rename(name, path)
	}
	return e
}
func configInstance(root string, port int, bind string) error {
	for _, dir := range []string{"config", "data", "cache", "logs"} {
		if e := os.MkdirAll(filepath.Join(root, dir), 0700); e != nil {
			return e
		}
	}
	if e := privateWrite(filepath.Join(root, "config", "network.xml"), networkXML(port, bind)); e != nil {
		return e
	}
	file := filepath.Join(root, "config", "system.xml")
	if _, e := os.Stat(file); os.IsNotExist(e) {
		return privateWrite(file, []byte(`<ServerConfiguration><ServerName>Mutti</ServerName></ServerConfiguration>`))
	}
	return nil
}
func cleanServerEnvironment(origin string, container bool) []string {
	out := []string{}
	for _, v := range os.Environ() {
		key := strings.SplitN(v, "=", 2)[0]
		if strings.HasPrefix(key, "JELLYFIN_") || strings.HasPrefix(key, "MUTTI_") || strings.HasPrefix(key, "ASPNETCORE_") {
			continue
		}
		out = append(out, v)
	}
	// Docker already used Host/Origin validation with localhost-only published
	// ports before the migration manager. Preserve that explicit package mode;
	// native and all validation processes continue requiring a loopback peer.
	localOnly := "1"
	if container {
		localOnly = "0"
	}
	return append(out, "DOTNET_CLI_TELEMETRY_OPTOUT=1", "MUTTI_LOCAL_ONLY="+localOnly, "MUTTI_PREVIEW_ORIGIN="+origin)
}
func (o Options) startServer(root string, port int, host, archive string, validation bool) (*process, error) {
	if e := o.installIntro(root); e != nil {
		return nil, e
	}
	if e := configInstance(root, port, o.Bind); e != nil {
		return nil, e
	}
	// Intro Skipper reads the configured path during construction, before the
	// server applies its command-line FFmpeg override. Keep both on our binary.
	encodingPath := filepath.Join(root, "config", "encoding.xml")
	encoding, e := os.ReadFile(encodingPath)
	if os.IsNotExist(e) {
		encoding = []byte("<EncodingOptions></EncodingOptions>")
	} else if e != nil {
		return nil, e
	}
	encoding, e = rewriteXML(encoding, nil, map[string]string{"EncoderAppPath": o.FFmpeg, "EncoderAppPathDisplay": o.FFmpeg})
	if e != nil {
		return nil, e
	}
	if e = privateWrite(encodingPath, encoding); e != nil {
		return nil, e
	}
	args := []string{"--datadir", filepath.Join(root, "data"), "--configdir", filepath.Join(root, "config"), "--cachedir", filepath.Join(root, "cache"), "--logdir", filepath.Join(root, "logs"), "--webdir", o.Web, "--ffmpeg", o.FFmpeg, "--package-name", "mutti-preview"}
	if archive != "" {
		args = append(args, "--restore-archive", archive)
	}
	exe := o.Server
	env := cleanServerEnvironment("http://"+host, o.Container && !validation)
	if validation {
		env = append(env, "MUTTI_IMPORT_VALIDATION=1")
	} else {
		env = append(env, "MUTTI_MANAGEMENT_ORIGIN="+o.Origin)
		if o.Connect != "" {
			_, connectPort, err := net.SplitHostPort(o.ConnectListen)
			if err != nil {
				return nil, errors.New("Ungültiger lokaler Kopplungszugang.")
			}
			env = append(env, "MUTTI_CONNECT_ADMIN_ORIGIN="+o.ConnectOrigin, "MUTTI_CONNECT_ADMIN_PORT="+connectPort)
		}
	}
	if validation && runtime.GOOS == "darwin" {
		// The staged server can read the library but cannot write to source media,
		// source configuration, the user's home or any other installation.
		canonicalRoot, e := filepath.EvalSymlinks(root)
		if e != nil {
			return nil, e
		}
		temp := filepath.Join(canonicalRoot, "tmp")
		if e = os.MkdirAll(temp, 0700); e != nil {
			return nil, e
		}
		filtered := []string{}
		for _, v := range env {
			if !strings.HasPrefix(v, "TMPDIR=") {
				filtered = append(filtered, v)
			}
		}
		env = append(filtered, "TMPDIR="+temp)
		profile := fmt.Sprintf(`(version 1) (allow default) (deny file-write*) (allow file-write* (subpath %s) (literal "/dev/null")) (deny network-outbound) (allow network-outbound (remote ip "localhost:*"))`, strconv.Quote(canonicalRoot))
		args = append([]string{"-p", profile, o.Server}, args...)
		exe = "/usr/bin/sandbox-exec"
	}
	return startProcess(exe, args, env, filepath.Join(root, "logs", "launcher.log"))
}
func freePort() (int, error) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return 0, e
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
func waitServer(ctx context.Context, p *process, api *API) (PublicInfo, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !p.running() {
			return PublicInfo{}, errors.New("Die importierte Testinstanz konnte nicht gestartet werden. Der bisherige Datenstand bleibt aktiv.")
		}
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		var info PublicInfo
		e := api.call(callCtx, "GET", "/System/Info/Public", nil, &info)
		ready := e == nil && info.Id != "" && api.call(callCtx, "GET", "/Users/Public", nil, nil) == nil
		cancel()
		if ready {
			return info, nil
		}
		select {
		case <-ctx.Done():
			return info, errors.New("Die Prüfung der importierten Instanz hat zu lange gedauert.")
		case <-ticker.C:
		}
	}
}
