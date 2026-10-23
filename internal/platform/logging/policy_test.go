package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetentionBatchesPreserveRawAndCurrentWriter(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir, "info")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		writeQueryRecord(t, s, now.AddDate(0, 0, -8), "INFO", "old", "app")
	}
	writeQueryRecord(t, s, now, "INFO", "https://owned.invalid/a?api_key=original", "app")
	for _, want := range []int{2, 2, 1, 0} {
		n, e := s.Prune(context.Background(), now.AddDate(0, 0, -7), 2)
		if e != nil || n != want {
			t.Fatal(n, e)
		}
	}
	writeQueryRecord(t, s, now, "INFO", "still-writable", "app")
	out, e := s.Query(context.Background(), Query{Page: 1, PageSize: 200})
	if e != nil || out.Total != 2 {
		t.Fatal(out, e)
	}
	if _, e = os.Stat(filepath.Join(dir, "retention.tmp")); !os.IsNotExist(e) {
		t.Fatal(e)
	}
}
func TestPolicyBoundariesAndExpiry(t *testing.T) {
	for _, n := range []int{0, 1, 7, 90, 91} {
		p := Policy{RetentionDays: n}
		if (p.Validate() == nil) != (n >= 1 && n <= 90) {
			t.Fatal(n)
		}
	}
	s, e := Open(t.TempDir(), "debug")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now()
	p := DefaultPolicy()
	s.ApplyPolicy(p, now)
	if s.Level.Level() != slog.LevelInfo {
		t.Fatal("default debug enabled")
	}
	p.DebugUntil = now.Add(time.Hour)
	s.ApplyPolicy(p, now)
	if s.Level.Level() != slog.LevelDebug {
		t.Fatal("debug not enabled")
	}
	s.ApplyPolicy(p, p.DebugUntil)
	if s.Level.Level() != slog.LevelInfo {
		t.Fatal("debug did not expire")
	}
}
func TestRetentionRejectsLinkAndCancelledOperation(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir, "info")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	outside := filepath.Join(t.TempDir(), "secret")
	if e = os.WriteFile(outside, []byte("private"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(outside, filepath.Join(dir, "retention.tmp")); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Prune(context.Background(), time.Now(), 10); e == nil {
		t.Fatal("accepted unsafe temp")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = s.Prune(ctx, time.Now(), 10); e == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestQuerySnapshotDoesNotShareMutableResponsesAndClearInvalidates(t *testing.T) {
	s, e := Open(t.TempDir(), "info")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now()
	writeQueryRecord(t, s, now.Add(-time.Minute), "INFO", "original", "app")
	q := Query{Before: now, Page: 1, PageSize: 200}
	a, e := s.Query(context.Background(), q)
	if e != nil {
		t.Fatal(e)
	}
	a.Items[0]["msg"] = "modified"
	b, e := s.Query(context.Background(), q)
	if e != nil || b.Items[0]["msg"] != "original" {
		t.Fatal(b, e)
	}
	if e = s.Clear(); e != nil {
		t.Fatal(e)
	}
	b, e = s.Query(context.Background(), q)
	if e != nil || b.Total != 0 {
		t.Fatal(b, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = s.Query(ctx, q); e == nil {
		t.Fatal("cached query ignored cancellation")
	}
}

func TestMaintenanceDoesNotOverwriteJustSavedPolicyWithStaleCopy(t *testing.T) {
	s, e := Open(t.TempDir(), "info")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	defer func() { cancel(); <-done }()
	go func() {
		defer close(done)
		s.Maintain(ctx, func(context.Context) (Policy, error) { p := DefaultPolicy(); p.RetentionDays = 1; return p, nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		v, _ := s.Stats()
		if v.Policy.RetentionDays == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("maintenance did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	p := DefaultPolicy()
	p.DebugUntil = time.Now().Add(time.Hour)
	s.ApplyPolicy(p, time.Now())
	time.Sleep(1100 * time.Millisecond)
	if s.Level.Level() != slog.LevelDebug {
		t.Fatal("maintenance reverted current saved settings")
	}
}

func TestClearCannotPublishInvalidationBeforeObtainingStoreLock(t *testing.T) {
	s, e := Open(t.TempDir(), "info")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.mu.Lock()
	epoch := s.queryEpoch.Load()
	entered, done := make(chan struct{}), make(chan error, 1)
	go func() { close(entered); done <- s.Clear() }()
	<-entered
	time.Sleep(20 * time.Millisecond)
	early := s.queryEpoch.Load() != epoch
	s.mu.Unlock()
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if early {
		t.Fatal("cache invalidation was observable before clearing could acquire the mutation lock")
	}
	if s.queryEpoch.Load() == epoch {
		t.Fatal("completed clear did not invalidate snapshots")
	}
}
func TestLogSnapshotPreservesStringSourceFromProductionShape(t *testing.T) {
	s, e := Open(t.TempDir(), "info")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	raw := []byte(`{"time":"2026-10-23T00:00:00Z","level":"INFO","source":"owned_source.go:7","msg":"source-string","url":"https://owned.invalid/?original=value"}` + "\n")
	if _, e = s.Write(raw); e != nil {
		t.Fatal(e)
	}
	out, e := s.Query(context.Background(), Query{Page: 1, PageSize: 200})
	if e != nil || out.Total != 1 || out.Items[0]["source"] != "owned_source.go:7" || out.Items[0]["url"] != "https://owned.invalid/?original=value" {
		t.Fatal(out, e)
	}
}

func TestRestartRecoversOnlyOwnedBoundedRetentionOutput(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0644} {
		dir := t.TempDir()
		path := filepath.Join(dir, "retention.tmp")
		if e := os.WriteFile(path, []byte("partial internal output"), mode); e != nil {
			t.Fatal(e)
		}
		if e := os.Chmod(path, mode); e != nil {
			t.Fatal(e)
		}
		s, e := Open(dir, "info")
		if mode == 0600 {
			if e != nil {
				t.Fatal(e)
			}
			s.Close()
			if _, e = os.Stat(path); !os.IsNotExist(e) {
				t.Fatal("owned orphan retained", e)
			}
		} else {
			if e == nil {
				s.Close()
				t.Fatal("unexpected file silently removed")
			}
			if _, e = os.Stat(path); e != nil {
				t.Fatal("unrecognized output removed", e)
			}
		}
	}
}

func TestUnreadablePolicyClosesDebugWithoutShorteningRetentionOrDeleting(t *testing.T) {
	s, e := Open(t.TempDir(), "info")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now()
	p := DefaultPolicy()
	p.RetentionDays = 90
	p.DebugUntil = now.Add(time.Hour)
	s.ApplyPolicy(p, now)
	writeQueryRecord(t, s, now.AddDate(0, 0, -8), "INFO", "retained", "app")
	ctx, cancel := context.WithCancel(context.Background())
	done, entered := make(chan struct{}), make(chan struct{})
	defer func() { cancel(); <-done }()
	go func() {
		defer close(done)
		s.Maintain(ctx, func(context.Context) (Policy, error) { close(entered); return Policy{}, errors.New("unavailable") }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	<-entered
	time.Sleep(20 * time.Millisecond)
	summary, _ := s.Stats()
	out, e := s.Query(ctx, Query{Page: 1, PageSize: 200})
	if e != nil || out.Total != 1 || summary.Policy.RetentionDays != 90 || summary.DebugActive {
		t.Fatal(summary, out.Total, e)
	}
}
func TestStalePolicyReadCannotOverwriteAcknowledgedSave(t *testing.T) {
	s, e := Open(t.TempDir(), "info")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now()
	writeQueryRecord(t, s, now.AddDate(0, 0, -8), "INFO", "retained", "app")
	ctx, cancel := context.WithCancel(context.Background())
	done, entered, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer func() { cancel(); <-done }()
	go func() {
		defer close(done)
		s.Maintain(ctx, func(context.Context) (Policy, error) { close(entered); <-release; return DefaultPolicy(), nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	<-entered
	p := DefaultPolicy()
	p.RetentionDays = 90
	s.ApplyPolicy(p, now)
	close(release)
	time.Sleep(20 * time.Millisecond)
	summary, _ := s.Stats()
	out, e := s.Query(ctx, Query{Page: 1, PageSize: 200})
	if e != nil || out.Total != 1 || summary.Policy.RetentionDays != 90 {
		t.Fatal(summary, out.Total, e)
	}
}
func TestEachRetentionBatchUsesLatestAcknowledgedDays(t *testing.T) {
	s, e := Open(t.TempDir(), "info")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now()
	writeQueryRecord(t, s, now.AddDate(0, 0, -8), "INFO", "retained", "app")
	s.ApplyPolicy(DefaultPolicy(), now)
	p := DefaultPolicy()
	p.RetentionDays = 90
	s.ApplyPolicy(p, now)
	n, e := s.pruneRetained(context.Background(), now, 1000)
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
}
