//go:build linux

package stillstore

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func renameExclusive(dir *os.File, from, to string) error {
	err := unix.Renameat2(int(dir.Fd()), from, int(dir.Fd()), to, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EEXIST) {
		return ErrConflict
	}
	return fsError(err)
}
