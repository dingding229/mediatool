package database

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	deferutil "media-tool/internal/platform/cleanup"
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

// WithEffectGuard gates nontransactional filesystem/network effects. The caller
// must still retain evidence/journals: external outcomes cannot be rolled back.
type effectGuardKey struct{}

func WithEffectGuard(ctx context.Context, guard func() error) context.Context {
	return context.WithValue(ctx, effectGuardKey{}, guard)
}
func CheckEffect(ctx context.Context) error {
	if guard, ok := ctx.Value(effectGuardKey{}).(func() error); ok {
		return guard()
	}
	return ctx.Err()
}

// Exec preserves autocommit for built-ins, but makes a plugin's final state
// projection transactional with its live authorization fence.
func Exec(ctx context.Context, db *pgxpool.Pool, sql string, args ...any) (pgconn.CommandTag, error) {
	if _, ok := ctx.Value(commitGuardKey{}).(CommitGuard); !ok {
		return db.Exec(ctx, sql, args...)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	defer deferutil.Rollback(ctx, tx)
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return tag, err
	}
	return tag, Commit(ctx, tx)
}
