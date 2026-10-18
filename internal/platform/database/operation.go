package database

import (
	"context"
	"github.com/jackc/pgx/v5"
)

type commitGuardKey struct{}
type CommitGuard func(context.Context, pgx.Tx) error

// WithCommitGuard adds a final authorization fence to the business transaction.
// It never holds an authorization lock over network I/O. Built-in callers retain
// their original behavior; a plugin cannot construct this context on the wire.
func WithCommitGuard(ctx context.Context, guard CommitGuard) context.Context {
	return context.WithValue(ctx, commitGuardKey{}, guard)
}
func CheckCommit(ctx context.Context, tx pgx.Tx) error {
	if guard, ok := ctx.Value(commitGuardKey{}).(CommitGuard); ok {
		return guard(ctx, tx)
	}
	return nil
}
func Commit(ctx context.Context, tx pgx.Tx) error {
	if err := CheckCommit(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
