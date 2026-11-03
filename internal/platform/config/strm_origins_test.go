package config

import (
	"strings"
	"testing"
)

func TestExplicitSTRMResolverOrigins(t *testing.T) {
	env := validEnv()
	env["STRM_RESOLVER_ORIGINS"] = "http://192.0.2.10:8080, https://play.example.test,http://192.0.2.10:8080"
	c, err := FromLookup(lookup(env))
	if err != nil || len(c.STRMResolverOrigins) != 2 || c.STRMResolverOrigins[0] != "http://192.0.2.10:8080" {
		t.Fatal(c.STRMResolverOrigins, err)
	}
	for _, raw := range []string{"*", "http://*.example.test", "http://0.0.0.0", "http://[::]", "http://user:secret@example.test", "http://example.test/", "http://example.test/path", "http://example.test?", "http://example.test#fragment", "http://example.test:70000", "http://example.test,,https://other.test", "file:///tmp/a", "https://example.test,", strings.Repeat("https://example.test,", 16) + "https://other.test"} {
		env["STRM_RESOLVER_ORIGINS"] = raw
		if _, err := FromLookup(lookup(env)); err == nil {
			t.Errorf("accepted unsafe origins %q", raw)
		}
	}
}
