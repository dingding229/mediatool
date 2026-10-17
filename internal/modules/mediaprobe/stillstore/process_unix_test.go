//go:build linux || darwin

package stillstore

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreProcessHelper(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_STILLSTORE_HELPER") != "1" {
		t.Skip("subprocess-only helper")
	}
	s, err := Open(os.Getenv("MEDIA_TOOL_STILLSTORE_ROOT"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	phase := os.Getenv("MEDIA_TOOL_STILLSTORE_PHASE")
	s.step = func(stage string) error {
		if phase == "crash-"+stage {
			os.Exit(23)
		} // Simulate a process crash: no Go defers.
		if phase == "pause" && stage == "synced" {
			if _, err := fmt.Fprintln(os.Stdout, "synced"); err != nil {
				os.Exit(24)
			}
			var release [1]byte
			if _, err := io.ReadFull(os.Stdin, release[:]); err != nil {
				os.Exit(25)
			}
		}
		return nil
	}
	if _, err := s.Publish(t.Context(), testAsset, testFP, testJPEG(t, color.Black, 16, 9)); err != nil {
		t.Fatal(err)
	}
}
func helperCommand(t *testing.T, ctx context.Context, root, phase string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestStoreProcessHelper$", "-test.timeout=10s")
	cmd.Env = append(os.Environ(), "MEDIA_TOOL_STILLSTORE_HELPER=1", "MEDIA_TOOL_STILLSTORE_ROOT="+root, "MEDIA_TOOL_STILLSTORE_PHASE="+phase, "GORACE=atexit_sleep_ms=0")
	return cmd
}
func TestSeparateProcessLockAndAtomicVisibility(t *testing.T) {
	s, root := testStore(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := helperCommand(t, ctx, root, "pause")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err == nil && line != "synced\n" {
			err = errors.New("unexpected helper signal")
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			cancel()
			if waitErr := cmd.Wait(); waitErr != nil {
				t.Log(waitErr)
			}
			t.Fatal(err)
		}
	case <-ctx.Done():
		cancel()
		if waitErr := cmd.Wait(); waitErr != nil {
			t.Log(waitErr)
		}
		t.Fatal("helper did not start")
	}
	// While tmp is fully synced but not renamed, readers see no final image.
	relative := "ab/" + testAsset + "-" + testFP[:16] + ".jpg"
	_, _, readErr := s.Read(t.Context(), relative)
	waitCtx, stop := context.WithTimeout(t.Context(), 100*time.Millisecond)
	_, publishErr := s.Publish(waitCtx, testAsset, testFP, testJPEG(t, color.Black, 16, 9))
	stop()
	if _, err := stdin.Write([]byte{1}); err != nil {
		t.Error(err)
	}
	if err := stdin.Close(); err != nil {
		t.Error(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child publish failed: %v %s", err, stderr.String())
	}
	if !errors.Is(readErr, ErrNotFound) || !errors.Is(publishErr, context.DeadlineExceeded) {
		t.Fatal("publication visible early or lock wait ignored context", readErr, publishErr)
	}
	if _, err := s.Publish(t.Context(), testAsset, testFP, testJPEG(t, color.Black, 16, 9)); err != nil {
		t.Fatal("lock not released", err)
	}
	assertNoTemporary(t, root)
}
func TestActualProcessCrashAndIdempotentRecovery(t *testing.T) {
	for _, stage := range []string{"synced", "renamed"} {
		t.Run(stage, func(t *testing.T) {
			s, root := testStore(t)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			raw, err := helperCommand(t, ctx, root, "crash-"+stage).CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 23 {
				t.Fatalf("helper did not crash at barrier: %v %s", err, raw)
			}
			temps, err := filepath.Glob(filepath.Join(root, "ab", ".tmp-*"))
			if err != nil {
				t.Fatal(err)
			}
			_, statErr := os.Stat(imageFile(t, root))
			if stage == "synced" && (len(temps) != 1 || !errors.Is(statErr, os.ErrNotExist)) {
				t.Fatal("pre-rename crash exposed final file")
			}
			if stage == "renamed" && (len(temps) != 0 || statErr != nil) {
				t.Fatal("post-rename crash lost final file")
			}
			// Re-open does not run GC: B2 will remove old orphan/tmp entries after grace.
			// Retry validates the winner and completes directory sync without overwrite.
			receipt, err := s.Publish(t.Context(), testAsset, testFP, testJPEG(t, color.Black, 16, 9))
			if err != nil {
				t.Fatal(err)
			}
			bytes, meta, err := s.Read(t.Context(), receipt.RelativePath)
			if err != nil || len(bytes) == 0 || meta != receipt {
				t.Fatal("crash retry invalid", err)
			}
			after, err := filepath.Glob(filepath.Join(root, "ab", ".tmp-*"))
			if err != nil || len(after) != len(temps) {
				t.Fatal("B1 must not prematurely GC crash files")
			}
		})
	}
}
