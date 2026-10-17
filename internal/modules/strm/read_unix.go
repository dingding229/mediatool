//go:build unix

package strm

import "golang.org/x/sys/unix"

const readNonblock = unix.O_NONBLOCK
