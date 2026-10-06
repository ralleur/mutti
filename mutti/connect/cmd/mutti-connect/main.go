// SPDX-License-Identifier: MPL-2.0
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/pion/stun/v4"
	connect "github.com/ralleur/mutti/connect"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func serve(ctx context.Context, address string, handler http.Handler) {
	listener, e := net.Listen("tcp", address)
	if e != nil {
		fmt.Fprintln(os.Stderr, "Mutti Connect: Listener nicht verfügbar.")
		os.Exit(1)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 45 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 45 * time.Second, MaxHeaderBytes: 32768}
	go func() { <-ctx.Done(); _ = server.Close() }()
	go func() { _ = server.Serve(listener) }()
}
func lanAddress() string {
	interfaces, _ := net.Interfaces()
	for _, i := range interfaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || strings.HasPrefix(i.Name, "utun") {
			continue
		}
		addresses, _ := i.Addrs()
		for _, a := range addresses {
			ip, _, e := net.ParseCIDR(a.String())
			if e == nil && ip.To4() != nil && ip.IsPrivate() {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}
func main() {
	mode := flag.String("mode", "server", "server, broker or client")
	state := flag.String("state", "", "private state directory / credentials file for client")
	broker := flag.String("broker", os.Getenv("MUTTI_SIGNAL_URL"), "HTTPS rendezvous origin (empty = LAN only)")
	stun := flag.String("stun", os.Getenv("MUTTI_STUN_URL"), "STUN URL, never TURN")
	adminOrigin := flag.String("admin-origin", "", "expected admin browser origin (defaults to listen address)")
	listen := flag.String("listen", "127.0.0.1:18595", "admin/client bind address, or broker HTTP listener")
	lan := flag.String("lan", "0.0.0.0:18599", "LAN rendezvous bind address")
	advertise := flag.String("advertise", os.Getenv("MUTTI_LAN_ORIGIN"), "LAN rendezvous URL advertised in QR")
	target := flag.String("target", "http://127.0.0.1:18596", "fixed Jellyfin loopback origin")
	targetHost := flag.String("target-host", "127.0.0.1:18596", "Jellyfin boundary Host header")
	stunListen := flag.String("stun-listen", ":3478", "broker STUN UDP listener")
	hubTarget := flag.String("hub", "", "fixed loopback module service origin (optional)")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch *mode {
	case "broker":
		serve(ctx, *listen, connect.NewBroker())
		go serveSTUN(ctx, *stunListen)
		fmt.Println("Mutti rendezvous ready; media relay disabled.")
	case "server":
		if *state == "" {
			fmt.Fprintln(os.Stderr, "--state is required")
			os.Exit(1)
		}
		if *broker == "" {
			_, port, e := net.SplitHostPort(*lan)
			if e != nil {
				panic(e)
			}
			if *advertise == "" {
				*advertise = "http://" + net.JoinHostPort(lanAddress(), port)
			}
			*broker = *advertise
			serve(ctx, *lan, connect.NewBroker())
		}
		server, e := connect.NewServer(*state, *broker, *stun, *target, *targetHost)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		if *hubTarget != "" {
			if e = server.SetHub(*hubTarget, os.Getenv("MUTTI_HUB_PEER")); e != nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(1)
			}
		}
		if *adminOrigin == "" {
			*adminOrigin = "http://" + *listen
		}
		serve(ctx, *listen, server.Admin(*adminOrigin))
		go server.Run(ctx)
		fmt.Println("Mutti Connect ready; media relay disabled.")
	case "client":
		b, e := os.ReadFile(*state)
		if e != nil {
			panic(e)
		}
		var credentials connect.Credentials
		if e = json.Unmarshal(b, &credentials); e != nil {
			panic(e)
		}
		client, e := connect.NewClient(credentials)
		if e != nil {
			panic(e)
		}
		defer client.Close()
		address, e := client.Gateway()
		if e != nil {
			panic(e)
		}
		fmt.Println(address)
	default:
		fmt.Fprintln(os.Stderr, "invalid mode")
		os.Exit(1)
	}
	// Docker uses one supervised process tree and waits for Jellyfin to shut down.
	if flag.NArg() > 0 {
		args := flag.Args()
		child := exec.Command(args[0], args[1:]...)
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if e := child.Start(); e != nil {
			panic(e)
		}
		done := make(chan struct{})
		go func() { _ = child.Wait(); close(done) }()
		select {
		case <-done:
			cancel()
			return
		case <-ctx.Done():
		}
		_ = child.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			_ = child.Process.Kill()
			<-done
		}
		return
	}
	<-ctx.Done()
}

func serveSTUN(ctx context.Context, address string) {
	socket, e := net.ListenPacket("udp", address)
	if e != nil {
		fmt.Fprintln(os.Stderr, "STUN listener unavailable")
		return
	}
	defer socket.Close()
	go func() { <-ctx.Done(); _ = socket.Close() }()
	buffer := make([]byte, 1500)
	// Binding only, no allocations/auth/relay. Rate bounded globally; responses
	// are approximately request-sized. Invalid or oversized packets are ignored.
	budget := time.NewTicker(10 * time.Millisecond)
	defer budget.Stop()
	for {
		n, addr, e := socket.ReadFrom(buffer)
		if e != nil {
			return
		}
		if n < 20 || n > 256 {
			continue
		}
		select {
		case <-budget.C:
		default:
			continue
		}
		message := &stun.Message{Raw: append([]byte(nil), buffer[:n]...)}
		if message.Decode() != nil || message.Type != stun.BindingRequest {
			continue
		}
		peer, ok := addr.(*net.UDPAddr)
		if !ok {
			continue
		}
		answer, e := stun.Build(stun.NewTransactionIDSetter(message.TransactionID), stun.BindingSuccess, &stun.XORMappedAddress{IP: peer.IP, Port: peer.Port}, stun.Fingerprint)
		if e == nil {
			_, _ = socket.WriteTo(answer.Raw, addr)
		}
	}
}
