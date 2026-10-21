package accounts

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSessionUserAgentBound(t *testing.T) {
	for _, input := range []string{strings.Repeat("a", 513), strings.Repeat("中", 180), "ok", "bad\xff"} {
		v := sessionMetadata([]SessionMetadata{{UserAgent: input}}).UserAgent
		if len(v) > 512 || !utf8.ValidString(v) {
			t.Fatalf("invalid bounded UA: %q", v)
		}
	}
}
