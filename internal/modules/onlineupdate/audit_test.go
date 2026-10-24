package onlineupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReconciliationWithoutTasksDoesNotRequireWritableRootOrDatabase(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	s := Service{Root: root}
	if e := s.reconcileAudits(t.Context()); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(root); !os.IsNotExist(e) {
		t.Fatal("empty reconciliation created runtime state", e)
	}
}
