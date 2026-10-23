package playeraccess

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientAddressTrustBoundaries(t *testing.T) {
	cases := []struct {
		name, peer string
		headers    []string
		trusted    []string
		ip, source string
		public     bool
	}{
		{"direct", "8.8.8.8:42", []string{"1.1.1.1"}, nil, "8.8.8.8", "direct", true},
		{"untrusted private cannot spoof", "10.0.0.2:42", []string{"8.8.8.8"}, nil, "", "direct", false},
		{"trusted append ignores spoofed first", "10.0.0.2:42", []string{"1.1.1.1, 8.8.8.8"}, []string{"10.0.0.2/32"}, "8.8.8.8", "trusted_proxy", true},
		{"multiple headers multi hop", "10.0.0.2:42", []string{"1.1.1.1, 8.8.8.8", "10.0.0.3"}, []string{"10.0.0.2/32", "10.0.0.3/32"}, "8.8.8.8", "trusted_proxy", true},
		{"stop at untrusted private", "10.0.0.2:42", []string{"8.8.8.8, 192.168.0.2"}, []string{"10.0.0.2/32"}, "", "trusted_proxy", false},
		{"malformed cannot hunt public IP", "10.0.0.2:42", []string{"evil, 8.8.8.8"}, []string{"10.0.0.2/32"}, "", "invalid_forwarded_chain", false},
		{"missing header", "10.0.0.2:42", nil, []string{"10.0.0.2/32"}, "", "proxy_without_header", false},
		{"IPv6", "[2001:4860:4860::8888]:42", nil, nil, "2001:4860:4860::8888", "direct", true},
		{"mapped IPv4", "[::ffff:8.8.8.8]:42", nil, nil, "8.8.8.8", "direct", true},
		{"invalid peer", "invalid", nil, nil, "", "unknown", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.peer
			for _, h := range c.headers {
				r.Header.Add("X-Forwarded-For", h)
			}
			got := ClientAddress(r, c.trusted)
			if got.ClientIP != c.ip || got.Source != c.source || got.Public != c.public {
				t.Fatalf("%+v", got)
			}
		})
	}
}
func TestPublicIPAndProxyValidation(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "100.64.3.1", "169.254.1.1", "192.168.1.1", "172.16.1.1", "10.0.0.1", "192.0.2.4", "198.51.100.3", "203.0.113.3", "198.18.1.1", "224.1.1.1", "240.1.1.1", "0.0.0.0", "::1", "fe80::1", "fc00::1", "2001:db8::1", "3fff::1"} {
		if PublicIP(netip.MustParseAddr(s)) {
			t.Error(s)
		}
	}
	for _, s := range []string{"0.0.0.0/0", "::/0", "10.0.0.1/24", "invalid", "::ffff:192.168.0.1/128"} {
		if ValidateProxies([]string{s}) {
			t.Error(s)
		}
	}
}

func TestProxyAddressIsNeverMislabelledAsVisitorPublicIP(t *testing.T) {
	for _, header := range []string{"", "8.8.8.8", "bad, 1.1.1.1"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "8.8.8.8:8080"
		if header != "" {
			r.Header.Set("X-Forwarded-For", header)
		}
		out := ClientAddress(r, []string{"8.8.8.8/32"})
		if out.Public {
			t.Fatal("proxy IP presented as public visitor", out)
		}
	}
}
