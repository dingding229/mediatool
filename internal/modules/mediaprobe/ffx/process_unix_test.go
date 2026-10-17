//go:build linux || darwin

package ffx

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func helperBlock(mode string) {
	if mode == "ignore" {
		signal.Ignore(syscall.SIGTERM)
	}
	path := os.Getenv("MEDIA_TOOL_FFX_PID_FILE")
	if path == "" {
		time.Sleep(20 * time.Second)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	child := exec.Command(exe)
	child.Env = append(os.Environ(), "MEDIA_TOOL_FFX_PID_FILE=")
	if err := child.Start(); err != nil {
		os.Exit(2)
	}
	raw, err := json.Marshal([]int{os.Getpid(), child.Process.Pid})
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		os.Exit(2)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	// The child inherits our group and must also receive the group's SIGTERM.
	// Waiting, not killing it here, catches a direct-parent-only cancellation bug.
	if err := child.Wait(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			os.Exit(2)
		}
	}
	signal.Stop(signals)
}
func TestTimeoutAndCancelKillProcessGroupAndReap(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "cancel"}[cancelled], func(t *testing.T) {
			r := fakeRunner(t, Config{ProbeTimeout: 800 * time.Millisecond})
			t.Setenv("MEDIA_TOOL_FFX_MODE", "block")
			file := filepath.Join(t.TempDir(), "helper-pids.json")
			t.Setenv("MEDIA_TOOL_FFX_PID_FILE", file)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			start := time.Now()
			go func() { _, err := r.Probe(ctx, syntheticURL, Options{}); done <- err }()
			var pids []int
			for deadline := time.Now().Add(600 * time.Millisecond); time.Now().Before(deadline); {
				raw, err := os.ReadFile(file)
				if err == nil && json.Unmarshal(raw, &pids) == nil && len(pids) == 2 {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if len(pids) != 2 {
				cancel()
				<-done
				t.Fatal("helper tree did not start")
			}
			group, err := syscall.Getpgid(pids[1])
			if err != nil || group != pids[0] {
				t.Fatal("descendant is not in owned process group", group, pids, err)
			}
			if cancelled {
				cancel()
			}
			err = <-done
			want := context.DeadlineExceeded
			if cancelled {
				want = context.Canceled
			}
			if !errors.Is(err, want) || time.Since(start) > 2*time.Second {
				t.Fatal("timeout/cancel result", err)
			}
			for _, pid := range pids {
				if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
					t.Fatalf("unreaped/alive child %d: %v", pid, err)
				}
			}
		})
	}
	r := fakeRunner(t, Config{ProbeTimeout: 80 * time.Millisecond})
	t.Setenv("MEDIA_TOOL_FFX_MODE", "ignore")
	if _, err := r.Probe(context.Background(), syntheticURL, Options{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("SIGTERM-resistant process not killed", err)
	}
}
