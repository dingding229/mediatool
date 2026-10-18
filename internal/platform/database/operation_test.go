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
