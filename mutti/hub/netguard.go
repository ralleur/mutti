// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

var errPublicTarget = errors.New("Nur Dienste auf diesem Gerät oder im privaten Heimnetz sind erlaubt.")

// privateIP accepts loopback, RFC 1918, carrier-internal Docker ranges, IPv6
// ULA and link-local. Everything else would be a cloud or Internet target.
func privateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// guardedClient never follows redirects and refuses to connect to public
// addresses, also after DNS resolution. No proxy environment is honoured.
func guardedClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: func(network, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || !privateIP(ip) {
			return errPublicTarget
		}
		return nil
	}}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{Proxy: nil, DialContext: dialer.DialContext, MaxIdleConnsPerHost: 8,
			IdleConnTimeout: 60 * time.Second, ResponseHeaderTimeout: 60 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect rejected") },
	}
}

// ValidateServiceURL normalises an owner-entered service origin. The address
// must be plain http(s) without credentials, query or path tricks and resolve
// to loopback or a private network only.
func ValidateServiceURL(ctx context.Context, raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("Bitte eine Adresse wie http://192.168.1.10:2283 eingeben.")
	}
	path := strings.TrimRight(u.Path, "/")
	if path != "" && path != "/api" {
		return "", errors.New("Bitte nur die Serveradresse ohne weiteren Pfad eingeben.")
	}
	host := u.Hostname()
	ips := []net.IP{net.ParseIP(host)}
	if ips[0] == nil {
		resolved, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil || len(resolved) == 0 {
			return "", errors.New("Der Dienstname ist im Heimnetz nicht auflösbar.")
		}
		ips = resolved
	}
	for _, ip := range ips {
		if !privateIP(ip) {
			return "", errPublicTarget
		}
	}
	return u.Scheme + "://" + u.Host, nil
}
