// SPDX-License-Identifier: GPL-2.0-or-later
// Disposable experiment only. testcontrol must never be deployed.
package directlab

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"tailscale.com/net/netns"
	"tailscale.com/net/stun"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
	"tailscale.com/tstest/integration/testcontrol"
	"tailscale.com/types/logger"
)

func TestDirectWithoutAnyDERPServer(t *testing.T) {
	t.Setenv("TS_NO_LOGS_NO_SUPPORT", "true")
	t.Setenv("TS_DISABLE_PORTMAPPER", "true")
	netns.SetEnabled(false)
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	go func() {
		b := make([]byte, 65536)
		for {
			n, addr, err := udp.ReadFromUDPAddrPort(b)
			if err != nil {
				return
			}
			tx, err := stun.ParseBindingRequest(b[:n])
			if err == nil {
				_, _ = udp.WriteToUDPAddrPort(stun.Response(tx, addr), addr)
			}
		}
	}()
	// STUN only: no DERP listener exists, nor a public relay map.
	dm := &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{1: {
		RegionID: 1, RegionCode: "stun-only", RegionName: "No relay",
		Nodes: []*tailcfg.DERPNode{{Name: "stun", RegionID: 1, HostName: "127.0.0.1",
			IPv4: "127.0.0.1", IPv6: "none", STUNOnly: true,
			STUNPort: udp.LocalAddr().(*net.UDPAddr).Port, STUNTestIP: "127.0.0.1"}},
	}}}
	controller := &testcontrol.Server{DERPMap: dm, Logf: logger.Discard}
	cs := httptest.NewUnstartedServer(controller)
	controller.HTTPTestServer = cs
	cs.Start()
	defer cs.Close()
	makeNode := func(name string) *tsnet.Server {
		n := &tsnet.Server{Hostname: name, Dir: t.TempDir(), ControlURL: cs.URL,
			Logf: logger.Discard, UserLogf: logger.Discard}
		t.Cleanup(func() { _ = n.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := n.Up(ctx); err != nil {
			t.Fatalf("%s startup: %v", name, err)
		}
		return n
	}
	server := makeNode("mutti-direct-server")
	ln, err := server.Listen("tcp", ":8443")
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "mutti direct without relay")
	}), ReadHeaderTimeout: 5 * time.Second}
	defer hs.Close()
	go hs.Serve(ln)
	client := makeNode("mutti-direct-client")
	ip, _ := server.TailscaleIPs()
	hc := &http.Client{Transport: &http.Transport{DialContext: client.Dial}, Timeout: 10 * time.Second}
	var body []byte
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		res, err := hc.Get(fmt.Sprintf("http://%s:8443/", ip))
		if err == nil {
			body, err = io.ReadAll(res.Body)
			res.Body.Close()
		}
		if err == nil {
			lastErr = nil
			break
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatal(lastErr)
	}
	if string(body) != "mutti direct without relay" {
		t.Fatalf("unexpected body: %q", body)
	}
	lc, err := client.LocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := lc.Ping(ctx, netip.MustParseAddr(ip.String()), tailcfg.PingDisco)
	if err != nil {
		t.Fatal(err)
	}
	if p.Endpoint == "" || p.DERPRegionID != 0 || p.PeerRelay != "" {
		t.Fatalf("expected direct only: %+v", p)
	}
	t.Logf("PASS: real encrypted userspace HTTP over direct endpoint %s; no DERP server configured or running. Scope: one host, not WAN.", p.Endpoint)
}
