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
