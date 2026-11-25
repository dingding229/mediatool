//go:build linux

package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"media-tool/internal/platform/testworker"
)

// Test-only launcher: emulate a kernel without Landlock for the actual worker
// binary. There is no production environment variable to fake kernel support.
func init() {
	if len(os.Args) < 3 || os.Args[1] != "test-without-landlock" {
		return
	}
	fail := func(err error) {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(31)
		}
	}
	runtime.LockOSThread()
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_LANDLOCK_CREATE_RULESET, Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.ENOSYS)},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	fail(unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0))
	_, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, 0, uintptr(unsafe.Pointer(&program)))
	runtime.KeepAlive(filter)
	if errno != 0 {
		fail(errno)
	}
	fail(unix.Exec(os.Args[2], os.Args[2:], os.Environ()))
}

func TestRP34WorkerLandlockPolicy(t *testing.T) {
	binary := os.Getenv("MEDIA_TOOL_PLUGIN_BINARY")
	if binary == "" {
		t.Skip("requires static application binary from kernel gate")
	}
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(t.TempDir(), "without-landlock")
	script := "#!/bin/sh\nexec " + strconv.Quote(testBinary) + " test-without-landlock " + strconv.Quote(binary) + " \"$@\"\n"
	if err = os.WriteFile(shim, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err = sandbox.Parent(); err != nil {
		t.Fatal(err)
	}
	for _, override := range []bool{false, true} {
		name := map[bool]string{false: "default_reject", true: "audited_override"}[override]
		t.Run(name, func(t *testing.T) {
			t.Setenv(sandbox.AllowWithoutLandlockEnv, map[bool]string{true: "1"}[override])
			s := fixture(t)
			d, err := definition("hello-go")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			row, err := s.Apply(ctx, "test-admin", Request{Kind: Install, OperationKey: "rp34-install", PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Confirm: true, Scopes: scopes(d), Config: json.RawMessage(`{}`)})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Apply(ctx, "test-admin", Request{Kind: Enable, OperationKey: "rp34-enable", InstanceID: row.ID, ExpectedRevision: row.Revision, Fingerprint: d.Fingerprint, Confirm: true}); err != nil {
				t.Fatal(err)
			}
			lease, err := s.Claim(ctx, jobs.UUID())
			if err != nil {
				t.Fatal(err)
			}
			super := Supervisor{Store: s, Executable: shim, Logger: slog.Default(), PerWorkerRSS: 256 << 20, MaxTasks: 256}
			if override {
				live, stop := context.WithCancel(ctx)
				done := make(chan Reason, 1)
				go func() { done <- super.execute(live, lease) }()
				defer func() { stop(); <-done }()
				testworker.Ready(t, s.DB, row.ID, func(c context.Context) error { _, err := s.UI(c, row.ID, "form", ""); return err })
			} else {
				why := super.execute(ctx, lease)
				if why != SandboxUnavailable {
					t.Fatal("worker did not refuse unavailable Landlock", why)
				}
				if err = s.Finish(ctx, lease, why); err != nil {
					t.Fatal(err)
				}
				var state, reason string
				if err = s.DB.QueryRow(ctx, `SELECT runtime_state,reason FROM plugin_v2_instances WHERE id=$1`, row.ID).Scan(&state, &reason); err != nil || state != "needs_action" || reason != "sandbox_unavailable" {
					t.Fatal("reason not visible in instance state", state, reason, err)
				}
			}
			var audited bool
			if err = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_logs WHERE action='plugins.sandbox.override' AND details->>'instanceId'=$1 AND details->>'generation'=$2)`, row.ID, lease.Token).Scan(&audited); err != nil || audited != override {
				t.Fatal("wrong override audit outcome", audited, err)
			}
		})
	}
}
