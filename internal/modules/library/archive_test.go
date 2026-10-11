package library

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestArchivedOutputIsRetainedAndCrashRecoveryIsIdempotent(t *testing.T) {
	root := t.TempDir()
	source := "Movies/owned/file.strm"
	saved := ".media-tool-retired/owned-unique/Movies/owned/file.strm.retired"
	content := []byte("fixed-file-grant\n")
	sum := sha256.Sum256(content)
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, source)), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, source), content, 0640); err != nil {
		t.Fatal(err)
	}
	if err := archiveFile(root, source, saved, sum[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, source)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old playable output remains")
	}
	actual, err := os.ReadFile(filepath.Join(root, saved))
	if err != nil || string(actual) != string(content) {
		t.Fatal("archive lost original bytes", err)
	}
	if err := archiveFile(root, source, saved, sum[:]); err != nil {
		t.Fatal("rename-before-commit recovery", err)
	}
	if err := os.WriteFile(filepath.Join(root, source), []byte("manual replacement"), 0640); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(archiveFile(root, source, saved, sum[:]), ErrArtifactConflict) {
		t.Fatal("recreated original was removed")
	}
	raw, _ := os.ReadFile(filepath.Join(root, source))
	if string(raw) != "manual replacement" {
		t.Fatal("manual replacement overwritten")
	}
}

func TestArchiveRefusesUnknownContentSymlinksAndForeignDestination(t *testing.T) {
	for _, kind := range []string{"source_changed", "source_symlink", "archive_changed", "archive_symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			source := "owned.strm"
			saved := ".media-tool-retired/unique/owned.strm.retired"
			original := []byte("owned")
			sum := sha256.Sum256(original)
			os.WriteFile(filepath.Join(root, source), original, 0640)
			os.MkdirAll(filepath.Dir(filepath.Join(root, saved)), 0750)
			switch kind {
			case "source_changed":
				os.WriteFile(filepath.Join(root, source), []byte("manual"), 0640)
			case "source_symlink":
				os.Remove(filepath.Join(root, source))
				os.WriteFile(filepath.Join(root, "elsewhere"), []byte("manual"), 0640)
				os.Symlink("elsewhere", filepath.Join(root, source))
			case "archive_changed":
				os.WriteFile(filepath.Join(root, saved), []byte("other owner"), 0640)
			case "archive_symlink":
				os.WriteFile(filepath.Join(root, "elsewhere"), original, 0640)
				os.Symlink("../../elsewhere", filepath.Join(root, saved))
			}
			if !errors.Is(archiveFile(root, source, saved, sum[:]), ErrArtifactConflict) {
				t.Fatal("unsafe archive accepted")
			}
			if _, err := os.Lstat(filepath.Join(root, source)); err != nil {
				t.Fatal("original disappeared", err)
			}
		})
	}
	root := t.TempDir()
	sum := sha256.Sum256([]byte("owned"))
	for _, pair := range [][2]string{{"../outside", ".media-tool-retired/a/one.retired"}, {"owned", "../outside"}, {"owned", ".media-tool-retired/a/owned.strm"}} {
		if !errors.Is(archiveFile(root, pair[0], pair[1], sum[:]), ErrArtifactConflict) {
			t.Fatal("unsafe destination accepted")
		}
	}
}

func TestAtomicRenameNeverReplacesConcurrentArchiveDestination(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "source"), []byte("owned"), 0640)
	os.WriteFile(filepath.Join(root, "destination"), []byte("manual"), 0640)
	directory, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if err = renameNoReplace(directory, "source", directory, "destination"); !errors.Is(err, os.ErrExist) {
		t.Fatal("destination collision did not refuse", err)
	}
	a, _ := os.ReadFile(filepath.Join(root, "source"))
	b, _ := os.ReadFile(filepath.Join(root, "destination"))
	if string(a) != "owned" || string(b) != "manual" {
		t.Fatal("no-replace contract lost bytes")
	}
}
