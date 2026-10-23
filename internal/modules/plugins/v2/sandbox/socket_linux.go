//go:build linux

package sandbox

import (
	"errors"
	"golang.org/x/sys/unix"
	"runtime"
	"unsafe"
)

// DenyUnixSockets is worker-only. Workers use inherited stdio for Host RPC;
// allowing new Unix sockets would expose app's optional host-root Docker socket
// through inherited supplementary groups. TSYNC covers every existing Go thread.
func DenyUnixSockets() error {
	var arch uint32
	switch runtime.GOARCH {
	case "amd64":
		arch = unix.AUDIT_ARCH_X86_64
	case "arm64":
		arch = unix.AUDIT_ARCH_AARCH64
	default:
		return errors.New("unsupported worker syscall architecture")
	}
	deny := uint32(unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM))
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 4},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: arch, Jt: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: deny},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
		{Code: unix.BPF_ALU | unix.BPF_AND | unix.BPF_K, K: 0x3fffffff},
		// io_uring can create sockets without a socket(2) syscall. Workers only
		// need stdio/epoll; do not let it bypass this newly introduced boundary.
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: uint32(unix.SYS_IO_URING_SETUP), Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: deny},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: uint32(unix.SYS_IO_URING_ENTER), Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: deny},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: uint32(unix.SYS_IO_URING_REGISTER), Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: deny},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: uint32(unix.SYS_SOCKET), Jt: 1},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: uint32(unix.SYS_SOCKETPAIR), Jf: 3},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 16},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.AF_UNIX, Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: deny},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if e := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); e != nil {
		return e
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	result, _, e := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&program)))
	runtime.KeepAlive(filter)
	if e != 0 {
		return e
	}
	if result != 0 {
		return errors.New("worker seccomp thread synchronization failed")
	}
	return nil
}
