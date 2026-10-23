package tooling

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGitTagAndDevelopmentBuildMetadata(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = root
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatal(e, string(b))
		}
	}
	git("init", "-q")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@invalid", "commit", "--allow-empty", "-qm", "fixture")
	git("tag", "v0.28.0")
	at := time.Date(2026, 10, 23, 1, 2, 3, 0, time.UTC)
	release, e := buildMetadata(root, "refs/tags/v0.28.0", at)
	if e != nil || release.Version != "0.28.0" || !strings.Contains(release.Tags, "dingding229/mediatool:v0.28.0") || !strings.Contains(release.Tags, ":sha-") || release.BuildTime != "2026-10-23T01:02:03Z" {
		t.Fatal(release, e)
	}
	dev, e := buildMetadata(root, "refs/heads/main", at)
	if e != nil || dev.Version != "0.28.0-dev+"+release.Commit[:12] || !strings.Contains(dev.Tags, ":latest") || !strings.Contains(dev.LDFlags, "buildinfo.Commit="+release.Commit) {
		t.Fatal(dev, e)
	}
	for _, ref := range []string{"refs/tags/v01.2.3", "refs/tags/v0.28.0;echo", "refs/tags/v0.28.1"} {
		if _, e = buildMetadata(root, ref, at); e == nil {
			t.Fatal(ref)
		}
	}
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@invalid", "commit", "--allow-empty", "-qm", "second")
	if _, e = buildMetadata(root, "refs/tags/v0.28.0", at); e == nil {
		t.Fatal("foreign tag commit accepted")
	}
}
func TestReleaseBodyUsesVersionSectionAndWorkflowHasMetadata(t *testing.T) {
	root := project(t)
	w := source(t, ".github/workflows/build.yml")
	for _, v := range []string{"fetch-depth: 0", "build-meta github", "SOURCE_VERSION=", "SOURCE_BUILD_TIME=", "contents: write", "gh release create", "--verify-tag"} {
		has(t, w, v)
	}
	docker := source(t, "deploy/Dockerfile")
	for _, v := range []string{"buildinfo.Version=${SOURCE_VERSION}", "buildinfo.Commit=${SOURCE_REVISION}", "buildinfo.BuildTime=${SOURCE_BUILD_TIME}"} {
		has(t, docker, v)
	}
	if _, e := os.Stat(filepath.Join(root, "CHANGELOG.md")); e != nil {
		t.Fatal(e)
	}
}
