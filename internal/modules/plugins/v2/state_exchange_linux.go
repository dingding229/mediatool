//go:build linux

package v2

import "golang.org/x/sys/unix"

func exchangeDirs(fd int, a, b string) error {
	return unix.Renameat2(fd, a, fd, b, unix.RENAME_EXCHANGE)
}
