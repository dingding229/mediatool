package tooling

import (
	"strings"
	"testing"
)

func TestDevAndPRVerifyWithoutPublishing(t *testing.T) {
	w := source(t, ".github/workflows/build.yml")
	gate := "if: github.event_name != 'pull_request' && (github.ref == 'refs/heads/main' || startsWith(github.ref, 'refs/tags/v'))"
	has(t, w, "branches: [main, dev]", "tags: ['v*']", "pull_request:", "permissions:\n  contents: read", "  browser:\n    needs: verify", "    needs: [verify, browser]", gate)
	// Keep publication behind the existing exact allowlist, not an event-only test.
	parts := strings.SplitN(w, "  publish:", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[1], "\n    needs: [verify, browser]\n    "+gate+"\n") {
		t.Fatal("publication gate changed")
	}
	// Neither complete verification job may become main-only or tag-only.
	if strings.Contains(parts[0], "\n    if:") {
		t.Fatal("dev verification must not be skipped at job level")
	}
	for _, tt := range []struct {
		event, ref string
		want       bool
	}{
		{"push", "refs/heads/dev", false},
		{"pull_request", "refs/pull/1/merge", false},
		{"pull_request", "refs/heads/main", false},
		{"push", "refs/heads/main", true},
		{"push", "refs/tags/v0.52.1", true},
	} {
		publish := tt.event != "pull_request" && (tt.ref == "refs/heads/main" || strings.HasPrefix(tt.ref, "refs/tags/v"))
		if publish != tt.want {
			t.Fatal(tt)
		}
	}
}
