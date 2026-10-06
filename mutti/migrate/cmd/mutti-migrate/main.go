// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"flag"
	"fmt"
	migrate "github.com/ralleur/mutti/migrate"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	var o migrate.Options
	flag.StringVar(&o.Root, "root", "", "Private Mutti data directory")
	flag.StringVar(&o.Server, "server", "", "Jellyfin executable")
	flag.StringVar(&o.Web, "web", "", "Jellyfin web directory")
	flag.StringVar(&o.FFmpeg, "ffmpeg", "", "FFmpeg executable")
	flag.StringVar(&o.IntroSkipper, "intro-skipper", "", "Bundled, checksum-pinned Intro Skipper directory")
	flag.StringVar(&o.Connect, "connect", "", "Optional Mutti Connect executable")
	flag.StringVar(&o.Listen, "listen", "127.0.0.1:18594", "Onboarding listener")
	flag.StringVar(&o.Origin, "origin", "http://127.0.0.1:18594", "Onboarding browser origin")
	flag.StringVar(&o.Backend, "backend", "http://127.0.0.1:18596", "Jellyfin loopback URL")
	flag.StringVar(&o.TargetOrigin, "target-origin", "http://127.0.0.1:18596", "Jellyfin browser origin")
	flag.StringVar(&o.ConnectListen, "connect-listen", "127.0.0.1:18595", "Pairing admin listener")
	flag.StringVar(&o.ConnectOrigin, "connect-origin", "http://127.0.0.1:18595", "Pairing admin origin")
	flag.StringVar(&o.Bind, "bind", "127.0.0.1", "Jellyfin bind address")
	flag.BoolVar(&o.Container, "container", false, "Preserve the Docker package loopback-published network boundary")
	nativeOwner := flag.Bool("native-owner-stdin", false, "Read the Mac app's private per-launch capability from its inherited pipe")
	flag.Parse()
	if *nativeOwner {
		// Lead a process group of our own before anything spawns. Jellyfin and
		// Connect inherit it, so the Mac app can kill the whole group as a last
		// resort. Errors (already a leader, EPERM) are harmless and ignored.
		_ = syscall.Setpgid(0, 0)
		// The Mac app writes exactly 64 hex characters and then keeps the pipe
		// open for as long as it lives; see WatchLifeline below.
		secret := make([]byte, 64)
		if _, err := io.ReadFull(os.Stdin, secret); err != nil {
			fmt.Fprintln(os.Stderr, "Invalid native owner pipe")
			os.Exit(2)
		}
		o.NativeOwnerToken = string(secret)
	}
	if o.Root == "" || o.Server == "" || o.Web == "" || o.FFmpeg == "" {
		fmt.Fprintln(os.Stderr, "root, server, web and ffmpeg are required")
		os.Exit(2)
	}
	listener, e := net.Listen("tcp", o.Listen)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	m, e := migrate.NewManager(o)
	if e != nil {
		listener.Close()
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	handler, e := m.Handler()
	if e != nil {
		listener.Close()
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *nativeOwner {
		// EOF on the inherited pipe means the Mac app is gone: shut down on the
		// same path as SIGTERM so Jellyfin and Connect never outlive the app.
		// Docker's stdin is /dev/null and must not trigger this.
		migrate.WatchLifeline(os.Stdin, cancel)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx); cancel() }()
	go func() {
		if e := server.Serve(listener); e != nil && e != http.ErrServerClosed {
			cancel()
		}
	}()
	<-ctx.Done()
	c, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	_ = server.Shutdown(c)
	if e := <-done; e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
