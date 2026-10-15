package library

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRecoveryReturnsAllArtifactFailuresAndStillFinalizesSafeItems(t *testing.T) {
	transient := errors.New("permission unavailable")
	finish := errors.New("finalize failed")
	calls := 0
	finalized := false
	err := recoverArtifactIDs(context.Background(), []string{"conflict", "transient", "safe"}, func(_ context.Context, id string) error {
		calls++
		if id == "conflict" {
			return ErrArtifactConflict
		}
		if id == "transient" {
			return transient
		}
		return nil
	}, func(context.Context) error { finalized = true; return finish })
	if calls != 3 || !finalized || !errors.Is(err, ErrArtifactConflict) || !errors.Is(err, transient) || !errors.Is(err, finish) || !strings.Contains(err.Error(), "conflict") {
		t.Fatal("recovery errors were swallowed", calls, finalized, err)
	}
}
func TestCanceledArtifactRecoveryDoesNotReportSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := recoverArtifactIDs(ctx, []string{"one"}, func(context.Context, string) error { t.Fatal("work after cancellation"); return nil }, func(context.Context) error { t.Fatal("finalize after cancellation"); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
