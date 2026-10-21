package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutputRootNeverFollowsSymlinks(t *testing.T) {
	t.Chdir(t.TempDir())
	if e := os.MkdirAll("web/public", 0755); e != nil {
		t.Fatal(e)
	}
	if e := safeOutput("web/public/plugins"); e != nil {
		t.Fatal(e)
	}
	if e := safeOutput("../data/plugins"); e == nil {
		t.Fatal("escaped root")
	}
	other := t.TempDir()
	if e := os.Symlink(other, filepath.Join("web", "public", "plugins")); e != nil {
		t.Fatal(e)
	}
	if e := safeOutput("web/public/plugins"); e == nil {
		t.Fatal("symlink root accepted")
	}
}

func TestSourceContentDigestIgnoresReleaseLocationAndTestsButIncludesHandlersSDKAndESM(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"sdk", "plugins/owned/ui/dist"} {
		if e := os.MkdirAll(filepath.Join(root, dir), 0755); e != nil {
			t.Fatal(e)
		}
	}
	write := func(p, v string) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(root, p), []byte(v), 0644); e != nil {
			t.Fatal(e)
		}
	}
	write("sdk/client.go", "package sdk")
	write("plugins/owned/main.go", "package owned")
	write("plugins/owned/ui/dist/main.js", "export default 1")
	first, e := sourceDigest(root, "owned")
	if e != nil {
		t.Fatal(e)
	}
	write("plugins/owned/main_test.go", "test-only-change")
	write("plugins/owned/README.md", "release-note")
	if next, e := sourceDigest(root, "owned"); e != nil || next != first {
		t.Fatal("test/docs changed behavior hash", e)
	}
	for _, p := range []string{"plugins/owned/main.go", "sdk/client.go", "plugins/owned/ui/dist/main.js"} {
		before, e := sourceDigest(root, "owned")
		if e != nil {
			t.Fatal(e)
		}
		write(p, "behavior changed")
		after, e := sourceDigest(root, "owned")
		if e != nil || before == after {
			t.Fatal("behavior omitted", p, e)
		}
	}
}
