// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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

// engine runs or reaches the local inference engine. The managed engine on
// macOS is confined by the OS sandbox to loopback; only an explicit owner
// download temporarily starts an unconfined downloader on the same store.
type engine struct {
	mu       sync.Mutex
	binary   string
	sandbox  bool
	dir      string
	mode     string
	external string
	port     int
	proc     *exec.Cmd
	done     chan struct{}
	state    string
	message  string
	lock     string // "verified", "unverified", "not_applicable"
	download *downloadProgress
	client   *http.Client
	stream   *http.Client
	starts   int // processes started, so attestation can re-measure
}

// managedBinary reports whether the hub runs its own bundled engine.
func (e *engine) managedBinary() (bool, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mode == "managed", e.binary
}

// managedBase returns the managed engine's address, or false if the hub
// uses an external engine or the managed one is not ready.
func (e *engine) managedBase() (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.mode != "managed" || e.state != "ready" {
		return "", false
	}
	return fmt.Sprintf("http://127.0.0.1:%d", e.port), true
}

func (e *engine) startCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.starts
}

type downloadProgress struct {
	Model     string `json:"model"`
	Status    string `json:"status"`
	Completed int64  `json:"completed"`
	Total     int64  `json:"total"`
	Error     string `json:"error,omitempty"`
}

func newEngine(dir, binary string, sandbox bool) *engine {
	stream := guardedClient(0)
	// Loading a large model can take a while before the first byte arrives.
	stream.Transport.(*http.Transport).ResponseHeaderTimeout = 3 * time.Minute
	return &engine{dir: dir, binary: binary, sandbox: sandbox && runtime.GOOS == "darwin", mode: "managed", state: "stopped", lock: "not_applicable",
		client: guardedClient(2 * time.Minute), stream: stream}
}

func (e *engine) modelsDir() string { return filepath.Join(e.dir, "models") }

const sandboxProfile = `(version 1) (allow default) (deny network-outbound) (allow network-outbound (remote ip "localhost:*")) (allow network-outbound (remote unix-socket))`

// configure switches between managed and external mode. External engines must
// be on loopback or the private network and never a cloud provider.
func (e *engine) configure(cfg *Engine) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cfg.Mode == e.mode && cfg.URL == e.external {
		return
	}
	e.stopLocked()
	e.mode, e.external = cfg.Mode, cfg.URL
	if e.mode == "external" {
		e.state, e.lock = "ready", "not_applicable"
	} else {
		e.state = "stopped"
	}
}

func (e *engine) base() (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.mode == "external" {
		return e.external, nil
	}
	if e.state != "ready" {
		return "", apiErr(503, "engine_not_ready", "Die lokale KI startet gerade oder ist nicht bereit.")
	}
	return fmt.Sprintf("http://127.0.0.1:%d", e.port), nil
}

// ensure starts the managed engine if it is not running.
func (e *engine) ensure(ctx context.Context) error {
	e.mu.Lock()
	if e.mode == "external" || e.state == "ready" || e.state == "downloading" {
		e.mu.Unlock()
		return nil
	}
	if e.binary == "" {
		e.state, e.message = "not_installed", "Dieses Paket enthält keine lokale KI-Engine."
		e.mu.Unlock()
		return apiErr(503, "engine_missing", e.message)
	}
	if e.state != "starting" {
		if err := e.startLocked(true); err != nil {
			e.state, e.message = "failed", "Die lokale KI-Engine konnte nicht gestartet werden."
			e.mu.Unlock()
			return apiErr(503, "engine_failed", e.message)
		}
	}
	e.mu.Unlock()
	return e.waitReady(ctx)
}

func (e *engine) startLocked(confined bool) error {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	e.port = l.Addr().(*net.TCPAddr).Port
	l.Close()
	home := filepath.Join(e.dir, "home")
	for _, d := range []string{e.modelsDir(), home} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
	}
	env := []string{"HOME=" + home, "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "OLLAMA_HOST=127.0.0.1:" + strconv.Itoa(e.port), "OLLAMA_MODELS=" + e.modelsDir(),
		"OLLAMA_NO_CLOUD=1", "OLLAMA_NUM_PARALLEL=1", "OLLAMA_MAX_LOADED_MODELS=1", "OLLAMA_KEEP_ALIVE=5m", "OLLAMA_CONTEXT_LENGTH=8192",
		"OLLAMA_NOPRUNE=1", "TMPDIR=" + filepath.Join(e.dir, "home")}
	exe, args := e.binary, []string{"serve"}
	if confined && e.sandbox {
		exe, args = "/usr/bin/sandbox-exec", []string{"-p", sandboxProfile, e.binary, "serve"}
	}
	log, err := os.OpenFile(filepath.Join(e.dir, "engine.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, args...)
	cmd.Env, cmd.Stdout, cmd.Stderr = env, log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = cmd.Start(); err != nil {
		log.Close()
		return err
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); log.Close(); close(done) }()
	e.proc, e.done, e.state = cmd, done, "starting"
	e.starts++
	if confined {
		e.lock = "unverified"
		if e.sandbox {
			if verifySandbox() {
				e.lock = "verified"
			}
		} else {
			e.lock = "not_applicable"
		}
	}
	go func() {
		<-done
		e.mu.Lock()
		if e.proc == cmd && e.state != "downloading" {
			e.state, e.message, e.proc = "failed", "Die lokale KI-Engine wurde unerwartet beendet.", nil
		}
		e.mu.Unlock()
	}()
	return nil
}

// verifySandbox proves the profile denies a non-loopback connection at the OS
// level, independent of the engine's own configuration. nc only reports the
// kernel's EPERM ("Operation not permitted") in verbose mode; an unconfined
// attempt to the TEST-NET address times out instead.
func verifySandbox() bool {
	out, err := exec.Command("/usr/bin/sandbox-exec", "-p", sandboxProfile, "/usr/bin/nc", "-v", "-z", "-G", "1", "192.0.2.1", "80").CombinedOutput()
	return err != nil && strings.Contains(strings.ToLower(string(out)), "not permitted")
}

func (e *engine) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		e.mu.Lock()
		state, port := e.state, e.port
		e.mu.Unlock()
		if state == "ready" || state == "downloading" {
			return nil
		}
		if state == "failed" {
			return apiErr(503, "engine_failed", "Die lokale KI-Engine konnte nicht gestartet werden.")
		}
		c, cancel := context.WithTimeout(ctx, time.Second)
		req, _ := http.NewRequestWithContext(c, "GET", fmt.Sprintf("http://127.0.0.1:%d/api/version", port), nil)
		res, err := e.client.Do(req)
		cancel()
		if err == nil {
			res.Body.Close()
			if res.StatusCode == 200 {
				e.mu.Lock()
				if e.port == port && e.state == "starting" {
					e.state, e.message = "ready", ""
				}
				e.mu.Unlock()
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return apiErr(503, "engine_timeout", "Die lokale KI-Engine antwortet nicht.")
}

func (e *engine) stopLocked() {
	if e.proc == nil || e.proc.Process == nil {
		return
	}
	proc, done := e.proc, e.done
	e.proc = nil
	// The engine spawns runner children; end the whole process group.
	_ = syscall.Kill(-proc.Process.Pid, syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = syscall.Kill(-proc.Process.Pid, syscall.SIGKILL)
		<-done
	}
	e.state = "stopped"
}

func (e *engine) stop() {
	e.mu.Lock()
	e.stopLocked()
	e.mu.Unlock()
}

type engineStatus struct {
	Mode        string            `json:"mode"`
	State       string            `json:"state"`
	Message     string            `json:"message,omitempty"`
	NetworkLock string            `json:"networkLock"`
	Download    *downloadProgress `json:"download,omitempty"`
	Installed   bool              `json:"installed"`
}

func (e *engine) status() engineStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	var d *downloadProgress
	if e.download != nil {
		copy := *e.download
		d = &copy
	}
	return engineStatus{Mode: e.mode, State: e.state, Message: e.message, NetworkLock: e.lock, Download: d, Installed: e.binary != "" || e.mode == "external"}
}

type installedModel struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

func (e *engine) installed(ctx context.Context) ([]installedModel, error) {
	base, err := e.base()
	if err != nil {
		return nil, err
	}
	var out struct {
		Models []installedModel `json:"models"`
	}
	if err = serviceCall(ctx, e.client, "GET", base+"/api/tags", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Models, nil
}

// verified reports whether the engine holds exactly the pinned artifact.
func (e *engine) verified(ctx context.Context, m CatalogModel) bool {
	models, err := e.installed(ctx)
	if err != nil {
		return false
	}
	for _, installed := range models {
		if installed.Name == m.ID && installed.Digest == m.Digest {
			return true
		}
	}
	return false
}

// pull downloads one catalog model. In managed mode the confined engine is
// stopped meanwhile; chat requests receive a clear "downloading" state.
func (e *engine) pull(m CatalogModel) error {
	e.mu.Lock()
	if e.download != nil && e.download.Status != "done" && e.download.Status != "failed" {
		e.mu.Unlock()
		return apiErr(409, "busy", "Es läuft bereits ein Download.")
	}
	managed := e.mode == "managed"
	if managed {
		if e.binary == "" {
			e.mu.Unlock()
			return apiErr(503, "engine_missing", "Dieses Paket enthält keine lokale KI-Engine.")
		}
		e.stopLocked()
		if err := e.startLocked(false); err != nil {
			e.state = "failed"
			e.mu.Unlock()
			return apiErr(503, "engine_failed", "Der Download konnte nicht gestartet werden.")
		}
		e.state = "downloading"
	}
	e.download = &downloadProgress{Model: m.ID, Status: "starting"}
	e.mu.Unlock()
	go e.runPull(m, managed)
	return nil
}

func (e *engine) runPull(m CatalogModel, managed bool) {
	fail := func(msg string) {
		e.mu.Lock()
		e.download.Status, e.download.Error = "failed", msg
		e.mu.Unlock()
	}
	defer func() {
		if managed {
			e.mu.Lock()
			e.stopLocked()
			e.state = "stopped"
			e.mu.Unlock()
			_ = e.ensure(context.Background())
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	var base string
	e.mu.Lock()
	if managed {
		base = fmt.Sprintf("http://127.0.0.1:%d", e.port)
	} else {
		base = e.external
	}
	e.mu.Unlock()
	for i := 0; i < 60 && managed; i++ {
		res, err := e.client.Get(base + "/api/version")
		if err == nil {
			res.Body.Close()
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	body, _ := json.Marshal(map[string]any{"model": m.ID, "stream": true})
	req, _ := http.NewRequestWithContext(ctx, "POST", base+"/api/pull", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// The downloader itself fetches from the official registry; Mutti only
	// talks to the engine on loopback or the owner's private network.
	res, err := e.stream.Do(req)
	if err != nil {
		fail("Der Download konnte nicht gestartet werden.")
		return
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		var ev struct {
			Status    string `json:"status"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
			Error     string `json:"error"`
		}
		if json.Unmarshal(scanner.Bytes(), &ev) != nil {
			continue
		}
		if ev.Error != "" {
			fail("Der Download ist fehlgeschlagen. Bitte Internetverbindung und freien Speicher prüfen.")
			return
		}
		e.mu.Lock()
		e.download.Status = ev.Status
		if ev.Total > 0 {
			e.download.Total, e.download.Completed = ev.Total, ev.Completed
		}
		e.mu.Unlock()
	}
	if scanner.Err() != nil {
		fail("Der Download wurde unterbrochen. Bereits geladene Teile werden beim nächsten Versuch weiterverwendet.")
		return
	}
	var tags struct {
		Models []installedModel `json:"models"`
	}
	if serviceCall(ctx, e.client, "GET", base+"/api/tags", nil, nil, &tags) != nil {
		fail("Das geladene Modell konnte nicht geprüft werden.")
		return
	}
	for _, t := range tags.Models {
		if t.Name == m.ID && t.Digest == m.Digest {
			e.mu.Lock()
			e.download.Status = "done"
			e.mu.Unlock()
			return
		}
	}
	fail("Das geladene Modell stimmt nicht mit der geprüften Version überein und wird nicht verwendet.")
}

// adopt copies an already downloaded artifact from another local engine store
// (e.g. an existing Ollama installation) after verifying every layer digest.
// The source store is only read.
func (e *engine) adopt(m CatalogModel, source string) error {
	name, tag, _ := strings.Cut(m.ID, ":")
	manifestPath := filepath.Join(source, "manifests", "registry.ollama.ai", "library", name, tag)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return apiErr(404, "not_found", "Dieses Modell liegt in der angegebenen lokalen Installation nicht vor.")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != m.Digest {
		return apiErr(409, "digest_mismatch", "Die vorhandene Modellversion ist nicht die geprüfte Version.")
	}
	var manifest struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
		Layers []struct {
			Digest string `json:"digest"`
			Size   int64  `json:"size"`
		} `json:"layers"`
	}
	if json.Unmarshal(raw, &manifest) != nil {
		return errInvalid
	}
	blobs := []string{manifest.Config.Digest}
	for _, l := range manifest.Layers {
		blobs = append(blobs, l.Digest)
	}
	e.mu.Lock()
	if e.download != nil && e.download.Status != "done" && e.download.Status != "failed" {
		e.mu.Unlock()
		return apiErr(409, "busy", "Es läuft bereits ein Download.")
	}
	e.download = &downloadProgress{Model: m.ID, Status: "adopting", Total: m.Bytes}
	e.mu.Unlock()
	go func() {
		err := e.copyVerified(source, blobs, m)
		if err == nil {
			target := filepath.Join(e.modelsDir(), "manifests", "registry.ollama.ai", "library", name, tag)
			if err = os.MkdirAll(filepath.Dir(target), 0700); err == nil {
				err = writePrivate(target, raw)
			}
		}
		e.mu.Lock()
		if err != nil {
			e.download.Status, e.download.Error = "failed", "Die Übernahme ist fehlgeschlagen; das Modell wird nicht verwendet."
		} else {
			e.download.Status, e.download.Completed = "done", m.Bytes
		}
		e.mu.Unlock()
	}()
	return nil
}

func (e *engine) copyVerified(source string, digests []string, m CatalogModel) error {
	dir := filepath.Join(e.modelsDir(), "blobs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, d := range digests {
		name := strings.Replace(d, ":", "-", 1)
		if !strings.HasPrefix(name, "sha256-") || strings.ContainsAny(name, "/\\") {
			return errors.New("invalid digest")
		}
		target := filepath.Join(dir, name)
		if _, err := os.Stat(target); err == nil {
			// An existing blob is only kept if its content matches its name.
			if sum, err := fileDigest(target); err == nil && "sha256-"+sum == name {
				continue
			}
			if err := os.Remove(target); err != nil {
				return err
			}
		}
		in, err := os.Open(filepath.Join(source, "blobs", name))
		if err != nil {
			return err
		}
		tmp, err := os.CreateTemp(dir, ".adopt-")
		if err != nil {
			in.Close()
			return err
		}
		h := sha256.New()
		counter := &progressWriter{e: e}
		_, err = io.Copy(io.MultiWriter(tmp, h, counter), in)
		in.Close()
		if closeErr := tmp.Close(); err == nil {
			err = closeErr
		}
		if err == nil && "sha256-"+hex.EncodeToString(h.Sum(nil)) != name {
			err = errors.New("digest mismatch")
		}
		if err == nil {
			err = os.Rename(tmp.Name(), target)
		}
		if err != nil {
			os.Remove(tmp.Name())
			return err
		}
	}
	return nil
}

type progressWriter struct{ e *engine }

func (p *progressWriter) Write(b []byte) (int, error) {
	p.e.mu.Lock()
	p.e.download.Completed += int64(len(b))
	p.e.mu.Unlock()
	return len(b), nil
}

func (e *engine) remove(ctx context.Context, id string) error {
	base, err := e.base()
	if err != nil {
		return err
	}
	return serviceCall(ctx, e.client, "DELETE", base+"/api/delete", nil, map[string]string{"model": id}, nil)
}

// systemMemoryGB reports physical memory for hardware-profile decisions.
func systemMemoryGB() int {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("/usr/sbin/sysctl", "-n", "hw.memsize").Output()
		if err == nil {
			if n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil {
				return int(n >> 30)
			}
		}
		return 0
	}
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					return int(kb >> 20)
				}
			}
		}
	}
	return 0
}
