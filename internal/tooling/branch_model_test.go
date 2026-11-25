package tooling

import (
	"os/exec"
	"path/filepath"
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

func TestReleaseTagAncestryChecksFreshMainBeforePublication(t *testing.T) {
	w := source(t, ".github/workflows/build.yml")
	_, publish, ok := strings.Cut(w, "  publish:\n")
	const gate = "      - name: Require release tag in main history\n"
	before, tail, found := strings.Cut(publish, gate)
	if !ok || !found || !strings.HasPrefix(tail, "        if: startsWith(github.ref, 'refs/tags/v')\n") || strings.Contains(before, "secrets.") || strings.Contains(before, "docker/") {
		t.Fatal("ancestry gate missing or placed after publication credentials")
	}
	step, _, _ := strings.Cut(tail, "      - name: Validate Docker Hub configuration\n")
	_, script, found := strings.Cut(step, "        run: |\n")
	if !found {
		t.Fatal("missing ancestry gate script")
	}
	has(t, script, "git fetch --no-tags origin +refs/heads/main:refs/remotes/origin/main", `git merge-base --is-ancestor "$GITHUB_SHA" origin/main`)

	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	commit := func(dir, message string) string {
		git(dir, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", message)
		return git(dir, "rev-parse", "HEAD")
	}
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin")
	checkout := filepath.Join(dir, "checkout")
	git(dir, "init", "-b", "main", origin)
	base := commit(origin, "base")
	main := commit(origin, "main")
	git(origin, "switch", "-c", "candidate", base)
	candidate := commit(origin, "unmerged")
	git(origin, "switch", "main")
	git(dir, "clone", "--no-tags", origin, checkout)

	for _, tt := range []struct {
		sha  string
		want bool
	}{{main, true}, {base, true}, {candidate, false}} {
		// A stale checkout would allow the candidate if the real step skipped fetch.
		git(checkout, "update-ref", "refs/remotes/origin/main", candidate)
		cmd := exec.Command("bash", "-e", "-c", script)
		cmd.Dir = checkout
		t.Setenv("GITHUB_SHA", tt.sha)
		out, err := cmd.CombinedOutput()
		if (err == nil) != tt.want || git(checkout, "rev-parse", "origin/main") != main {
			t.Fatalf("ancestry decision for %s: %v: %s", tt.sha, err, out)
		}
	}
}
