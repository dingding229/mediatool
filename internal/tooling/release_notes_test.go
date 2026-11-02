package tooling

import (
	"encoding/json"
	"media-tool/internal/modules/updatecheck"
	"media-tool/internal/platform/buildinfo"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestCurrentChangelogCanGenerateStableReleaseBeforePublication(t *testing.T) {
	root := project(t)
	text := source(t, "CHANGELOG.md")
	var first string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") && line != "## 未发布" {
			first = line
			break
		}
	}
	match := regexp.MustCompile(`^## \[([0-9]+\.[0-9]+\.[0-9]+)\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$`).FindStringSubmatch(first)
	if len(match) != 2 {
		t.Fatalf("stable changelog heading is not understood by release-notes: %q", first)
	}
	out, e := os.CreateTemp(t.TempDir(), "release-notes")
	if e != nil {
		t.Fatal(e)
	}
	old := os.Stdout
	os.Stdout = out
	defer func() { os.Stdout = old; _ = out.Close() }()
	digest := "sha256:" + strings.Repeat("a", 64)
	commit := strings.Repeat("b", 40)
	if e = releaseNotesCommand(root, []string{match[1], digest, commit, strconv.FormatInt(buildinfo.Schema, 10)}); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Clean(out.Name()))
	if e != nil {
		t.Fatal(e)
	}
	parts := strings.SplitN(string(raw), updatecheck.MetadataStart, 2)
	if len(parts) != 2 {
		t.Fatal("missing release metadata")
	}
	var meta updatecheck.Metadata
	if e = json.Unmarshal([]byte(strings.TrimSuffix(parts[1], "\n-->\n")), &meta); e != nil || meta.Validate() != nil || meta.Digest != digest || meta.Commit != commit || meta.Version != match[1] {
		t.Fatal(meta, e)
	}
	has(t, source(t, ".github/workflows/build.yml"), "Validate stable changelog before any publication")
}

func TestV0520FinalizerCannotRebuildOrMovePublishedImageOrTag(t *testing.T) {
	s := source(t, ".github/workflows/finalize-v0520.yml")
	has(t, s, "workflow_dispatch:", "actions: read", "2a612d39386ec3caa25590b568303e9dda73641c", "sha256:900c575f87bbd1dd8560ac985d83d79df885a4fa08930d4ad51622f208668c9b", "Require original gates and current full main verification", "Unexpected runtime change:", "Build and push linux/amd64 only", "Create stable GitHub Release", ".conclusion==\"success\"", "git rev-parse", "docker image inspect", "gh release create", "--verify-tag")
	for _, bad := range []string{"docker build ", "docker buildx build", "docker push ", "git push ", "git tag ", "rerun", "--force"} {
		if strings.Contains(s, bad) {
			t.Fatal("immutable finalizer contains forbidden operation", bad)
		}
	}
}
