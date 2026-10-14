//go:build linux || darwin

package filemanager

import (
	"golang.org/x/sys/unix"
	"os"
)

func directoryWritable(dir *os.File) (bool, error) {
	var st unix.Statfs_t
	if err := unix.Fstatfs(int(dir.Fd()), &st); err != nil {
		return false, ErrUnavailable
	}
	// ST_RDONLY on Linux, MNT_RDONLY on Darwin; both are bit zero.
	if st.Flags&1 != 0 {
		return false, nil
	}
	if err := unix.Faccessat(int(dir.Fd()), ".", unix.W_OK, 0); err != nil {
		return false, nil
	}
	return true, nil
}
