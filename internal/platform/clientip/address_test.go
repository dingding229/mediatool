package clientip

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestPublicOnlyAndProxyBoundary(t *testing.T) {
	for _, c := range []struct {
		name, peer, xff, real, want string
		trusted                     []string
	}{
		{"direct public", "8.8.8.8:3", "1.1.1.1", "", "8.8.8.8", nil},
		{"private forged", "192.168.1.2:3", "8.8.8.8", "1.1.1.1", "", nil},
		{"trusted peer", "192.168.1.2:3", "8.8.8.8", "", "8.8.8.8", []string{"192.168.1.2/32"}},
		{"rightmost untrusted", "192.168.1.2:3", "9.9.9.9, 1.1.1.1", "", "1.1.1.1", []string{"192.168.1.2/32"}},
		{"trusted multi hop", "192.168.1.2:3", "8.8.8.8, 192.168.1.3", "", "8.8.8.8", []string{"192.168.1.2/32", "192.168.1.3/32"}},
		{"do not cross private untrusted boundary", "192.168.1.2:3", "8.8.8.8, 10.0.0.8", "", "", []string{"192.168.1.2/32"}},
		{"real fallback", "192.168.1.2:3", "", "1.1.1.1", "1.1.1.1", []string{"192.168.1.2/32"}},
		{"xff precedes real", "192.168.1.2:3", "8.8.8.8", "1.1.1.1", "8.8.8.8", []string{"192.168.1.2/32"}},
		{"malformed chain rejected", "192.168.1.2:3", "bad,8.8.8.8", "1.1.1.1", "", []string{"192.168.1.2/32"}},
		{"ipv6", "[2606:4700:4700::1111]:3", "", "", "2606:4700:4700::1111", nil},
		{"mapped", "[::ffff:8.8.8.8]:3", "", "", "8.8.8.8", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.peer
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			if c.real != "" {
				r.Header.Set("X-Real-IP", c.real)
			}
			v := ClientAddress(r, c.trusted)
			if v.ClientIP != c.want || v.Public != (c.want != "") {
				t.Fatalf("got %+v", v)
			}
			if v.ConnectionIP != "" && !PublicString(v.ConnectionIP) {
				t.Fatal("private peer retained")
			}
		})
	}
}
func TestAddressFamiliesAndReservedRanges(t *testing.T) {
	for _, s := range []string{"0.0.0.0", "10.1.1.1", "100.64.0.1", "100.127.255.255", "127.0.0.1", "169.254.1.1", "172.16.1.1", "192.168.1.1", "192.0.2.1", "198.18.1.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.1.1.1", "255.255.255.255", "::", "::1", "::ffff:10.0.0.1", "fc00::1", "fd00::1", "fe80::1", "ff02::1", "2001:db8::1", "2001:2::1", "2002::1", "3fff::1", "64:ff9b::808:808", "fe80::1%eth0"} {
		if PublicIP(netip.MustParseAddr(s)) || Sanitize(s) != "" {
			t.Fatal("nonpublic accepted", s)
		}
	}
	for _, s := range []string{"1.1.1.1", "8.8.8.8", "100.128.0.1", "2606:4700:4700::1111", "2001:4860:4860::8888", "::ffff:8.8.8.8"} {
		if !PublicString(s) || Sanitize(s) == "" {
			t.Fatal("public rejected", s)
		}
	}
}

// Security admission needs reserved transport identities; every existing public-only facade stays unchanged.
func TestAdmissionIdentityDoesNotChangeStatisticsOrHistoricalCallers(t *testing.T) {
	for _, c := range []struct {
		peer, xff       string
		proxies         []string
		security, stats string
	}{
		{"203.0.113.9:123", "8.8.8.8", nil, "203.0.113.9", ""},
		{"[2001:db8::9]:123", "", nil, "2001:db8::9", ""},
		{"10.0.0.1:123", "203.0.113.9", []string{"10.0.0.0/8"}, "203.0.113.9", ""},
		{"10.0.0.1:123", "8.8.8.8", []string{"10.0.0.0/8"}, "8.8.8.8", "8.8.8.8"},
		{"8.8.8.8:123", "1.1.1.1", nil, "8.8.8.8", "8.8.8.8"},
		{"10.0.0.1:123", "bad,8.8.8.8", []string{"10.0.0.0/8"}, "", ""},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.peer
		r.Header.Set("X-Forwarded-For", c.xff)
		raw := AdmissionAddress(r, c.proxies)
		got := ClientAddress(r, c.proxies)
		if raw.ClientIP != c.security || got.ClientIP != c.stats {
			t.Fatal(c, raw, got)
		}
		if got.ConnectionIP != "" && !PublicString(got.ConnectionIP) {
			t.Fatal("private peer entered stats")
		}
		if Observed(WithAddress(r, got)) != got {
			t.Fatal("context facade changed")
		}
	}
}
