package clientinfo

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// RequestOrigin trusts forwarding headers only from a configured immediate peer.
// A comma/multi-value authority is not a license to select an arbitrary host.
func RequestOrigin(r *http.Request, proxies []string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	peer := r.RemoteAddr
	if h, _, e := net.SplitHostPort(peer); e == nil {
		peer = h
	}
	ip, e := netip.ParseAddr(peer)
	trusted := false
	if e == nil {
		for _, raw := range proxies {
			if p, e := netip.ParsePrefix(raw); e == nil && p.Contains(ip.Unmap()) {
				trusted = true
				break
			}
		}
	}
	if trusted && len(r.Header.Values("X-Forwarded-Host")) == 1 && len(r.Header.Values("X-Forwarded-Proto")) == 1 {
		h, p := r.Header.Get("X-Forwarded-Host"), strings.ToLower(r.Header.Get("X-Forwarded-Proto"))
		if !strings.ContainsAny(h, ",/\\?#@ \t\r\n") && h != "" && (p == "http" || p == "https") {
			host, scheme = h, p
		}
	}
	return scheme + "://" + host
}
