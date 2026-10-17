package stillqueue

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// SnapshotPage and Oldest are bounded maintenance cursors, never browse queries.
func (r *Repository) SnapshotPage(ctx context.Context, after int64, unknown bool, limit int) ([]Snapshot, error) {
	return r.snapshotPage(ctx, after, unknown, limit, "item_id")
}
func (r *Repository) Oldest(ctx context.Context, limit int) ([]Snapshot, error) {
	return r.snapshotPage(ctx, 0, false, limit, "last_access_at,item_id")
}
func (r *Repository) snapshotPage(parent context.Context, after int64, unknown bool, limit int, order string) ([]Snapshot, error) {
	if r == nil || r.db == nil || after < 0 || limit < 1 || limit > MaxCacheBatch {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, cacheBudget)
	defer cancel()
	filter := ""
	if unknown {
		filter = " AND size_bytes=0"
	} // Keep the partial index usable under prepared plans.
	// order/filter are fixed literals, never caller input.
	rows, err := r.db.Query(ctx, `SELECT item_id,relative_path,digest,published_at,last_access_at,size_bytes FROM media_stills
 WHERE published_at IS NOT NULL AND item_id>$1 `+filter+` ORDER BY `+order+` LIMIT $2`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Snapshot, 0, limit)
	for rows.Next() {
		var s Snapshot
		if err = rows.Scan(&s.ItemID, &s.RelativePath, &s.Digest, &s.PublishedAt, &s.LastAccessAt, &s.SizeBytes); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Calibrate fences the observed snapshot. size=0 drops a missing/unstatable
// reference, but preserves an independent live attempt instead of killing it.
func (r *Repository) Calibrate(parent context.Context, s Snapshot, size int64) (ok bool, err error) {
	if r == nil || r.db == nil || s.ItemID < 100000 || s.PublishedAt.IsZero() || size < 0 || size > MaxImageBytes {
		return false, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, cacheBudget)
	defer cancel()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer done()
		if e := tx.Rollback(cleanup); e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = errors.Join(err, e)
		}
		if err != nil {
			ok = false
		}
	}()
	if _, err = tx.Exec(ctx, `SELECT set_config('lock_timeout','200ms',true),set_config('statement_timeout','1500ms',true)`); err != nil {
		return false, err
	}
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended('media-tool:native-still-quota',0))`).Scan(&locked); err != nil {
		return false, err
	}
	if !locked {
		return false, ErrCacheBusy
	}
	if size > 0 {
		tag, e := tx.Exec(ctx, `UPDATE media_stills SET size_bytes=$5 WHERE item_id=$1 AND relative_path=$2 AND digest=$3 AND published_at=$4`, s.ItemID, s.RelativePath, s.Digest, s.PublishedAt, size)
		if e != nil {
			return false, e
		}
		ok = tag.RowsAffected() == 1
	} else {
		// Result and attempt are separate. A new lease may already be generating a
		// replacement for an old legacy snapshot; only the successful reference goes.
		tag, e := tx.Exec(ctx, `UPDATE media_stills SET relative_path='',digest='',width=0,height=0,size_bytes=0,
 result_asset_id=NULL,result_fingerprint=NULL,published_at=NULL,last_access_at=NULL,evicted_at=clock_timestamp()
 WHERE item_id=$1 AND relative_path=$2 AND digest=$3 AND published_at=$4 AND state='grabbing' AND lease_until>clock_timestamp()`, s.ItemID, s.RelativePath, s.Digest, s.PublishedAt)
		if e != nil {
			return false, e
		}
		ok = tag.RowsAffected() == 1
		if !ok {
			n, e := evictSnapshots(ctx, tx, []Snapshot{s})
			if e != nil {
				return false, e
			}
			ok = n == 1
		}
	}
	err = tx.Commit(ctx)
	return ok, err
}

// Called under the file publication lock immediately before unlink. Active
// attempts protect even old orphan paths until their fenced completion/expiry.
func (r *Repository) Referenced(parent context.Context, path string) (bool, error) {
	if r == nil || r.db == nil || path == "" || len(path) > 1024 {
		return false, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, cacheBudget)
	defer cancel()
	var used bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_stills WHERE published_at IS NOT NULL AND relative_path=$1
 UNION ALL SELECT 1 FROM media_stills WHERE state='grabbing' AND lease_until>clock_timestamp()
 AND left(asset_id::text,2)||'/'||asset_id::text||'-'||left(fingerprint,16)||'.jpg'=$1)`, path).Scan(&used)
	return used, err
}
