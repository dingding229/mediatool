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

func TestCommitGuardsComposeRatherThanReplacingHostAuthority(t *testing.T) {
	ctx := context.Background()
	order := []int{}
	for _, i := range []int{1, 2, 3} {
		ctx = WithCommitGuard(ctx, func(context.Context, pgx.Tx) error { order = append(order, i); return nil })
	}
	if e := CheckCommit(ctx, nil); e != nil || len(order) != 3 || order[0] != 1 || order[2] != 3 {
		t.Fatal(order, e)
	}
	denied := errors.New("parent host revoked")
	ctx = WithCommitGuard(context.Background(), func(context.Context, pgx.Tx) error { return denied })
	ctx = WithCommitGuard(ctx, func(context.Context, pgx.Tx) error { t.Fatal("later guard bypassed parent"); return nil })
	if !errors.Is(CheckCommit(ctx, nil), denied) {
		t.Fatal("parent fence lost")
	}
}
