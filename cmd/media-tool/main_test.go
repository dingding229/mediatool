package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSuperviseCancelsAndJoinsOnFailure(t *testing.T) {
	var joined atomic.Bool
	failure := errors.New("fixture")
	err := supervise(context.Background(), func(ctx context.Context) error { <-ctx.Done(); joined.Store(true); return nil }, func(context.Context) error { return failure })
	if !errors.Is(err, failure) || !joined.Load() {
		t.Fatal(err)
	}
}
func TestSuperviseSignalAndUnexpectedExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := supervise(ctx, func(ctx context.Context) error { <-ctx.Done(); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := supervise(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("unplanned exit accepted")
	}
}
