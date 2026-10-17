//go:build darwin

package stillstore

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func renameExclusive(dir *os.File, from, to string) error {
	err := unix.RenameatxNp(int(dir.Fd()), from, int(dir.Fd()), to, unix.RENAME_EXCL)
	if errors.Is(err, unix.EEXIST) {
		return ErrConflict
	}
	return fsError(err)
}
