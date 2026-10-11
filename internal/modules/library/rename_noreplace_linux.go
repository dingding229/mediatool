//go:build linux

package library

import (
	"golang.org/x/sys/unix"
	"os"
)

func renameNoReplace(from *os.File, fromName string, to *os.File, toName string) error {
	return unix.Renameat2(int(from.Fd()), fromName, int(to.Fd()), toName, unix.RENAME_NOREPLACE)
}
