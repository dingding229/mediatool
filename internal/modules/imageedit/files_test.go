package imageedit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestImageRootRejectsSymlinkedParentBeforeCreatingFiles(t *testing.T) {
	base, outside := t.TempDir(), t.TempDir()
	parent := filepath.Join(base, "native-media")
	if e := os.Symlink(outside, parent); e != nil {
		t.Fatal(e)
	}
	repo := Repository{Root: filepath.Join(parent, "images")}
	if _, e := repo.openRoot(); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if e := repo.ensureRoot(); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(outside, "images")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("created outside image directory", e)
	}
}
