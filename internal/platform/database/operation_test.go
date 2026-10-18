package database

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestOptionalCommitGuardIsContextLocalAndPropagatesFailure(t *testing.T) {
	ctx := context.Background()
	if e := CheckCommit(ctx, nil); e != nil {
		t.Fatal(e)
	}
	sentinel := errors.New("revoked")
	called := 0
	c := WithCommitGuard(ctx, func(context.Context, pgx.Tx) error { called++; return sentinel })
	if !errors.Is(Commit(c, nil), sentinel) || called != 1 {
		t.Fatal("guard did not prevent commit")
	}
	if e := CheckCommit(ctx, nil); e != nil {
		t.Fatal("guard escaped caller context")
	}
}

func TestOptionalEffectGuardPreservesCancellationAndContextIsolation(t *testing.T) {
	ctx := context.Background()
	if e := CheckEffect(ctx); e != nil {
		t.Fatal(e)
	}
	sentinel := errors.New("revoked before effect")
	called := 0
	guarded := WithEffectGuard(ctx, func() error { called++; return sentinel })
	if !errors.Is(CheckEffect(guarded), sentinel) || called != 1 {
		t.Fatal("effect guard bypassed")
	}
	if e := CheckEffect(ctx); e != nil {
		t.Fatal("guard escaped context")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if !errors.Is(CheckEffect(cancelled), context.Canceled) {
		t.Fatal("cancellation bypassed")
	}
}
