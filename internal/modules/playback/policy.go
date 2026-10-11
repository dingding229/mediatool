package playback

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
)

var ErrUnsafeTarget = errors.New("unsafe playback target")

type LookupFunc func(context.Context, string) ([]net.IPAddr, error)
type Policy struct {
	AllowedHosts []string
	BlockedHosts []string
	AllowHTTP    bool
	Lookup       LookupFunc
}

func (p Policy) Syntax(raw string) (*url.URL, error) {
	if strings.ContainsAny(raw, "\r\n\x00") {
		return nil, ErrUnsafeTarget
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && !(p.AllowHTTP && u.Scheme == "http")) {
		return nil, ErrUnsafeTarget
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	for _, blocked := range p.BlockedHosts {
		if host == strings.TrimSuffix(strings.ToLower(blocked), ".") {
			return nil, ErrUnsafeTarget
		}
	}
	if host == "localhost" || !strings.Contains(host, ".") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, ErrUnsafeTarget
	}
	if len(p.AllowedHosts) > 0 {
		found := false
		for _, allowed := range p.AllowedHosts {
			allowed = strings.ToLower(strings.TrimSpace(allowed))
			if host == allowed || (strings.HasPrefix(allowed, "*.") && strings.HasSuffix(host, allowed[1:]) && host != allowed[2:]) {
				found = true
				break
			}
		}
		if !found {
			return nil, ErrUnsafeTarget
		}
	}
	if ip := net.ParseIP(host); ip != nil && !publicIP(ip) {
		return nil, ErrUnsafeTarget
	}
	return u, nil
}
func (p Policy) Check(ctx context.Context, raw string) error {
	u, err := p.Syntax(raw)
	if err != nil {
		return err
	}
	_, err = p.addresses(ctx, u.Hostname())
	return err
}
func (p Policy) addresses(ctx context.Context, host string) ([]net.IPAddr, error) {
	lookup := p.Lookup
	if lookup == nil {
		lookup = net.DefaultResolver.LookupIPAddr
	}
	ips, err := lookup(ctx, host)
	if err != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil || len(ips) == 0 {
		return nil, ErrUnsafeTarget
	}
	// Reject mixed public/private answers too; accepting just one safe answer can
	// hide rebinding and address-family surprises.
	for _, ip := range ips {
		if !publicIP(ip.IP) {
			return nil, ErrUnsafeTarget
		}
	}
	return ips, nil
}
func (p Policy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrUnsafeTarget
	}
	ips, err := p.addresses(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{}
	var last error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}
func publicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return false
	}
	// CGNAT (cloud metadata may live here), benchmark and reserved IPv4 blocks.
	for _, cidr := range []string{"100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4", "192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "2001::/32"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}
