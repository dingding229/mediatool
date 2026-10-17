//go:build linux

package sandbox

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Landlock is deliberately small and best effort. It adds no network/syscall broker.
// All Go threads must restrict themselves; release workers are built without CGO.
func Landlock(data, tmp string, extra []string) (bool, error) {
	return landlock(data, tmp, extra, landlockABI)
}
func landlockABI() (uintptr, syscall.Errno) {
	abi, _, err := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	return abi, err
}
func landlock(data, tmp string, extra []string, query func() (uintptr, syscall.Errno)) (bool, error) {
	abi, errno := query()
	if errno == unix.ENOSYS || errno == unix.EOPNOTSUPP || errno == unix.ENOMSG {
		return false, nil
	}
	if errno != 0 {
		return false, errno
	}
	rights := uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE | unix.LANDLOCK_ACCESS_FS_MAKE_REG | unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_SYM | unix.LANDLOCK_ACCESS_FS_MAKE_SOCK | unix.LANDLOCK_ACCESS_FS_MAKE_FIFO | unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK)
	if abi >= 2 {
		rights |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		rights |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	attr := unix.LandlockRulesetAttr{Access_fs: rights}
	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), 8, 0)
	if errno != 0 {
		return false, errno
	}
	defer func() { _ = unix.Close(int(fd)) }() // descriptor teardown only
	add := func(name string, access uint64, required bool) error {
		p, e := unix.Open(name, unix.O_PATH|unix.O_CLOEXEC, 0)
		if e != nil {
			if !required && errors.Is(e, os.ErrNotExist) {
				return nil
			}
			return e
		}
		defer func() { _ = unix.Close(p) }() // descriptor teardown only
		var st unix.Stat_t
		if e = unix.Fstat(p, &st); e != nil {
			return e
		}
		if st.Mode&unix.S_IFMT != unix.S_IFDIR {
			access &= uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_TRUNCATE)
		}
		rule := unix.LandlockPathBeneathAttr{Allowed_access: access & rights, Parent_fd: int32(p)}
		_, _, err := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, fd, unix.LANDLOCK_RULE_PATH_BENEATH, uintptr(unsafe.Pointer(&rule)), 0, 0, 0)
		if err != 0 {
			return err
		}
		return nil
	}
	ro := uint64(unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_EXECUTE)
	exe, e := os.Executable()
	if e != nil {
		return false, e
	}
	for _, path := range []string{exe, "/bin", "/usr/bin", "/lib", "/lib64", "/usr/lib", "/etc/ssl/certs", "/etc/resolv.conf", "/etc/hosts", "/etc/nsswitch.conf", "/etc/localtime", "/usr/share/zoneinfo"} {
		if e = add(path, ro, false); e != nil {
			return false, e
		}
	}
	for _, path := range append([]string{data, tmp}, extra...) {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return false, os.ErrInvalid
		}
		if e = add(path, rights, true); e != nil {
			return false, e
		}
	}
	if e = add("/dev/null", uint64(unix.LANDLOCK_ACCESS_FS_READ_FILE|unix.LANDLOCK_ACCESS_FS_WRITE_FILE), false); e != nil {
		return false, e
	}
	_, _, errno = syscall.AllThreadsSyscall6(unix.SYS_PRCTL, unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0, 0)
	if errno == unix.ENOTSUP {
		return false, nil
	}
	if errno != 0 {
		return false, errno
	}
	_, _, errno = syscall.AllThreadsSyscall(unix.SYS_LANDLOCK_RESTRICT_SELF, fd, 0, 0)
	if errno == unix.ENOTSUP {
		return false, nil
	}
	if errno != 0 {
		return false, errno
	}
	return true, nil
}
