// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	hub "github.com/ralleur/mutti/hub"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "qualify" {
		qualify(os.Args[2:])
		return
	}
	var o hub.Options
	listen := flag.String("listen", "127.0.0.1:18593", "Loopback listener for Connect and the Jellyfin bridge")
	flag.StringVar(&o.State, "state", "", "Private hub data directory")
	flag.StringVar(&o.Jellyfin, "jellyfin", "http://127.0.0.1:18596", "Fixed loopback Jellyfin origin")
	flag.StringVar(&o.Host, "jellyfin-host", "", "Host header expected by Jellyfin")
	flag.StringVar(&o.Ollama, "ollama", "", "Engine executable for the managed local AI mode")
	flag.BoolVar(&o.Sandbox, "sandbox", true, "Confine the managed engine to loopback (macOS)")
	flag.Parse()
	o.PeerSecret = os.Getenv("MUTTI_HUB_PEER")
	if o.State == "" {
		fmt.Fprintln(os.Stderr, "--state is required")
		os.Exit(2)
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		fmt.Fprintln(os.Stderr, "the hub listens on loopback only")
		os.Exit(2)
	}
	h, err := hub.New(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	l, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	server := &http.Server{Handler: h.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	go h.Run(ctx)
	go func() {
		if err := server.Serve(l); err != nil && err != http.ErrServerClosed {
			cancel()
		}
	}()
	<-ctx.Done()
	stop, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	_ = server.Shutdown(stop)
	time.Sleep(200 * time.Millisecond)
}

// qualify measures this binary with its bundled engine against a synthetic
// instance and writes evidence plus unsigned candidates. It grants nothing.
func qualify(args []string) {
	fs := flag.NewFlagSet("qualify", flag.ExitOnError)
	var o hub.QualifyOptions
	fs.StringVar(&o.State, "state", "", "hub state directory of the synthetic instance")
	fs.StringVar(&o.Jellyfin, "jellyfin", "", "loopback Jellyfin origin of the instance")
	fs.StringVar(&o.JellyfinHost, "jellyfin-host", "", "Host header Jellyfin expects")
	fs.StringVar(&o.Ollama, "ollama", "", "bundled engine executable")
	fs.StringVar(&o.Models, "models", "", "verified model store")
	fs.StringVar(&o.Model, "model", "", "catalog model ID")
	fs.StringVar(&o.Suite, "suite", "", "frozen qualification suite")
	fs.StringVar(&o.Profiles, "profiles", "", "JSON file with test profile tokens")
	fs.StringVar(&o.Objects, "objects", "", "JSON file with fixture object IDs")
	fs.StringVar(&o.Output, "out", "", "new result directory")
	fs.IntVar(&o.Repetitions, "repetitions", 3, "repetitions per case")
	_ = fs.Parse(args)
	if err := hub.RunQualification(o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
