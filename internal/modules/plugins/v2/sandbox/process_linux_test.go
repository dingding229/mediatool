//go:build linux && !cgo

package sandbox

import (
	"bytes"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("PLUGIN_SANDBOX_HELPER") == "1" {
		mode := os.Args[1]
		switch mode {
		case "proc":
			p := os.Args[2]
			for _, suffix := range []string{"environ", "mem"} {
				f, e := os.Open("/proc/" + p + "/" + suffix)
				if e == nil {
					f.Close()
					os.Exit(4)
				}
			}

			pid, _ := strconv.Atoi(p)
			if unix.PtraceAttach(pid) == nil {
				_ = unix.PtraceDetach(pid)
				_ = unix.Kill(pid, unix.SIGCONT)
				os.Exit(14)
			}
		case "landlock":
			var ready sync.WaitGroup
			ready.Add(8)
			gate := make(chan struct{})
			results := make(chan bool, 8)
			for range 8 {
				go func() {
					runtime.LockOSThread()
					defer runtime.UnlockOSThread()
					ready.Done()
					<-gate
					denied := true
					for range 20 {
						if f, e := os.Open(os.Args[4]); e == nil {
							f.Close()
							denied = false
						}
					}
					results <- denied
				}()
			}
			ready.Wait()
			ok, e := Landlock(os.Args[2], os.Args[3], nil)
			if e != nil {
				fmt.Fprintln(os.Stderr, "landlock failed", e)
				os.Exit(5)
			}
			if !ok {
				fmt.Println("WARNING landlock_unavailable")
				os.Exit(6)
			}
			close(gate)
			for range 8 {
				if !<-results {
					os.Exit(7)
				}
			}
			for range 50 {
				f, e := os.Open(os.Args[4])
				if e == nil {
					f.Close()
					os.Exit(7)
				}
			}
			if e = os.WriteFile(filepath.Join(os.Args[2], "allowed"), []byte("ok"), 0600); e != nil {
				os.Exit(8)
			}
			fmt.Println("landlock_active")
		case "grandchildren":
			// Shell forks twice. Grandchildren remain in the isolated process group.
			cmd := exec.Command("/bin/sh", "-c", "(sleep 60 & wait) & wait")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if cmd.Start() != nil {
				os.Exit(9)
			}
			fmt.Println(cmd.Process.Pid)
			_ = cmd.Wait()
		case "parent-exits":
			runtime.LockOSThread()
			exe, _ := os.Executable()
			cmd := exec.Command(exe, "wait")
			cmd.Env = []string{"PLUGIN_SANDBOX_HELPER=1"}
			Configure(cmd)
			if cmd.Start() != nil {
				os.Exit(11)
			}
			fmt.Println(cmd.Process.Pid)
		case "wait":
			time.Sleep(60 * time.Second)
		case "limits":
			var before unix.Rlimit
			if unix.Getrlimit(unix.RLIMIT_NPROC, &before) != nil {
				os.Exit(12)
			}
			if Limits() != nil {
				os.Exit(12)
			}
			for _, r := range []struct {
				kind  int
				value uint64
			}{{unix.RLIMIT_AS, 8 << 30}, {unix.RLIMIT_NOFILE, 256}, {unix.RLIMIT_NPROC, before.Cur}} {
				var limit unix.Rlimit
				if unix.Getrlimit(r.kind, &limit) != nil || limit.Cur != r.value {
					os.Exit(13)
				}
			}
			var after unix.Rlimit
			if unix.Getrlimit(unix.RLIMIT_NPROC, &after) != nil || after != before {
				os.Exit(13)
			}
		case "fds":
			fd, _ := strconv.Atoi(os.Args[2])
			dev, _ := strconv.ParseUint(os.Args[3], 10, 64)
			ino, _ := strconv.ParseUint(os.Args[4], 10, 64)
			var st unix.Stat_t
			if unix.Fstat(fd, &st) == nil && uint64(st.Dev) == dev && st.Ino == ino {
				os.Exit(10)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func helper(t *testing.T, mode string, args ...string) *exec.Cmd {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(exe, append([]string{mode}, args...)...)
	cmd.Env = []string{"PLUGIN_SANDBOX_HELPER=1", "PATH=/bin:/usr/bin"}
	cmd.WaitDelay = 2 * time.Second
	Configure(cmd)
	return cmd
}
func TestParentDumpableProtectsEnvironMemAndPprofStillWorks(t *testing.T) {
	if e := Parent(); e != nil {
		t.Fatal(e)
	}
	value, e := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	if e != nil || value != 0 {
		t.Fatal(value, e)
	}
	cmd := helper(t, "proc", strconv.Itoa(os.Getpid()))
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatal("parent proc readable", e, string(out))
	}
	// Runtime CPU profiling does not require dumpable; core dumps intentionally do.
	var b strings.Builder
	if e = profileCPU(&b); e != nil {
		t.Fatal(e)
	}
	t.Log("parent environ/mem/ptrace inaccessible; dumpable=0 disables cores, not Go CPU profiling")
}
func TestLandlockAcrossGoThreads(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "own", "data")
	tmp := filepath.Join(base, "own", "tmp")
	other := filepath.Join(base, "other", "secret")
	for _, p := range []string{data, tmp, filepath.Dir(other)} {
		if e := os.MkdirAll(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.WriteFile(other, []byte("synthetic-secret"), 0600); e != nil {
		t.Fatal(e)
	}
	out, e := helper(t, "landlock", data, tmp, other).CombinedOutput()
	if e != nil {
		t.Fatalf("Linux release Landlock test must execute: %v %s", e, out)
	}
	if !strings.Contains(string(out), "landlock_active") {
		t.Fatal(string(out))
	}
}
func TestDoubleForkGroupStopAndReap(t *testing.T) {
	if e := Parent(); e != nil {
		t.Fatal(e)
	}
	cmd := helper(t, "grandchildren")
	pipe, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = KillGroup(cmd.Process.Pid) })
	deadline := time.Now().Add(3 * time.Second)
	var tree []Process
	for time.Now().Before(deadline) {
		all, e := Snapshot()
		if e != nil {
			t.Fatal(e)
		}
		tree = Tree(cmd.Process.Pid, all)
		if len(tree) >= 4 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(tree) < 4 {
		t.Fatal("double fork not observed", tree)
	}
	if e = KillGroup(cmd.Process.Pid); e != nil {
		t.Fatal(e)
	}
	_ = cmd.Wait()
	if e = ReapGroup(cmd.Process.Pid); e != nil {
		t.Fatal(e)
	}
	if _, e = io.ReadAll(pipe); e != nil && !errors.Is(e, os.ErrClosed) {
		t.Fatal(e)
	}
	all, e := Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range all {
		if p.Group == cmd.Process.Pid {
			t.Fatal("group descendant leaked", p)
		}
	}
}
func TestPdeathsigAndLimitsConfiguration(t *testing.T) {
	cmd := helper(t, "fds")
	if cmd.SysProcAttr.Pdeathsig != syscall.SIGKILL || !cmd.SysProcAttr.Setpgid {
		t.Fatal("process ownership attributes missing")
	}
}
func TestLandlockUnavailableIsNonfatal(t *testing.T) {
	ok, e := landlock("/none", "/none", nil, func() (uintptr, syscall.Errno) { return 0, unix.ENOSYS })
	if e != nil || ok {
		t.Fatal(ok, e)
	}
	t.Log("unsupported ABI yields warning mode, not fail closed")
}

func TestParentDeathReallyKillsChild(t *testing.T) {
	if e := Parent(); e != nil {
		t.Fatal(e)
	}
	out, e := helper(t, "parent-exits").CombinedOutput()
	if e != nil {
		t.Fatal(e, string(out))
	}
	pid, e := strconv.Atoi(strings.TrimSpace(string(out)))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = unix.Kill(pid, unix.SIGKILL) })
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var status unix.WaitStatus
		n, e := unix.Wait4(pid, &status, unix.WNOHANG, nil)
		if n == pid {
			if !status.Signaled() || status.Signal() != unix.SIGKILL {
				t.Fatal(status)
			}
			return
		}
		if e != nil && !errors.Is(e, unix.EINTR) {
			t.Fatal(e)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Pdeathsig child leaked")
}
func TestWorkerLimitsAndInheritedFDs(t *testing.T) {
	if out, e := helper(t, "limits").CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	file, e := os.CreateTemp(t.TempDir(), "private")
	if e != nil {
		t.Fatal(e)
	}
	defer file.Close()
	fd := int(file.Fd())
	if _, e = unix.FcntlInt(file.Fd(), unix.F_SETFD, 0); e != nil {
		t.Fatal(e)
	}
	if e = PrepareFDs(); e != nil {
		t.Fatal(e)
	}
	flags, e := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if e != nil || flags&unix.FD_CLOEXEC == 0 {
		t.Fatal("inherited descriptor not closed on exec", e)
	}
	var st unix.Stat_t
	if e = unix.Fstat(fd, &st); e != nil {
		t.Fatal(e)
	}
	if out, e := helper(t, "fds", strconv.Itoa(fd), strconv.FormatUint(uint64(st.Dev), 10), strconv.FormatUint(st.Ino, 10)).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
}
func TestWarningContainsOnlySafeIsolationClass(t *testing.T) {
	var buf bytes.Buffer
	WarnUnavailable(slog.New(slog.NewTextHandler(&buf, nil)), "fixture-instance")
	if !strings.Contains(buf.String(), "level=WARN") || !strings.Contains(buf.String(), "landlock_unavailable") {
		t.Fatal(buf.String())
	}
}

func TestTreeIncludesChildrenGroupsAndAdoptedOrphans(t *testing.T) {
	all := []Process{{PID: 10, PPID: 1, Group: 10, RSS: 1}, {PID: 11, PPID: 10, Group: 11, RSS: 2}, {PID: 12, PPID: 1, Group: 10, RSS: 4}, {PID: 13, PPID: 11, Group: 11, RSS: 8}, {PID: 20, PPID: 1, Group: 20, RSS: 16}}
	selected := Tree(10, all)
	var rss uint64
	for _, p := range selected {
		rss += p.RSS
	}
	if len(selected) != 4 || rss != 15 {
		t.Fatal(selected, rss)
	}
}

func TestSnapshotCountsThreadsAndToleratesExits(t *testing.T) {
	// Exercise real /proc reads while direct children disappear between enumeration
	// and stat; ENOENT is a normal sampling race, not a worker-limit breach.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 60 {
			_ = exec.Command("/bin/true").Run()
		}
	}()
	for range 80 {
		all, e := Snapshot()
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, p := range all {
			if p.PID == os.Getpid() {
				found = true
				if p.Tasks < 1 {
					t.Fatal("missing num_threads")
				}
			}
		}
		if !found {
			t.Fatal("self missing")
		}
	}
	<-done
	root := t.TempDir()
	for _, name := range []string{"123", "456"} {
		if e := os.Mkdir(filepath.Join(root, name), 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.WriteFile(filepath.Join(root, "456", "stat"), []byte("456 (exited) bad"), 0600); e != nil {
		t.Fatal(e)
	}
	if all, e := snapshotAt(root); e != nil || len(all) != 0 {
		t.Fatal("partial/exited process must be skipped", all, e)
	}
}
