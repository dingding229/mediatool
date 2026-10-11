package library

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupQuarantineNeverTouchesActiveOutputsAndRecoversUnlinkGap(t *testing.T) {
	root := t.TempDir()
	archive := ".media-tool-retired/owned/old.strm.retired"
	q := ".media-tool-retired/owned/.cleanup-approval.retired"
	raw := []byte("old generated URL\n")
	sum := sha256.Sum256(raw)
	os.MkdirAll(filepath.Dir(filepath.Join(root, archive)), 0750)
	os.WriteFile(filepath.Join(root, archive), raw, 0640)
	os.WriteFile(filepath.Join(root, "active.strm"), []byte("active"), 0640)
	if ok, err := quarantineArchive(root, archive, q, sum[:]); !ok || err != nil {
		t.Fatal(err)
	}
	if ok, err := quarantineArchive(root, archive, q, sum[:]); !ok || err != nil {
		t.Fatal("rename gap recovery", err)
	}
	if ok, err := removeQuarantined(root, archive, q, sum[:]); !ok || err != nil {
		t.Fatal(err)
	}
	if ok, err := removeQuarantined(root, archive, q, sum[:]); !ok || err != nil {
		t.Fatal("unlink gap recovery", err)
	}
	actual, _ := os.ReadFile(filepath.Join(root, "active.strm"))
	if string(actual) != "active" {
		t.Fatal("active source removed")
	}
}
func TestCleanupRefusesManualChangesCollisionAndSymlink(t *testing.T) {
	for _, kind := range []string{"modified", "symlink", "collision", "recreated"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			a := ".media-tool-retired/id/a.retired"
			q := ".media-tool-retired/id/.cleanup-id.retired"
			sum := sha256.Sum256([]byte("owned"))
			os.MkdirAll(filepath.Dir(filepath.Join(root, a)), 0750)
			os.WriteFile(filepath.Join(root, a), []byte("owned"), 0640)
			switch kind {
			case "modified":
				os.WriteFile(filepath.Join(root, a), []byte("manual"), 0640)
			case "symlink":
				os.Remove(filepath.Join(root, a))
				os.WriteFile(filepath.Join(root, "manual"), []byte("owned"), 0640)
				os.Symlink("../../manual", filepath.Join(root, a))
			case "collision":
				os.WriteFile(filepath.Join(root, q), []byte("manual"), 0640)
			case "recreated":
				quarantineArchive(root, a, q, sum[:])
				os.WriteFile(filepath.Join(root, a), []byte("manual"), 0640)
			}
			if ok, err := quarantineArchive(root, a, q, sum[:]); ok || !errors.Is(err, ErrArtifactConflict) {
				t.Fatal("unsafe quarantine", kind, err)
			}
			if _, err := os.Lstat(filepath.Join(root, a)); err != nil {
				t.Fatal("archive lost", kind)
			}
		})
	}
	root := t.TempDir()
	sum := sha256.Sum256([]byte("owned"))
	if ok, err := quarantineArchive(root, ".media-tool-retired/id/missing.retired", ".media-tool-retired/id/.cleanup-id.retired", sum[:]); ok || err == nil {
		t.Fatal("unknown disappearance counted as removal")
	}
	for _, a := range []string{"../outside", "active.strm", ".media-tool-retired/id/not-retired.strm"} {
		if ok, err := removeQuarantined(root, a, ".media-tool-retired/id/.cleanup-id.retired", sum[:]); ok || err == nil {
			t.Fatal("arbitrary output deletion")
		}
	}
}
