// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"flag"
	"fmt"
	migrate "github.com/ralleur/mutti/migrate"
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
	flag.StringVar(&o.Connect, "connect", "", "Optional Mutti Connect executable")
	flag.StringVar(&o.Listen, "listen", "127.0.0.1:18594", "Onboarding listener")
	flag.StringVar(&o.Origin, "origin", "http://127.0.0.1:18594", "Onboarding browser origin")
	flag.StringVar(&o.Backend, "backend", "http://127.0.0.1:18596", "Jellyfin loopback URL")
	flag.StringVar(&o.TargetOrigin, "target-origin", "http://127.0.0.1:18596", "Jellyfin browser origin")
	flag.StringVar(&o.ConnectListen, "connect-listen", "127.0.0.1:18595", "Pairing admin listener")
	flag.StringVar(&o.ConnectOrigin, "connect-origin", "http://127.0.0.1:18595", "Pairing admin origin")
	flag.StringVar(&o.Bind, "bind", "127.0.0.1", "Jellyfin bind address")
	flag.BoolVar(&o.Container, "container", false, "Preserve the Docker package loopback-published network boundary")
	flag.Parse()
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
