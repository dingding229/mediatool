//go:build linux

package sandbox

import (
	"bytes"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func Parent() error {
	if e := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); e != nil {
		return e
	}
	if e := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); e != nil {
		return e
	}
	return PrepareFDs()
}
func PrepareFDs() error {
	files, e := os.ReadDir("/proc/self/fd")
	if e != nil {
		return e
	}
	if len(files) > 8192 {
		return errors.New("descriptor scan limit")
	}
	for _, f := range files {
		fd, e := strconv.Atoi(f.Name())
		if e != nil || fd < 3 {
			continue
		}
		old, e := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if errors.Is(e, unix.EBADF) {
			continue
		}
		if e != nil {
			return e
		}
		if _, e = unix.FcntlInt(uintptr(fd), unix.F_SETFD, old|unix.FD_CLOEXEC); e != nil && !errors.Is(e, unix.EBADF) {
			return e
		}
	}
	return nil
}
func Configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
func Limits() error {
	for _, r := range []struct {
		kind int
		max  uint64
	}{{unix.RLIMIT_AS, 8 << 30}, {unix.RLIMIT_NOFILE, 256}, {unix.RLIMIT_FSIZE, 256 << 20}} {
		var old unix.Rlimit
		if e := unix.Getrlimit(r.kind, &old); e != nil {
			return e
		}
		limit := r.max
		if old.Max < limit {
			limit = old.Max
		}
		if e := unix.Setrlimit(r.kind, &unix.Rlimit{Cur: limit, Max: limit}); e != nil {
			return e
		}
	}
	return nil
}

type Process struct {
	PID, PPID, Group int
	Start            string
	RSS              uint64
	Tasks            uint64
	CPU              time.Duration
}

func Snapshot() ([]Process, error) { return snapshotAt("/proc") }

func snapshotAt(root string) ([]Process, error) {
	entries, e := os.ReadDir(root)
	if e != nil {
		return nil, e
	}
	out := []Process{}
	if len(entries) > 8192 {
		return nil, errors.New("process scan limit")
	}
	for _, d := range entries {
		pid, e := strconv.Atoi(d.Name())
		if e != nil {
			continue
		}
		raw, e := os.ReadFile(root + "/" + d.Name() + "/stat")
		if e != nil {
			continue
		}
		end := strings.LastIndexByte(string(raw), ')')
		if end < 0 {
			continue
		}
		fields := strings.Fields(string(raw[end+1:]))
		if len(fields) < 22 {
			continue
		}
		ppid, e1 := strconv.Atoi(fields[1])
		group, e2 := strconv.Atoi(fields[2])
		pages, e3 := strconv.ParseUint(fields[21], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		utime, e4 := strconv.ParseUint(fields[11], 10, 64)
		stime, e5 := strconv.ParseUint(fields[12], 10, 64)
		tasks, e6 := strconv.ParseUint(fields[17], 10, 64)
		if e4 != nil || e5 != nil || e6 != nil || tasks == 0 {
			continue
		}
		out = append(out, Process{PID: pid, PPID: ppid, Group: group, Start: fields[19], RSS: pages * uint64(os.Getpagesize()), Tasks: tasks, CPU: time.Duration(utime+stime) * 10 * time.Millisecond})
	}
	return out, nil
}

// Tree includes orphans retaining the worker's process group, as well as descendants.
func Tree(root int, all []Process) []Process {
	children := map[int][]int{}
	selected := map[int]bool{root: true}
	queue := []int{root}
	for _, p := range all {
		children[p.PPID] = append(children[p.PPID], p.PID)
		if p.Group == root && !selected[p.PID] {
			selected[p.PID] = true
			queue = append(queue, p.PID)
		}
	}
	for i := 0; i < len(queue); i++ {
		for _, pid := range children[queue[i]] {
			if !selected[pid] {
				selected[pid] = true
				queue = append(queue, pid)
			}
		}
	}
	out := []Process{}
	for _, p := range all {
		if selected[p.PID] {
			out = append(out, p)
		}
	}
	return out
}

func KillGroup(pid int) error {
	e := unix.Kill(-pid, unix.SIGKILL)
	if errors.Is(e, unix.ESRCH) {
		return nil
	}
	return e
}
func KillKnown(p Process) error {
	all, e := Snapshot()
	if e != nil {
		return e
	}
	for _, now := range all {
		if now.PID == p.PID && now.Start == p.Start {
			e = unix.Kill(p.PID, unix.SIGKILL)
			if errors.Is(e, unix.ESRCH) {
				return nil
			}
			return e
		}
	}
	return nil
}

// Call only after cmd.Wait: never steal its direct-child exit status.
func ReapGroup(group int) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var st unix.WaitStatus
		pid, e := unix.Wait4(-group, &st, unix.WNOHANG, nil)
		if errors.Is(e, unix.ECHILD) {
			return nil
		}
		if e != nil {
			return e
		}
		if pid == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	return errors.New("plugin descendants not reaped")
}

// Adopted children that changed process group are attributable by a non-secret
// inherited instance label. Cleared labels are a reviewed trusted-code limitation.
func Orphans(instance string, all []Process) []Process { return tagged(instance, all, true) }
func Tagged(instance string, all []Process) []Process  { return tagged(instance, all, false) }
func tagged(instance string, all []Process, adoptedOnly bool) []Process {
	out := []Process{}
	needle := []byte("MEDIATOOL_PLUGIN_INSTANCE=" + instance + "\x00")
	for _, p := range all {
		if adoptedOnly && p.PPID != os.Getpid() {
			continue
		}
		f, e := os.Open("/proc/" + strconv.Itoa(p.PID) + "/environ")
		if e != nil {
			continue
		}
		raw, e := io.ReadAll(io.LimitReader(f, 8192))
		_ = f.Close()
		if e == nil && bytes.Contains(append([]byte{0}, raw...), append([]byte{0}, needle...)) {
			out = append(out, p)
		}
	}
	return out
}
func ReapKnown(p Process) error { return ReapOwned([]Process{p}) }

// One shared deadline, not two seconds per descendant.
func ReapOwned(owned []Process) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		all, e := Snapshot()
		if e != nil {
			return e
		}
		pending := false
		for _, p := range owned {
			alive := false
			for _, now := range all {
				if now.PID == p.PID && now.Start == p.Start {
					alive = true
					break
				}
			}
			if !alive {
				continue
			}
			var status unix.WaitStatus
			pid, e := unix.Wait4(p.PID, &status, unix.WNOHANG, nil)
			if pid == p.PID {
				continue
			}
			if e != nil && !errors.Is(e, unix.ECHILD) {
				return e
			}
			pending = true
		}
		if !pending {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("plugin child exit not confirmed")
}
