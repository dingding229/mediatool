package maintenance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"media-tool/internal/modules/jobs"
)

func TestExpiredBackupDeletionChecksBothDirectoriesWithoutRemovingFiles(t *testing.T) {
	for _, tc := range []struct {
		name                string
		original, tombstone bool
		state               string
	}{
		{"both absent", false, false, "deleted"},
		{"original remains", true, false, "failed"},
		{"only quarantine remains", false, true, "failed"},
		{"both remain", true, true, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Engine{Root: t.TempDir()}
			id := jobs.UUID()
			dir, err := e.dir(id)
			if err != nil {
				t.Fatal(err)
			}
			for path, exists := range map[string]bool{dir: tc.original, dir + ".deleting": tc.tombstone} {
				if !exists {
					continue
				}
				if err = os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(path, "manifest.json"), []byte("retain on recovery"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
			state, message, err := e.backupDeletionRecoveryState(id, now.Add(-11*time.Minute), now, os.Lstat)
			if err != nil || state != tc.state {
				t.Fatalf("got %q, %q, %v", state, message, err)
			}
			if state == "failed" {
				want := "可重新删除"
				if tc.original && tc.tombstone {
					want = "需人工核对"
				}
				if !strings.Contains(message, want) {
					t.Fatal("missing actionable recovery message", message)
				}
			}
			for path, exists := range map[string]bool{dir: tc.original, dir + ".deleting": tc.tombstone} {
				if exists {
					raw, readErr := os.ReadFile(filepath.Join(path, "manifest.json"))
					if readErr != nil || string(raw) != "retain on recovery" {
						t.Fatal("recovery changed backup files", path, readErr)
					}
				} else if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
					t.Fatal("recovery unexpectedly created a directory", path, statErr)
				}
			}
		})
	}
}

func TestBackupDeletionRecoveryUsesDeleteStartRatherThanBackupAge(t *testing.T) {
	e := Engine{Root: t.TempDir()}
	id := jobs.UUID()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	for _, age := range []time.Duration{-time.Minute, 0, 9 * time.Minute, deletionRecoveryAfter - time.Nanosecond} {
		state, message, err := e.backupDeletionRecoveryState(id, now.Add(-age), now, func(string) (os.FileInfo, error) {
			t.Fatal("active deletion was inspected")
			return nil, nil
		})
		if state != "" || message != "" || err != nil {
			t.Fatal("active deletion was recovered", age, state, message, err)
		}
	}
	state, _, err := e.backupDeletionRecoveryState(id, now.Add(-deletionRecoveryAfter), now, os.Lstat)
	if err != nil || state != "deleted" {
		t.Fatal("ten-minute recovery boundary ignored", state, err)
	}
}

func TestBackupDeletionRecoveryDoesNotTreatInspectionErrorsAsAbsent(t *testing.T) {
	e := Engine{Root: t.TempDir()}
	id := jobs.UUID()
	dir, err := e.dir(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, suffix := range []string{"", ".deleting"} {
		for _, cause := range []error{os.ErrPermission, syscall.EIO} {
			failedPath := dir + suffix
			lstat := func(path string) (os.FileInfo, error) {
				if path == failedPath {
					return nil, &os.PathError{Op: "lstat", Path: path, Err: cause}
				}
				return os.Lstat(path)
			}
			_, inspectErr := inspectBackupDeletion(dir, lstat)
			if !errors.Is(inspectErr, cause) {
				t.Fatal("inspection error discarded", suffix, inspectErr)
			}
			state, message, recoveryErr := e.backupDeletionRecoveryState(id, now.Add(-11*time.Minute), now, lstat)
			if state != "failed" || !errors.Is(recoveryErr, cause) || !strings.Contains(message, "重新删除") {
				t.Fatal("inaccessible path misreported as deleted", suffix, state, message, recoveryErr)
			}
		}
	}
}

func TestBackupDeletionRecoveryDoesNotFollowUnexpectedSymlinks(t *testing.T) {
	for _, suffix := range []string{"", ".deleting"} {
		e := Engine{Root: t.TempDir()}
		id := jobs.UUID()
		dir, err := e.dir(id)
		if err != nil {
			t.Fatal(err)
		}
		// Even a dangling symlink is an existing filesystem entry, not absence.
		if err = os.Symlink(filepath.Join(t.TempDir(), "absent"), dir+suffix); err != nil {
			t.Fatal(err)
		}
		now := time.Now()
		state, message, err := e.backupDeletionRecoveryState(id, now.Add(-11*time.Minute), now, os.Lstat)
		if err != nil || state != "failed" || !strings.Contains(message, "需人工核对") {
			t.Fatal("symlink mistaken for absent backup", suffix, state, message, err)
		}
		if _, err = os.Lstat(dir + suffix); err != nil {
			t.Fatal("recovery removed symlink", err)
		}
	}
}

func TestBackupDeletionRecoveryRejectsUnsafeRootAndID(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct{ root, id string }{
		{"relative", jobs.UUID()},
		{"", jobs.UUID()},
		{t.TempDir(), "../../not-owned"},
	} {
		e := Engine{Root: tc.root}
		state, _, err := e.backupDeletionRecoveryState(tc.id, now.Add(-11*time.Minute), now, func(string) (os.FileInfo, error) {
			t.Fatal("unsafe path inspected")
			return nil, nil
		})
		if state != "failed" || !errors.Is(err, ErrUnsafe) {
			t.Fatal("unsafe root/id accepted", state, err)
		}
	}
}
