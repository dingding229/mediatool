//go:build darwin

package v2

import "golang.org/x/sys/unix"

func exchangeDirs(fd int, a, b string) error { return unix.RenameatxNp(fd, a, fd, b, unix.RENAME_SWAP) }
