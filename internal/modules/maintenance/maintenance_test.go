package maintenance

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"

	"github.com/jackc/pgx/v5/pgconn"
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

func TestRequireOneAffectedRejectsStaleMaintenanceCompletion(t *testing.T) {
	if err := requireOneAffected(pgconn.NewCommandTag("UPDATE 0")); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict for zero-row completion, got %v", err)
	}
	if err := requireOneAffected(pgconn.NewCommandTag("UPDATE 1")); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateCleanupOwnsEveryFailureUntilHandoff(t *testing.T) {
	for _, stage := range []string{"open clone", "copy metadata", "initialize SQL", "invalidate approvals", "restore outputs"} {
		t.Run(stage, func(t *testing.T) {
			failed := errors.New(stage)
			cleanupErr := errors.New("drop failed")
			committed := false
			calls := 0
			result := failed
			cleanupFailedCandidate(&committed, &result, func() error { calls++; return cleanupErr })
			if calls != 1 || !errors.Is(result, failed) || !errors.Is(result, cleanupErr) {
				t.Fatal("cleanup or original failure lost", calls, result)
			}
		})
	}
	committed := true
	var result error
	cleanupFailedCandidate(&committed, &result, func() error { t.Fatal("handed-off candidate was dropped"); return nil })
	if result != nil {
		t.Fatal(result)
	}
}
