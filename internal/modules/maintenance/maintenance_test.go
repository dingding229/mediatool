package maintenance

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRelativePathsAndPolicy(t *testing.T) {
	for _, p := range []string{"", ".", "../a", "/a", "a/../b", "a\\b", "a\nb"} {
		if safeRelative(p) {
			t.Fatal(p)
		}
	}
	if !safeRelative("电影/影片 (2026)/Film.strm") {
		t.Fatal("unicode rejected")
	}
	p := Defaults()
	if p.Validate() != nil {
		t.Fatal(p)
	}
	p.Consecutive = 0
	if p.Validate() == nil {
		t.Fatal("bad policy accepted")
	}
	e := Engine{Root: t.TempDir()}
	if _, err := e.dir("../../etc"); err == nil {
		t.Fatal("unsafe UUID accepted")
	}
}
func TestRestoreOutputsNeverOverwritesAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	raw := []byte("owned fixed URL\n")
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	w.WriteHeader(&tar.Header{Name: "Movies/Owned.strm", Mode: 0600, Size: int64(len(raw))})
	w.Write(raw)
	w.Close()
	file := filepath.Join(t.TempDir(), "outputs.tar")
	os.WriteFile(file, archive.Bytes(), 0600)
	e := Engine{OutputRoot: root}
	proof := []FileProof{{Path: "Movies/Owned.strm", Bytes: int64(len(raw)), Digest: hash(raw)}}
	if err := e.restoreOutputs(context.Background(), file, proof); err != nil {
		t.Fatal(err)
	}
	if err := e.restoreOutputs(context.Background(), file, proof); err != nil {
		t.Fatal("same original should remain untouched", err)
	}
	os.WriteFile(filepath.Join(root, "Movies/Owned.strm"), []byte("manual"), 0600)
	if err := e.restoreOutputs(context.Background(), file, proof); err == nil {
		t.Fatal("manual file overwritten")
	}
	current, _ := os.ReadFile(filepath.Join(root, "Movies/Owned.strm"))
	if string(current) != "manual" {
		t.Fatal("manual content changed")
	}
	os.Remove(filepath.Join(root, "Movies/Owned.strm"))
	os.Remove(filepath.Join(root, "Movies"))
	os.Symlink(t.TempDir(), filepath.Join(root, "Movies"))
	if err := e.restoreOutputs(context.Background(), file, proof); err == nil {
		t.Fatal("symlink parent allowed")
	}
}
