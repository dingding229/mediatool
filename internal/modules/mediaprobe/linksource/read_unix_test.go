//go:build unix

package linksource

import (
	"context"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRootReaderRejectsFIFOWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "fifo.strm")
	if err := unix.Mkfifo(name, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := (RootReader{Root: root}).Read(context.Background(), "fifo.strm")
	assertFailure(t, err, Manual, Mismatch)
}
