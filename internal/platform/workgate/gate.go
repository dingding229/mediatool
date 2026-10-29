// Package workgate owns the lightweight admission fence shared by background
// work and maintenance, without depending on backup/restore execution.
package workgate

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/updateidle"
)

var ErrConflict = errors.New("维护状态已变化，请刷新后重新预览")

// BeginWork fences in-flight synchronous writers and scheduler callbacks. The
// restore sets paused first, drains jobs/plugins, then takes the exclusive form
// of this session lock. Never hold a transaction across an external operation.
func BeginWork(ctx context.Context, db *pgxpool.Pool) (func(), error) {
	noop := func() {}
	if db == nil {
		return noop, nil
	}
	gateRelease, allowed := updateidle.Default.Enter()
	if !allowed {
		return noop, ErrConflict
	}
	conn, err := db.Acquire(ctx)
	if err != nil {
		gateRelease()
		return noop, err
	}
	var ok bool
	err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock_shared(91220883)`).Scan(&ok)
	if err != nil || !ok {
		conn.Release()
		gateRelease()
		return noop, ErrConflict
	}
	release := func() {
		defer gateRelease()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(c, `SELECT pg_advisory_unlock_shared(91220883)`); err != nil {
			deferutil.CloseContext(c, conn.Conn(), "Discard only after failed advisory unlock; no application data is written during teardown")
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
