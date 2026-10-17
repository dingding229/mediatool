//go:build !linux

package sandbox

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func Parent() error                                   { return nil }
func Configure(cmd *exec.Cmd)                         { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func Limits() error                                   { return nil }
func Landlock(string, string, []string) (bool, error) { return false, nil }

type Process struct {
	PID, PPID, Group int
	Start            string
	RSS              uint64
	Tasks            uint64
	CPU              time.Duration
}

func Snapshot() ([]Process, error)  { return nil, errors.New("Linux process accounting required") }
func Tree(int, []Process) []Process { return nil }
func KillGroup(pid int) error {
	e := syscall.Kill(-pid, syscall.SIGKILL)
	if e == syscall.ESRCH {
		return nil
	}
	return e
}
func KillKnown(p Process) error { return nil }
func ReapGroup(int) error       { return nil }

func Orphans(string, []Process) []Process { return nil }
func ReapKnown(Process) error             { return nil }

func PrepareFDs() error { return nil }

func Tagged(string, []Process) []Process { return nil }

func ReapOwned([]Process) error { return nil }
