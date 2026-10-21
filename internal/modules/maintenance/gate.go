package maintenance

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// BeginWork fences in-flight synchronous writers and scheduler callbacks. The
// restore sets paused first, drains jobs/plugins, then takes the exclusive form
// of this session lock. Never hold a transaction across an external operation.
func BeginWork(ctx context.Context, db *pgxpool.Pool) (func(), error) {
	noop := func() {}
	if db == nil {
		return noop, nil
	}
	conn, err := db.Acquire(ctx)
	if err != nil {
		return noop, err
	}
	var ok bool
	err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock_shared(91220883)`).Scan(&ok)
	if err != nil || !ok {
		conn.Release()
		return noop, ErrConflict
	}
	release := func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(c, `SELECT pg_advisory_unlock_shared(91220883)`); err != nil {
			conn.Conn().Close(c)
		}
		conn.Release()
	}
	var paused bool
	if err = conn.QueryRow(ctx, `SELECT paused FROM public.ops_mode WHERE singleton`).Scan(&paused); err != nil || paused {
		release()
		return noop, ErrConflict
	}
	return release, nil
}
