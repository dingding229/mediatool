package diagnosticpolicy

import (
	"errors"
	"media-tool/internal/modules/playback"
	"slices"
	"strings"
	"testing"
)

func TestPolicyUsesRuntimeOriginsWithoutSecrets(t *testing.T) {
	p := New("https://user:PRIVATE@app.example.test:8443/path?token=PRIVATE", "https://member.example.test:8092", "http://cloud.example.test:19798")
	for _, host := range []string{"localhost", "server", "clouddrive2", "app.example.test", "member.example.test", "cloud.example.test"} {
		if !slices.Contains(p.BlockedHosts, host) {
			t.Fatalf("missing blocked runtime host %s", host)
		}
		if _, err := p.Syntax("https://" + host + "/media"); !errors.Is(err, playback.ErrUnsafeTarget) {
			t.Fatal("accepted service target", host, err)
		}
	}
	if strings.Contains(strings.Join(p.BlockedHosts, ","), "PRIVATE") {
		t.Fatal("policy retained credentials")
	}
	if _, err := p.Syntax("https://cdn.example.test/media"); err != nil {
		t.Fatal("unrelated CDN denied", err)
	}
	for _, raw := range []string{"http://cdn.example.test/media", "https://user:pass@cdn.example.test/media", "https://127.0.0.1/media"} {
		if _, err := p.Syntax(raw); !errors.Is(err, playback.ErrUnsafeTarget) {
			t.Fatal("weakened original policy", raw)
		}
	}
}

func TestPolicyHasNoFixedDeploymentAndAcceptsIPv6Config(t *testing.T) {
	p := New("", "://bad", "https://[2001:db8::8]:8443")
	if !slices.Contains(p.BlockedHosts, "2001:db8::8") {
		t.Fatal(p.BlockedHosts)
	}
	if _, err := p.Syntax("https://[2001:db8::8]/media"); !errors.Is(err, playback.ErrUnsafeTarget) {
		t.Fatal(err)
	}
	other := New("https://other.example.test")
	if slices.Contains(other.BlockedHosts, "app.example.test") {
		t.Fatal("previous deployment leaked into policy")
	}
}
