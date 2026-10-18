package stillqueue

import (
	"context"
	"encoding/json"
	"errors"
	dbutil "media-tool/internal/platform/database"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe"
	"media-tool/internal/modules/mediaprobe/linksource"
)

type Repository struct {
	db     *pgxpool.Pool
	config Config
}

func New(db *pgxpool.Pool, c Config) (*Repository, error) {
	c, err := c.Normalize()
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, ErrInvalid
	}
	return &Repository{db: db, config: c}, nil
}
func (r *Repository) Limits() Config { return r.config }

const enabledSQL = `SELECT enabled AND probe_enabled AND stills_enabled FROM native_server_settings WHERE singleton`

func (r *Repository) Enabled(parent context.Context) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()
	var enabled bool
	err := r.db.QueryRow(ctx, enabledSQL).Scan(&enabled)
	return enabled, err
}
func (r *Repository) Enqueue(ctx context.Context, itemID int64) (bool, error) {
	return r.enqueue(ctx, itemID, false, false)
}
func (r *Repository) Retry(ctx context.Context, itemID int64) (bool, error) {
	return r.enqueue(ctx, itemID, true, false)
}

// RequeueEvicted is the image-request recovery path; normal sweeps never revive
// the same evicted source. Row locking makes concurrent requests coalesce once.
func (r *Repository) RequeueEvicted(ctx context.Context, itemID int64) (bool, error) {
	return r.enqueue(ctx, itemID, false, true)
}
func (r *Repository) enqueue(parent context.Context, itemID int64, force, evictedOnly bool) (queued bool, err error) {
	if r == nil || r.db == nil || itemID < 100000 {
		return false, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		if e := tx.Rollback(ctx); err == nil && e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = e
		}
	}()
	var on bool
	if err = tx.QueryRow(ctx, enabledSQL+` FOR SHARE`).Scan(&on); err != nil {
		return false, err
	}
	if !on {
		return false, ErrDisabled
	}
	// Serialize candidate selection + row changes per episode, not per asset.
	// No network, process, file access or global lock exists inside this transaction.
	var id int64
	err = tx.QueryRow(ctx, `SELECT id FROM native_item_ids WHERE id=$1 AND entity_type='episode' FOR UPDATE`, itemID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotEligible
	}
	if err != nil {
		return false, err
	}
	if evictedOnly {
		var due bool
		err = tx.QueryRow(ctx, `SELECT evicted_at<=clock_timestamp()-$2::bigint*interval '1 millisecond' FROM media_stills
 WHERE item_id=$1 AND state='skipped' AND last_error_reason='cache_evicted' FOR UPDATE`, itemID, EvictionCooldown.Milliseconds()).Scan(&due)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !due {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	var asset, fp string
	var ticks int64
	var hasStill bool
	if err = tx.QueryRow(ctx, candidateSQL, itemID).Scan(&id, &asset, &fp, &ticks, &hasStill); err != nil {
		return false, err
	}
	if asset == "" || hasStill {
		reason := SourceUnavailable
		if hasStill {
			reason = TMDBStill
		}
		// Invalidate an old lease but retain its successful file snapshot for GC/D.
		tag, writeErr := tx.Exec(ctx, `INSERT INTO media_stills(item_id,state,last_error_class,last_error_reason)VALUES($1,'skipped','permanent',$2)
 ON CONFLICT(item_id) DO UPDATE SET state='skipped',asset_id=NULL,fingerprint='',duration_ticks=0,grabbing_started_at=NULL,lease_until=NULL,lease_token=NULL,reserved_bytes=0,evicted_at=NULL,
 last_error_class='permanent',last_error_reason=excluded.last_error_reason,updated_at=now()`, itemID, reason)
		err = writeErr
		if err == nil && tag.RowsAffected() != 1 {
			return false, ErrInvalid
		}
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO media_stills AS s(item_id,asset_id,fingerprint,duration_ticks)VALUES($1,$2,$3,$4)
 ON CONFLICT(item_id) DO UPDATE SET asset_id=excluded.asset_id,fingerprint=excluded.fingerprint,duration_ticks=excluded.duration_ticks,
 state='pending',attempts=0,next_attempt_at=clock_timestamp(),grabbing_started_at=NULL,lease_until=NULL,lease_token=NULL,reserved_bytes=0,evicted_at=NULL,last_error_class=NULL,last_error_reason=NULL,updated_at=now()
 WHERE s.asset_id IS DISTINCT FROM excluded.asset_id OR s.fingerprint<>excluded.fingerprint OR $5`, itemID, asset, fp, ticks, force || evictedOnly)
		if err == nil {
			err = tx.QueryRow(ctx, `SELECT state='pending' AND next_attempt_at<=clock_timestamp() FROM media_stills WHERE item_id=$1`, itemID).Scan(&queued)
		}
		if err == nil && queued {
			payload, e := json.Marshal(struct {
				ItemID      int64  `json:"itemId"`
				AssetID     string `json:"assetId"`
				Fingerprint string `json:"fingerprint"`
			}{itemID, asset, fp})
			if e != nil {
				return false, e
			}
			_, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,queue,payload,idempotency_key,max_attempts)VALUES($1,'native.still','still',$2,$3,1)
 ON CONFLICT(idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET state=CASE WHEN jobs.state='running' THEN 'running' ELSE 'pending' END,
 attempts=CASE WHEN jobs.state='running' THEN jobs.attempts ELSE 0 END,payload=CASE WHEN jobs.state='running' THEN excluded.payload||'{"rerun":true}'::jsonb ELSE excluded.payload END,
 available_at=now(),last_error=NULL,updated_at=now() WHERE jobs.state<>'pending'`, jobs.UUID(), payload, "still:"+asset+":"+fp)
		}
	}
	if err != nil {
		return false, err
	}
	if force {
		if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','native.still.retry',jsonb_build_object('itemId',$1::bigint,'assetId',$2::text,'fingerprint',$3::text))`, itemID, asset, fp); err != nil {
			return false, err
		}
	}
	return queued, dbutil.Commit(ctx, tx)
}

// Caller must reserve both execution lanes BEFORE Claim (T25-C). Lease starts
// from one database clock sample; waits never create a durable attempt.
func (r *Repository) Claim(parent context.Context, itemID int64) (*Lease, error) {
	if r == nil || r.db == nil || itemID < 100000 {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	l := Lease{ItemID: itemID, Token: jobs.UUID()}
	err := r.db.QueryRow(ctx, `WITH current AS (`+candidateSQL+`), chosen AS(
 SELECT s.item_id,c.asset_id,c.fingerprint,c.ticks FROM media_stills s JOIN current c ON c.id=s.item_id
 WHERE s.item_id=$1 AND s.state='pending' AND s.next_attempt_at<=clock_timestamp() AND NOT c.has_still AND c.asset_id<>''
 AND s.asset_id::text=c.asset_id AND s.fingerprint=c.fingerprint AND (`+enabledSQL+`)
 FOR UPDATE OF s SKIP LOCKED),stamp AS MATERIALIZED(SELECT clock_timestamp() t)
 UPDATE media_stills s SET state='grabbing',grabbing_started_at=stamp.t,lease_until=stamp.t+$3::bigint*interval '1 millisecond',lease_token=$2,
 duration_ticks=c.ticks,attempts=least(s.attempts,2147483646)+1,updated_at=now() FROM chosen c,stamp WHERE s.item_id=c.item_id
 RETURNING s.asset_id::text,s.fingerprint,s.duration_ticks,s.attempts,s.lease_until`, itemID, l.Token, r.config.Lease.Milliseconds()).Scan(&l.AssetID, &l.Fingerprint, &l.DurationTicks, &l.Attempt, &l.Until)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// Metadata publication only. T25-B/C must immutably publish the file first;
// stale receipts return false and leave only an unreferenced, GC-able file.
func (r *Repository) Succeed(parent context.Context, l Lease, image Image) (bool, error) {
	path, err := ImagePath(l.AssetID, l.Fingerprint)
	if r == nil || r.db == nil || !validLease(l) || err != nil || image.RelativePath != path || !hex64.MatchString(image.Digest) || image.Width < 1 || image.Width > 1280 || image.Height < 1 || image.Height > 16384 || image.SizeBytes < 1 || image.SizeBytes > MaxImageBytes {
		return false, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	tag, err := r.db.Exec(ctx, `WITH current AS (`+candidateSQL+`) UPDATE media_stills s SET state='ready',
 result_asset_id=s.asset_id,result_fingerprint=s.fingerprint,relative_path=$6,digest=$7,width=$8,height=$9,size_bytes=$10,reserved_bytes=0,evicted_at=NULL,published_at=clock_timestamp(),last_access_at=clock_timestamp(),
 grabbing_started_at=NULL,lease_until=NULL,lease_token=NULL,last_error_class=NULL,last_error_reason=NULL,updated_at=now()
 FROM current c WHERE s.item_id=$1 AND s.asset_id=$2 AND s.fingerprint=$3 AND s.lease_token=$4 AND s.attempts=$5
 AND s.state='grabbing' AND s.lease_until>clock_timestamp() AND s.reserved_bytes=$10 AND (`+enabledSQL+`)
 AND NOT c.has_still AND c.asset_id=s.asset_id::text AND c.fingerprint=s.fingerprint`, l.ItemID, l.AssetID, l.Fingerprint, l.Token, l.Attempt, image.RelativePath, image.Digest, image.Width, image.Height, image.SizeBytes)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
func (r *Repository) Fail(parent context.Context, l Lease, f Failure) (bool, error) {
	if r == nil || r.db == nil || !validLease(l) || !f.valid() {
		return false, ErrInvalid
	}
	state := mediaprobe.StillFailed
	delay := time.Duration(0)
	if f.Class == linksource.Manual {
		state = mediaprobe.StillNeedsAction
	}
	if f.Class == linksource.Retryable {
		state = mediaprobe.StillPending
		delay = Backoff(l.Attempt, f.RetryAfter)
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	tag, err := r.db.Exec(ctx, `UPDATE media_stills SET state=$6,next_attempt_at=clock_timestamp()+$7::bigint*interval '1 millisecond',
 last_error_class=$8,last_error_reason=$9,grabbing_started_at=NULL,lease_until=NULL,lease_token=NULL,reserved_bytes=0,updated_at=now()
 WHERE item_id=$1 AND asset_id=$2 AND fingerprint=$3 AND lease_token=$4 AND attempts=$5 AND state='grabbing' AND lease_until>clock_timestamp()`, l.ItemID, l.AssetID, l.Fingerprint, l.Token, l.Attempt, state, delay.Milliseconds(), f.Class, f.Reason)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
func (r *Repository) RecoverExpired(parent context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	tag, err := r.db.Exec(ctx, `WITH expired AS(SELECT item_id FROM media_stills WHERE state='grabbing' AND lease_until<=clock_timestamp() ORDER BY lease_until,item_id LIMIT 200 FOR UPDATE SKIP LOCKED)
 UPDATE media_stills s SET state='pending',next_attempt_at=clock_timestamp(),grabbing_started_at=NULL,lease_until=NULL,lease_token=NULL,reserved_bytes=0,
 last_error_class='retryable',last_error_reason='lease_expired',updated_at=now() FROM expired e WHERE s.item_id=e.item_id`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
func (r *Repository) Current(parent context.Context, itemID int64) (*Snapshot, error) {
	if r == nil || r.db == nil || itemID < 100000 {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	s := Snapshot{ItemID: itemID}
	err := r.db.QueryRow(ctx, `SELECT relative_path,digest,width,height,size_bytes,coalesce(result_asset_id::text,''),coalesce(result_fingerprint,''),published_at,last_access_at
 FROM media_stills WHERE item_id=$1 AND published_at IS NOT NULL`, itemID).Scan(&s.RelativePath, &s.Digest, &s.Width, &s.Height, &s.SizeBytes, &s.AssetID, &s.Fingerprint, &s.PublishedAt, &s.LastAccessAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}
