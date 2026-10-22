package logging

import (
	"context"
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
