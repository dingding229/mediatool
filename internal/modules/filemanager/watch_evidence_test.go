package filemanager

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchPrepareRejectsChangedMtimeAndSymlink(t *testing.T) {
	ctx, e, root, b := fixture(t)
	name := "Synthetic.2026.mkv"
	full := filepath.Join(root.Path, name)
	if err := os.WriteFile(full, []byte("tiny"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := b.Find(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	wrong := f.Modified - 1
	in := Input{ConnectionID: root.ID, Operation: "organize", Source: name, Destination: "organized", ExpectedFileID: f.ID, ExpectedSize: &f.Size, ExpectedModified: &wrong}
	if _, err = e.Preview(ctx, in); !errors.Is(err, ErrChanged) {
		t.Fatal("mtime fence missing", err)
	}
	in.ExpectedModified = &f.Modified
	p, err := e.Preview(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	modified := time.Unix(0, f.Modified).Add(time.Second)
	if err = os.Chtimes(full, modified, modified); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Execute(ctx, p.ID, p.Digest, true); !errors.Is(err, ErrChanged) {
		t.Fatal("post-preview mutation passed", err)
	}
	if err = os.Symlink(t.TempDir(), filepath.Join(root.Path, "escape")); err != nil {
		t.Fatal(err)
	}
	in.Source = "escape/secret.mkv"
	in.ExpectedModified = nil
	if _, err = e.Preview(ctx, in); err == nil {
		t.Fatal("symlink passed")
	}
}
