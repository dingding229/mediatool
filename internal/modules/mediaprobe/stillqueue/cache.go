package stillqueue

import (
	"context"
	"errors"
	dbutil "media-tool/internal/platform/database"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrLeaseLost = errors.New("still lease lost")
var ErrCacheBusy = errors.New("still cache busy")
var ErrUnknownSize = errors.New("still cache requires size reconciliation")

const cacheBudget = 2 * time.Second
const MaxCacheBatch = 200

type Usage struct{ SnapshotBytes, ReservedBytes, UnknownSizes int64 }
type Admission struct {
	Reserved bool
	Evicted  []Snapshot
}
type Access struct {
	ItemID               int64
	RelativePath, Digest string
}

// Expired reservations are logically free even before the bounded recovery
// sweep clears their physical columns. Snapshot sizes never use filesystem atime.
const usageSQL = `SELECT
 (SELECT coalesce(sum(size_bytes),0)::bigint FROM media_stills WHERE published_at IS NOT NULL),
 (SELECT coalesce(sum(reserved_bytes),0)::bigint FROM media_stills WHERE reserved_bytes>0 AND lease_until>statement_timestamp()),
 (SELECT count(*) FROM media_stills WHERE published_at IS NOT NULL AND size_bytes=0)`

func (r *Repository) Usage(parent context.Context) (u Usage, err error) {
	if r == nil || r.db == nil {
		return u, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, cacheBudget)
	defer cancel()
	err = r.db.QueryRow(ctx, usageSQL).Scan(&u.SnapshotBytes, &u.ReservedBytes, &u.UnknownSizes)
	return u, err
}

// Admit is DB-only. B2-B must call it under the publication lock, process Evicted
// paths only AFTER commit, and publish only when Reserved. <=200 evictions may
// make partial progress without admitting a large request; retry on a later run.
func (r *Repository) Admit(parent context.Context, l Lease, size int64) (out Admission, err error) {
	if r == nil || r.db == nil || !validLease(l) || size < 1 || size > MaxImageBytes {
		return out, ErrInvalid
	}
	if size > r.config.MaxBytes {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(parent, cacheBudget)
	defer cancel()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer done()
		if e := tx.Rollback(cleanup); e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = errors.Join(err, e)
		}
		if err != nil {
			out = Admission{}
		} // Never authorize file deletion on an uncertain commit.
	}()
	if _, err = tx.Exec(ctx, `SELECT set_config('lock_timeout','200ms',true),set_config('statement_timeout','1500ms',true)`); err != nil {
		return out, err
	}
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended('media-tool:native-still-quota',0))`).Scan(&locked); err != nil {
		return out, err
	}
	if !locked {
		return out, ErrCacheBusy
	}
	var prior int64
	err = tx.QueryRow(ctx, `SELECT reserved_bytes FROM media_stills WHERE item_id=$1 AND asset_id=$2 AND fingerprint=$3
 AND lease_token=$4 AND attempts=$5 AND state='grabbing' AND lease_until>clock_timestamp() AND (`+enabledSQL+`) FOR UPDATE`, l.ItemID, l.AssetID, l.Fingerprint, l.Token, l.Attempt).Scan(&prior)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrLeaseLost
	}
	if err != nil {
		return out, err
	}
	var u Usage
	if err = tx.QueryRow(ctx, usageSQL).Scan(&u.SnapshotBytes, &u.ReservedBytes, &u.UnknownSizes); err != nil {
		return out, err
	}
	if u.UnknownSizes > 0 {
		return out, ErrUnknownSize
	}
	used := u.SnapshotBytes + u.ReservedBytes - prior
	if used+size > r.config.MaxBytes {
		rows, e := tx.Query(ctx, `SELECT item_id,relative_path,digest,published_at,size_bytes,
 CASE WHEN state='grabbing' AND lease_until>clock_timestamp() THEN reserved_bytes ELSE 0 END FROM media_stills
 WHERE published_at IS NOT NULL AND item_id<>$1 ORDER BY last_access_at,item_id LIMIT 200 FOR UPDATE SKIP LOCKED`, l.ItemID)
		if e != nil {
			return out, e
		}
		target := min(r.config.MaxBytes*9/10, r.config.MaxBytes-size)
		for rows.Next() {
			var s Snapshot
			var reserved int64
			if e = rows.Scan(&s.ItemID, &s.RelativePath, &s.Digest, &s.PublishedAt, &s.SizeBytes, &reserved); e != nil {
				break
			}
			if used > target {
				out.Evicted = append(out.Evicted, s)
				used -= s.SizeBytes + reserved
			}
		}
		e = errors.Join(e, rows.Err())
		rows.Close()
		if e != nil {
			return out, e
		}
		if len(out.Evicted) > 0 {
			var n int64
			n, err = evictSnapshots(ctx, tx, out.Evicted)
			if err != nil {
				return out, err
			}
			if n != int64(len(out.Evicted)) {
				return out, ErrLeaseLost
			}
		}
		// Do not publish unless the low-water target was reached. Partial evictions
		// commit (and are returned) so a bounded retry can continue, without scanning.
		out.Reserved = used <= target
	} else {
		out.Reserved = true
	}
	// Recheck the request fence even when committing only partial eviction progress.
	tag, e := tx.Exec(ctx, `UPDATE media_stills SET reserved_bytes=CASE WHEN $7 THEN $6 ELSE reserved_bytes END WHERE item_id=$1 AND asset_id=$2 AND fingerprint=$3
 AND lease_token=$4 AND attempts=$5 AND state='grabbing' AND lease_until>clock_timestamp()`, l.ItemID, l.AssetID, l.Fingerprint, l.Token, l.Attempt, size, out.Reserved)
	if e != nil {
		return out, e
	}
	if tag.RowsAffected() != 1 {
		return out, ErrLeaseLost
	}

	err = dbutil.Commit(ctx, tx)
	return out, err
}

const evictSQL = `UPDATE media_stills s SET state='skipped',relative_path='',digest='',width=0,height=0,size_bytes=0,
 result_asset_id=NULL,result_fingerprint=NULL,published_at=NULL,last_access_at=NULL,reserved_bytes=0,
 grabbing_started_at=NULL,lease_until=NULL,lease_token=NULL,evicted_at=clock_timestamp(),next_attempt_at=clock_timestamp()+$5::bigint*interval '1 millisecond',
 last_error_class='permanent',last_error_reason='cache_evicted',updated_at=now()
 FROM unnest($1::bigint[],$2::text[],$3::text[],$4::timestamptz[]) e(id,path,digest,published)
 WHERE s.item_id=e.id AND s.relative_path=e.path AND s.digest=e.digest AND s.published_at=e.published`

func evictSnapshots(ctx context.Context, tx pgx.Tx, snapshots []Snapshot) (int64, error) {
	ids := make([]int64, 0, len(snapshots))
	paths := make([]string, 0, len(snapshots))
	digests := make([]string, 0, len(snapshots))
	stamps := make([]time.Time, 0, len(snapshots))
	for _, s := range snapshots {
		ids = append(ids, s.ItemID)
		paths = append(paths, s.RelativePath)
		digests = append(digests, s.Digest)
		stamps = append(stamps, s.PublishedAt)
	}
	tag, err := tx.Exec(ctx, evictSQL, ids, paths, digests, stamps, EvictionCooldown.Milliseconds())
	return tag.RowsAffected(), err
}

// TouchBatch is the backing sink for B2-B's bounded in-memory coalescer. Database
// time + a conditional UPDATE enforces <=1 write/hour, even across processes.
func (r *Repository) TouchBatch(parent context.Context, accesses []Access) (int64, error) {
	if r == nil || r.db == nil || len(accesses) > MaxCacheBatch {
		return 0, ErrInvalid
	}
	if len(accesses) == 0 {
		return 0, nil
	}
	ids := make([]int64, 0, len(accesses))
	paths := make([]string, 0, len(accesses))
	digests := make([]string, 0, len(accesses))
	for _, a := range accesses {
		if a.ItemID < 100000 || a.RelativePath == "" || len(a.RelativePath) > 1024 || !hex64.MatchString(a.Digest) {
			return 0, ErrInvalid
		}
		ids = append(ids, a.ItemID)
		paths = append(paths, a.RelativePath)
		digests = append(digests, a.Digest)
	}
	ctx, cancel := context.WithTimeout(parent, cacheBudget)
	defer cancel()
	tag, err := dbutil.Exec(ctx, r.db, `UPDATE media_stills s SET last_access_at=clock_timestamp()
 FROM unnest($1::bigint[],$2::text[],$3::text[]) a(id,path,digest) WHERE s.item_id=a.id AND s.relative_path=a.path AND s.digest=a.digest
 AND s.published_at IS NOT NULL AND s.last_access_at<=clock_timestamp()-interval '1 hour'`, ids, paths, digests)
	return tag.RowsAffected(), err
}

// EvictSnapshot fences an observed reference. The caller may delete its file
// only after true, under B2-B's publication lock and fresh in-use check. An error
// (including uncertain commit) NEVER authorizes deletion; GC handles orphans.
func (r *Repository) EvictSnapshot(parent context.Context, s Snapshot) (ok bool, err error) {
	if r == nil || r.db == nil || s.ItemID < 100000 || s.RelativePath == "" || len(s.RelativePath) > 1024 || !hex64.MatchString(s.Digest) || s.PublishedAt.IsZero() {
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
	n, err := evictSnapshots(ctx, tx, []Snapshot{s})
	if err != nil {
		return false, err
	}
	err = dbutil.Commit(ctx, tx)
	return n == 1, err
}
