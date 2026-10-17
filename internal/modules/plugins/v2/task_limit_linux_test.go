//go:build linux

package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Initialize RPC unwinds asynchronously when the watchdog closes the peer.
// Capture its concurrent bounded diagnostics without racing the assertions.
type taskLog struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *taskLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}
func (b *taskLog) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.Buffer.String() }

// Helper is test-only: no plugin mutation or runtime test switch is shipped.
func TestTaskLoadHelper(t *testing.T) {
	mode := os.Getenv("PLUGIN_TASK_LOAD_HELPER")
	if mode == "" {
		return
	}
	if mode == "threads" {
		var ready sync.WaitGroup
		ready.Add(40)
		for range 40 {
			go func() { runtime.LockOSThread(); ready.Done(); select {} }()
		}
		ready.Wait()
	} else if mode == "fork" {
		// Fixed bounded fanout, never a fork bomb.
		for range 40 {
			c := exec.Command("/bin/sleep", "30")
			if e := c.Start(); e != nil {
				os.Exit(3)
			}
		}
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

func TestWorkerTaskWatchdogKillsForkAndThreadLoad(t *testing.T) {
	if e := sandbox.Parent(); e != nil {
		t.Fatal(e)
	}
	// A same-UID sibling exceeds the tested budget, but must not count against
	// any worker. This reproduces the shared-UID NPROC failure without changing it.
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	sibling := exec.Command(exe, "-test.run=^TestTaskLoadHelper$")
	sibling.Env = []string{"PLUGIN_TASK_LOAD_HELPER=threads", "GOMAXPROCS=2"}
	sandbox.Configure(sibling)
	if e = sibling.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = sandbox.KillGroup(sibling.Process.Pid); _ = sibling.Wait() })
	siblingReady := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		all, e := sandbox.Snapshot()
		if e != nil {
			t.Fatal(e)
		}
		for _, p := range all {
			if p.PID == sibling.Process.Pid && p.Tasks > 32 {
				siblingReady = true
			}
		}
		if siblingReady {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !siblingReady {
		t.Fatal("same-UID sibling task load not established")
	}
	for _, mode := range []string{"normal", "fork", "threads"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			exe, e := os.Executable()
			if e != nil {
				t.Fatal(e)
			}
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestTaskLoadHelper$")
			cmd.Env = []string{"PLUGIN_TASK_LOAD_HELPER=" + mode, "GOMAXPROCS=2", "PATH=/usr/bin:/bin"}
			sandbox.Configure(cmd)
			var logs taskLog
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			waited := false
			defer func() {
				_ = sandbox.KillGroup(cmd.Process.Pid)
				if !waited {
					_ = cmd.Wait()
				}
				if e := sandbox.ReapGroup(cmd.Process.Pid); e != nil {
					t.Error(e)
				}
			}()
			s := Supervisor{MaxTasks: 32, Logger: slog.New(slog.NewTextHandler(&logs, nil))}
			observed := false
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
				all, e := sandbox.Snapshot()
				if e != nil {
					t.Fatal(e)
				}
				tree := sandbox.Tree(cmd.Process.Pid, all)
				var tasks uint64
				for _, p := range tree {
					tasks += p.Tasks
				}
				killed := s.enforceTasks(tasks, "test-instance", cmd.Process.Pid)
				if mode == "normal" {
					if killed {
						t.Fatal("normal worker killed")
					}
					if tasks > 0 {
						observed = true
					}
					time.Sleep(20 * time.Millisecond)
					if time.Until(deadline) < 4500*time.Millisecond {
						break
					}
				} else if killed {
					observed = true
					if e = cmd.Wait(); e == nil {
						t.Fatal("over-budget worker survived")
					}
					waited = true
					if !strings.Contains(logs.String(), "reason=tasks_exceeded") {
						t.Fatal("missing bounded reason")
					}
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if !observed {
				t.Fatal("task budget not observed", mode)
			}
		})
	}
}

func TestTaskLimitFencedFinishAndCrashBackoff(t *testing.T) {
	s := fixture(t)
	if e := sandbox.Parent(); e != nil {
		t.Fatal(e)
	}
	defs, e := Catalog()
	if e != nil {
		t.Fatal(e)
	}
	d := defs[0]
	ctx := context.Background()
	first, e := s.Apply(ctx, "test-admin", Request{Kind: Install, OperationKey: "tasks-install", PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Confirm: true, Scopes: scopes(d), Config: json.RawMessage(`{}`)})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Apply(ctx, "test-admin", Request{Kind: Enable, OperationKey: "tasks-enable", InstanceID: first.ID, ExpectedRevision: first.Revision, Fingerprint: d.Fingerprint, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	l, e := s.Claim(ctx, jobs.UUID())
	if e != nil {
		t.Fatal(e)
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	wrapper := filepath.Join(t.TempDir(), "task-fixture")
	// Test-only process simulates an uncooperative plugin during initialize. It
	// never answers RPC; the real supervisor sampler must kill it before timeout.
	body := "#!/bin/sh\nexport PLUGIN_TASK_LOAD_HELPER=threads\nexec '" + strings.ReplaceAll(exe, "'", "'\"'\"'") + "' -test.run=^TestTaskLoadHelper$\n"
	if e = os.WriteFile(wrapper, []byte(body), 0700); e != nil {
		t.Fatal(e)
	}
	var logs taskLog
	super := Supervisor{Store: s, Executable: wrapper, Logger: slog.New(slog.NewTextHandler(&logs, nil)), MaxTasks: 32, PerWorkerRSS: 256 << 20}
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	reason := super.execute(runCtx, l)
	if reason != ResourceLimit || !strings.Contains(logs.String(), "reason=tasks_exceeded") {
		t.Fatal(reason, logs.String())
	}
	if e = s.Finish(ctx, l, reason); e != nil {
		t.Fatal(e)
	}
	var state State
	var stored Reason
	var pid int
	var later bool
	if e = s.DB.QueryRow(ctx, `SELECT runtime_state,reason,worker_pid,next_restart_at>now() FROM plugin_v2_instances WHERE id=$1`, first.ID).Scan(&state, &stored, &pid, &later); e != nil {
		t.Fatal(e)
	}
	if state != Backoff || stored != ResourceLimit || pid != 0 || !later {
		t.Fatal(state, stored, pid, later)
	}
	// A second (late) finish is fenced; it cannot turn the failed generation ready.
	if e = s.Finish(ctx, l, NoReason); e == nil {
		t.Fatal("late finish accepted")
	}
	t.Log("tasks_exceeded -> group kill/reap -> resource_limit/backoff; late finish fenced")
}
