// SPDX-License-Identifier: MPL-2.0
package connect

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/pion/datachannel"
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

func privateHost(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()))
}

// Data channels preserve messages; TLS/yamux need a byte stream. Never discard
// the unread portion of a message, and keep writes below SCTP's message limit.
type channelConn struct {
	DataChannel interface {
		io.ReadWriteCloser
		SetReadDeadline(time.Time) error
		SetWriteDeadline(time.Time) error
	}
	pending         []byte
	readMu, writeMu sync.Mutex
	deadlineMu      sync.Mutex
	closeOnce       sync.Once
	closed          atomic.Bool
	closeErr        error
}

func (c *channelConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if c.closed.Load() {
		return 0, net.ErrClosed
	}
	if len(c.pending) == 0 {
		b := make([]byte, 65536)
		n, e := c.DataChannel.Read(b)
		if e != nil {
			return 0, e
		}
		c.pending = b[:n]
	}
	n := copy(p, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}
func (c *channelConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.closed.Load() {
		return 0, net.ErrClosed
	}
	total := 0
	for len(p) > 0 {
		n := len(p)
		if n > 16384 {
			n = 16384
		}
		w, e := c.DataChannel.Write(p[:n])
		total += w
		if e != nil {
			return total, e
		}
		p = p[w:]
		if w == 0 {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}
func (c *channelConn) LocalAddr() net.Addr  { return channelAddr("local") }
func (c *channelConn) RemoteAddr() net.Addr { return channelAddr("peer") }
func (c *channelConn) Close() error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		// SCTP Close initiates a graceful stream reset. A vanished peer cannot
		// acknowledge it, so explicitly wake TLS/yamux's blocked read and write.
		_ = c.SetDeadline(time.Now())
		c.closeErr = c.DataChannel.Close()
	})
	return c.closeErr
}
func (c *channelConn) SetReadDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	if c.closed.Load() {
		t = time.Now()
	}
	return c.DataChannel.SetReadDeadline(t)
}
func (c *channelConn) SetWriteDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	if c.closed.Load() {
		t = time.Now()
	}
	return c.DataChannel.SetWriteDeadline(t)
}
func (c *channelConn) SetDeadline(t time.Time) error {
	if e := c.SetReadDeadline(t); e != nil {
		return e
	}
	return c.SetWriteDeadline(t)
}

type channelAddr string

func (a channelAddr) Network() string { return "mutti-direct" }
func (a channelAddr) String() string  { return string(a) }

type Peer struct {
	PC     *webrtc.PeerConnection
	ready  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newPeer(stun string) (*Peer, error) {
	s := webrtc.SettingEngine{}
	s.DetachDataChannels()
	s.SetIncludeLoopbackCandidate(true)
	if interfaces := os.Getenv("MUTTI_ICE_INTERFACES"); interfaces != "" {
		allowed := strings.Split(interfaces, ",")
		s.SetInterfaceFilter(func(name string) bool {
			for _, a := range allowed {
				if a == name {
					return true
				}
			}
			return false
		})
	}
	if minimum := os.Getenv("MUTTI_UDP_MIN"); minimum != "" {
		lo, e1 := strconv.ParseUint(minimum, 10, 16)
		hi, e2 := strconv.ParseUint(os.Getenv("MUTTI_UDP_MAX"), 10, 16)
		if e1 != nil || e2 != nil || lo == 0 || hi < lo {
			return nil, errors.New("invalid UDP range")
		}
		if e := s.SetEphemeralUDPPortRange(uint16(lo), uint16(hi)); e != nil {
			return nil, e
		}
	}
	s.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	s.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6})
	s.SetICETimeouts(5*time.Second, 10*time.Second, 2*time.Second)
	config := webrtc.Configuration{}
	if stun != "" {
		if !strings.HasPrefix(stun, "stun:") {
			return nil, errors.New("relay forbidden")
		}
		config.ICEServers = []webrtc.ICEServer{{URLs: []string{stun}}}
	}
	pc, e := webrtc.NewAPI(webrtc.WithSettingEngine(s)).NewPeerConnection(config)
	if e != nil {
		return nil, e
	}
	p := &Peer{PC: pc, ready: make(chan net.Conn, 1), closed: make(chan struct{})}
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "mutti-v1" {
			_ = dc.Close()
			return
		}
		p.attach(dc)
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			p.once.Do(func() { close(p.closed) })
		}
	})
	return p, nil
}
func (p *Peer) attach(dc *webrtc.DataChannel) {
	dc.OnOpen(func() {
		d, e := dc.Detach()
		if e != nil {
			p.Close()
			return
		}
		raw, ok := d.(*datachannel.DataChannel)
		if !ok {
			_ = d.Close()
			p.Close()
			return
		}
		c := &channelConn{DataChannel: raw}
		select {
		case p.ready <- c:
		case <-p.closed:
			_ = c.Close()
		default:
			_ = c.Close()
		}
	})
}
func (p *Peer) Close() { p.once.Do(func() { close(p.closed) }); _ = p.PC.Close() }
func (p *Peer) local(ctx context.Context, offer bool) (webrtc.SessionDescription, error) {
	var s webrtc.SessionDescription
	var e error
	if offer {
		dc, err := p.PC.CreateDataChannel("mutti-v1", nil)
		if err != nil {
			return s, err
		}
		p.attach(dc)
		s, e = p.PC.CreateOffer(nil)
	} else {
		s, e = p.PC.CreateAnswer(nil)
	}
	if e != nil {
		return s, e
	}
	done := webrtc.GatheringCompletePromise(p.PC)
	if e = p.PC.SetLocalDescription(s); e != nil {
		return s, e
	}
	select {
	case <-ctx.Done():
		return s, ctx.Err()
	case <-done:
		return *p.PC.LocalDescription(), nil
	}
}
func (p *Peer) remote(s webrtc.SessionDescription) error {
	// Both peers reject relay candidates before Pion can use them. No TURN server
	// or alternate transport exists in this component.
	if len(s.SDP) > 60000 || strings.Contains(s.SDP, " typ relay") {
		return errors.New("relay/oversized SDP rejected")
	}
	return p.PC.SetRemoteDescription(s)
}
func (p *Peer) secure(ctx context.Context, identity Identity, serverPin string, server bool) (*yamux.Session, string, error) {
	var raw net.Conn
	select {
	case raw = <-p.ready:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	case <-p.closed:
		return nil, "", errors.New("Direkte Verbindung nicht möglich. Prüfe das Netz und versuche es erneut.")
	}
	cert, e := identity.TLS()
	if e != nil {
		return nil, "", e
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, SessionTicketsDisabled: true}
	var conn *tls.Conn
	if server {
		cfg.ClientAuth = tls.RequireAnyClientCert
		conn = tls.Server(raw, cfg)
	} else {
		// Self-signed household identity: trust the QR's public-key fingerprint,
		// not a certificate or DTLS fingerprint supplied by the rendezvous service.
		cfg.InsecureSkipVerify = true
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) != 1 || certificatePin(cs.PeerCertificates[0]) != serverPin {
				return errors.New("Mutti-Identität stimmt nicht mit der Kopplung überein.")
			}
			return nil
		}
		conn = tls.Client(raw, cfg)
	}
	if e = conn.HandshakeContext(ctx); e != nil {
		_ = conn.Close()
		return nil, "", e
	}
	cs := conn.ConnectionState()
	if len(cs.PeerCertificates) != 1 {
		_ = conn.Close()
		return nil, "", errors.New("invalid identity")
	}
	peerCert := cs.PeerCertificates[0]
	if _, ok := peerCert.PublicKey.(*ecdsa.PublicKey); !ok {
		_ = conn.Close()
		return nil, "", errors.New("unsupported identity")
	}
	// Possession is proved by TLS CertificateVerify. Registration policy follows
	// on an isolated pairing endpoint, never on the Jellyfin proxy.
	y := yamux.DefaultConfig()
	y.LogOutput = io.Discard
	y.AcceptBacklog = 32
	y.StreamOpenTimeout = 10 * time.Second
	y.KeepAliveInterval = 5 * time.Second
	y.StreamCloseTimeout = 10 * time.Second
	y.MaxStreamWindowSize = 4 * 1024 * 1024
	var session *yamux.Session
	if server {
		session, e = yamux.Server(conn, y)
	} else {
		session, e = yamux.Client(conn, y)
	}
	return session, certificatePin(peerCert), e
}
