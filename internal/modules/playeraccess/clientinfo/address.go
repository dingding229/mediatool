// Package clientinfo contains database-free request evidence rules.
package clientinfo

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Address is observed evidence, not authentication. Never substitute the
// server's egress address for an unavailable visitor address.
type Address struct {
	ClientIP     string
	ConnectionIP string
	Source       string
	Public       bool
}

func ValidateProxies(values []string) bool {
	if len(values) > 32 {
		return false
	}
	for _, v := range values {
		p, err := netip.ParsePrefix(v)
		if err != nil || p != p.Masked() || p.Bits() == 0 || p.Addr().Is4In6() {
			return false
		}
	}
	return true
}

var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/3"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

func PublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}
func ClientAddress(r *http.Request, proxies []string) Address {
	out := Address{ClientIP: "unknown", ConnectionIP: "unknown", Source: "unknown"}
	raw := r.RemoteAddr
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	peer, err := netip.ParseAddr(raw)
	if err != nil || peer.Zone() != "" {
		return out
	}
	peer = peer.Unmap()
	out.ClientIP, out.ConnectionIP, out.Source = peer.String(), peer.String(), "direct"
	trusted := func(a netip.Addr) bool {
		for _, v := range proxies {
			p, e := netip.ParsePrefix(v)
			if e == nil && p.Contains(a.Unmap()) {
				return true
			}
		}
		return false
	}
	if trusted(peer) {
		// A malformed chain is not permission to pick the first plausible public IP.
		raw = strings.Join(r.Header.Values("X-Forwarded-For"), ",")
		if raw == "" {
			out.Source = "proxy_without_header"
		} else if len(raw) > 4096 {
			out.Source = "invalid_forwarded_chain"
		} else {
			parts := strings.Split(raw, ",")
			chain := []netip.Addr{}
			valid := len(parts) <= 32
			for _, s := range parts {
				a, e := netip.ParseAddr(strings.TrimSpace(s))
				if e != nil || a.Zone() != "" {
					valid = false
					break
				}
				chain = append(chain, a.Unmap())
			}
			if !valid {
				out.Source = "invalid_forwarded_chain"
			} else {
				current := peer
				for i := len(chain) - 1; i >= 0 && trusted(current); i-- {
					current = chain[i]
				}
				out.ClientIP = current.String()
				out.Source = "trusted_proxy"
				if trusted(current) {
					out.Source = "trusted_chain_without_client"
				}
			}
		}
	}
	ip, err := netip.ParseAddr(out.ClientIP)
	out.Public = err == nil && (out.Source == "direct" || out.Source == "trusted_proxy") && PublicIP(ip)
	return out
}
