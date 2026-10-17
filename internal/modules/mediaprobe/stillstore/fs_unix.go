//go:build linux || darwin

package stillstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const directoryFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

func fsError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.ENOENT) {
		return ErrNotFound
	}
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
		return ErrUnsafe
	}
	return ErrUnavailable
}

// Every component is opened relative to an already trusted directory handle.
// Never resolve symlinks, even links pointing elsewhere inside the still root.
func openRoot(path string) (out *os.File, err error) {
	fd, err := unix.Open("/", directoryFlags, 0)
	if err != nil {
		return nil, fsError(err)
	}
	dir := os.NewFile(uintptr(fd), "still-parent")
	defer func() {
		if dir != nil {
			closeFile(dir, &err)
		}
	}()
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		next, e := openDir(dir, part, i >= len(parts)-2)
		if e != nil {
			return nil, e
		}
		closeErr := dir.Close()
		dir = next
		if closeErr != nil {
			return nil, ErrUnavailable
		}
	}
	out = dir
	dir = nil
	return out, nil
}
func openDir(parent *os.File, name string, create bool) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), name, directoryFlags, 0)
	if errors.Is(err, unix.ENOENT) && create {
		err = unix.Mkdirat(int(parent.Fd()), name, 0700)
		if err != nil && !errors.Is(err, unix.EEXIST) {
			return nil, fsError(err)
		}
		fd, err = unix.Openat(int(parent.Fd()), name, directoryFlags, 0)
	}
	if err != nil {
		return nil, fsError(err)
	}
	file := os.NewFile(uintptr(fd), "still-directory")
	// Sync even pre-existing entries: another opener may have crashed between
	// mkdir and syncing this parent. Read-only opens never sync directories.
	if create && parent.Sync() != nil {
		err := error(ErrUnavailable)
		closeFile(file, &err)
		return nil, err
	}
	return file, nil
}
func openShard(root *os.File, name string, create bool) (*os.File, error) {
	if len(name) != 2 || strings.Trim(name, "0123456789abcdef") != "" {
		return nil, ErrUnsafe
	}
	return openDir(root, name, create)
}
func checkedFile(fd int) (file *os.File, err error) {
	file = os.NewFile(uintptr(fd), "still-file")
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil {
		err = ErrUnavailable
	} else if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 {
		err = ErrUnsafe
	}
	if err != nil {
		closeFile(file, &err)
		return nil, err
	}
	return file, nil
}
func openLeaf(dir *os.File, name string) (*os.File, error) {
	if strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return nil, ErrUnsafe
	}
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fsError(err)
	}
	return checkedFile(fd)
}
func temporary(dir *os.File) (*os.File, string, error) {
	for i := 0; i < 3; i++ {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return nil, "", ErrUnavailable
		}
		name := ".tmp-" + hex.EncodeToString(nonce[:])
		fd, err := unix.Openat(int(dir.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return nil, "", fsError(err)
		}
		file, err := checkedFile(fd)
		if err != nil {
			discard(dir, name, &err)
			return nil, "", err
		}
		return file, name, nil
	}
	return nil, "", ErrUnavailable
}

// Distinct opens are intentional: flock on one shared open-file description
// would not serialize concurrent goroutines using that same description.
func lockRoot(ctx context.Context, root *os.File) (out *os.File, err error) {
	fd, e := unix.Openat(int(root.Fd()), ".lock", unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if errors.Is(e, unix.EEXIST) {
		fd, e = unix.Openat(int(root.Fd()), ".lock", unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	}
	if e != nil {
		return nil, fsError(e)
	}
	file, err := checkedFile(fd)
	if err != nil {
		return nil, err
	}
	defer func() {
		if out == nil {
			closeFile(file, &err)
		}
	}()
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if e == nil {
			return file, nil
		}
		if !errors.Is(e, unix.EWOULDBLOCK) && !errors.Is(e, unix.EAGAIN) {
			return nil, ErrUnavailable
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
func discard(dir *os.File, name string, err *error) {
	e := unix.Unlinkat(int(dir.Fd()), name, 0)
	if errors.Is(e, unix.ENOENT) {
		return
	} // A successful no-replace rename consumed it.
	if e != nil {
		*err = errors.Join(*err, ErrUnavailable)
		return
	}
	if dir.Sync() != nil {
		*err = errors.Join(*err, ErrUnavailable)
	}
}
