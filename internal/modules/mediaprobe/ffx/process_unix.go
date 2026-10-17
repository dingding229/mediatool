//go:build linux || darwin

package ffx

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func configureGroup(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}
func stopGroup(cmd *exec.Cmd, grace bool) error {
	if cmd.Process == nil {
		return nil
	}
	kill := func(signal syscall.Signal) error {
		err := syscall.Kill(-cmd.Process.Pid, signal)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	if grace {
		if err := kill(syscall.SIGTERM); err != nil {
			return err
		}
		// Give well-behaved tools a brief window to reap their children, then force
		// kill the whole group even if a descendant ignored SIGTERM.
		time.Sleep(100 * time.Millisecond)
	}
	if err := kill(syscall.SIGKILL); err != nil {
		return err
	}
	return nil
}
