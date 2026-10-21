package tooling

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCIFullRaceOnceRetainsEveryNativeGate(t *testing.T) {
	w := source(t, ".github/workflows/build.yml")
	verify := strings.Split(strings.Split(w, "  verify:")[1], "  publish:")[0]
	if strings.Count(verify, "bash scripts/test-go.sh") != 1 {
		t.Fatal("duplicate full/focused race pipeline")
	}
	has(t, verify, "bash scripts/test-plugin-kernel.sh", "go test -race -count=3 -v ./internal/modules/mediaprobe/...", "go vet ./...", "errcheck", "bash scripts/test-e2e.sh", "MEDIA_TOOL_TEST_PLUGIN_UID", "CGO_ENABLED=0 go build", "$GITHUB_SHA")
	for _, p := range []string{"scripts/test-go.sh", "scripts/test-plugin-kernel.sh", "scripts/test-e2e.sh"} {
		has(t, source(t, p), "copy_test_app")
	}
}
func TestSharedCIBinaryRejectsAnotherRevision(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "app")
	os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0700)
	os.WriteFile(binary+".sha", []byte("same-revision\n"), 0600)
	for _, revision := range []string{"same-revision", "wrong-revision"} {
		out := filepath.Join(dir, revision)
		cmd := exec.Command("bash", "-c", `source scripts/test-common.sh;copy_test_app "$1"`, "bash", out)
		cmd.Dir = project(t)
		cmd.Env = append(os.Environ(), "GITHUB_ACTIONS=true", "GITHUB_SHA="+revision, "MEDIA_TOOL_STATIC_TEST_BINARY="+binary)
		raw, err := cmd.CombinedOutput()
		if (err == nil) != (revision == "same-revision") {
			t.Fatal(revision, err, string(raw))
		}
	}
}
