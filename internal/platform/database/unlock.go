package database

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	deferutil "media-tool/internal/platform/cleanup"
	"time"
)

// Unlock releases a session advisory lock before the pool connection is returned.
// On error/false, discard the physical connection; never return a still-locked
// session to the pool. SQL/keys are fixed by internal callers, not HTTP input.
func Unlock(parent context.Context, conn *pgxpool.Conn, query string, args ...any) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
	defer cancel()
	var released bool
	if err := conn.QueryRow(ctx, query, args...).Scan(&released); err != nil || !released {
		closeCtx, done := context.WithTimeout(context.WithoutCancel(parent), time.Second)
		defer done()
		deferutil.Ignore(conn.Conn().Close(closeCtx), "Discarding failed-unlock connection; pgx closes its transport even when protocol termination fails")
	}
}
