//go:build darwin

package filemanager

import (
	"golang.org/x/sys/unix"
	"os"
)

func renameNoReplace(from *os.File, fromName string, to *os.File, toName string) error {
	return unix.RenameatxNp(int(from.Fd()), fromName, int(to.Fd()), toName, unix.RENAME_EXCL)
}
