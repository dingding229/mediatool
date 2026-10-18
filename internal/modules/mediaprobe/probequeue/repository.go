package probequeue

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/mediaprobe/probeinfo"
	dbutil "media-tool/internal/platform/database"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/nativelibrary/catalog"
)

type Repository struct{ DB *pgxpool.Pool }

const currentFingerprint = `native_probe_fingerprint(f.size_bytes,f.provider_file_id,f.metadata)`

// Enqueue changes the desired fingerprint without touching the current result.
// A same-fingerprint call never overrides backoff or a terminal state.
// Version-zero ready snapshots transition once to pending; after that failed/
// pending/probing states cannot be reset by schema sweeps. The result is kept.
func (r Repository) Enqueue(ctx context.Context, assetID string) (bool, error) {
	return r.enqueue(ctx, assetID, false)
}
func (r Repository) enqueue(parent context.Context, assetID string, force bool) (queued bool, err error) {
	if r.DB == nil || !validID(assetID) {
		return false, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	assetID = strings.ToLower(assetID)
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		if closeErr := tx.Rollback(ctx); err == nil && closeErr != nil && !errors.Is(closeErr, pgx.ErrTxClosed) {
			err = closeErr
		}
	}()
	var enabled bool
	if err = tx.QueryRow(ctx, `SELECT enabled AND probe_enabled FROM native_server_settings WHERE singleton FOR SHARE`).Scan(&enabled); err != nil {
		return false, err
	}
	if !enabled {
		return false, ErrDisabled
	}
	var file, fingerprint string
	// Serialize with file changes only for this short local transaction.
	err = tx.QueryRow(ctx, `WITH visible AS (`+catalog.ExposableAssetsSQL+`) SELECT f.id::text,`+currentFingerprint+`
 FROM cloud_files f WHERE EXISTS(SELECT 1 FROM visible v WHERE v.asset_id=$1 AND v.cloud_file_id=f.id) FOR SHARE OF f`, assetID).Scan(&file, &fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotExposable
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO media_probe_results AS p(cloud_file_id,file_fingerprint)VALUES($1,$2)
 ON CONFLICT(cloud_file_id) DO UPDATE SET file_fingerprint=excluded.file_fingerprint,state='pending',attempts=0,
 next_attempt_at=clock_timestamp(),probing_started_at=NULL,lease_until=NULL,lease_token=NULL,last_error_class=NULL,last_error_reason=NULL,updated_at=now()
 WHERE p.file_fingerprint<>excluded.file_fingerprint OR $3
 OR (p.state='ready' AND coalesce(p.result->>'schema_version','0')='0')`, file, fingerprint, force)
	if err != nil {
		return false, err
	}
	var due bool
	if err = tx.QueryRow(ctx, `SELECT state='pending' AND next_attempt_at<=clock_timestamp() FROM media_probe_results WHERE cloud_file_id=$1`, file).Scan(&due); err != nil {
		return false, err
	}
	if due {
		payload, marshalErr := json.Marshal(struct {
			AssetID     string `json:"assetId"`
			Fingerprint string `json:"fingerprint"`
		}{assetID, fingerprint})
		if marshalErr != nil {
			return false, marshalErr
		}
		// Running wake-ups request one more notification on successful completion.
		// max_attempts=1 prevents generic job retry; the probe table owns all timing.
		_, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,queue,payload,idempotency_key,max_attempts)VALUES($1,'native.probe','probe',$2,$3,1)
 ON CONFLICT(idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET
 state=CASE WHEN jobs.state='running' THEN 'running' ELSE 'pending' END,
 attempts=CASE WHEN jobs.state='running' THEN jobs.attempts ELSE 0 END,
 payload=CASE WHEN jobs.state='running' THEN excluded.payload||'{"rerun":true}'::jsonb ELSE excluded.payload END,
 available_at=now(),last_error=NULL,updated_at=now()
 WHERE jobs.state<>'pending'`, jobs.UUID(), payload, "probe:"+assetID+":"+fingerprint)
		if err != nil {
			return false, err
		}
	}
	if force {
		if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','native.probe.retry',jsonb_build_object('assetId',$1::text,'fingerprint',$2::text))`, assetID, fingerprint); err != nil {
			return false, err
		}
	}
	return due, dbutil.Commit(ctx, tx)
}
func (r Repository) Retry(ctx context.Context, assetID string) (bool, error) {
	return r.enqueue(ctx, assetID, true)
}

// Claim never uses job attempts/available_at. A wake-up may be obsolete: the
// current row alone decides whether to run. No external call holds a row lock.
func (r Repository) Claim(parent context.Context, assetID string, duration time.Duration) (*Lease, error) {
	if r.DB == nil || !validID(assetID) || duration < time.Millisecond || duration > 4*time.Minute {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	assetID = strings.ToLower(assetID)
	l := Lease{AssetID: assetID, Token: jobs.UUID()}
	err := r.DB.QueryRow(ctx, `WITH visible AS (`+catalog.ExposableAssetsSQL+`), candidate AS(
 SELECT p.cloud_file_id FROM media_probe_results p JOIN cloud_files f ON f.id=p.cloud_file_id
 WHERE (SELECT enabled AND probe_enabled FROM native_server_settings WHERE singleton) AND p.state='pending' AND p.next_attempt_at<=clock_timestamp() AND p.file_fingerprint=`+currentFingerprint+`
 AND EXISTS(SELECT 1 FROM visible v WHERE v.asset_id=$1 AND v.cloud_file_id=p.cloud_file_id)
 ORDER BY p.next_attempt_at,p.cloud_file_id LIMIT 1 FOR UPDATE OF p SKIP LOCKED)
 UPDATE media_probe_results p SET state='probing',probing_started_at=clock_timestamp(),lease_token=$2,
 lease_until=clock_timestamp()+$3::bigint*interval '1 millisecond',attempts=least(p.attempts,2147483646)+1,updated_at=now()
 FROM candidate c WHERE p.cloud_file_id=c.cloud_file_id RETURNING p.cloud_file_id::text,p.file_fingerprint,p.attempts`, assetID, l.Token, duration.Milliseconds()).Scan(&l.CloudFileID, &l.Fingerprint, &l.Attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// Succeed writes only typed whitelisted data, with ownership AND current-file
// checks. A changed file or expired/replaced lease returns false,nil (abandon).
func (r Repository) Succeed(parent context.Context, l Lease, result Result) (bool, error) {
	if r.DB == nil || !validLease(l) || result.SchemaVersion != probeinfo.SchemaVersion || result.Validate() != nil {
		return false, ErrInvalid
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	tag, err := r.DB.Exec(ctx, `WITH visible AS (`+catalog.ExposableAssetsSQL+`) UPDATE media_probe_results p SET state='ready',
 result=$6::jsonb,result_fingerprint=p.file_fingerprint,probed_at=clock_timestamp(),last_error_class=NULL,last_error_reason=NULL,
 probing_started_at=NULL,lease_until=NULL,lease_token=NULL,updated_at=now()
 WHERE p.cloud_file_id=$1 AND p.file_fingerprint=$2 AND p.lease_token=$3 AND p.attempts=$4 AND p.state='probing' AND p.lease_until>clock_timestamp()
 AND (SELECT enabled AND probe_enabled FROM native_server_settings WHERE singleton) AND EXISTS(SELECT 1 FROM visible v JOIN cloud_files f ON f.id=v.cloud_file_id WHERE v.asset_id=$5 AND v.cloud_file_id=p.cloud_file_id AND `+currentFingerprint+`=p.file_fingerprint)`, l.CloudFileID, l.Fingerprint, l.Token, l.Attempt, l.AssetID, raw)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
func (r Repository) Fail(parent context.Context, l Lease, f Failure) (bool, error) {
	if r.DB == nil || !validLease(l) || !f.valid() {
		return false, ErrInvalid
	}
	state := mediaprobe.ProbeFailed
	delay := time.Duration(0)
	if f.Class == linksource.Manual {
		state = mediaprobe.ProbeNeedsAction
	}
	if f.Class == linksource.Retryable {
		state = mediaprobe.ProbePending
		delay = Backoff(l.Attempt, f.RetryAfter)
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	tag, err := r.DB.Exec(ctx, `UPDATE media_probe_results SET state=$5,next_attempt_at=clock_timestamp()+$6::bigint*interval '1 millisecond',
 last_error_class=$7,last_error_reason=$8,probing_started_at=NULL,lease_until=NULL,lease_token=NULL,updated_at=now()
 WHERE cloud_file_id=$1 AND file_fingerprint=$2 AND lease_token=$3 AND attempts=$4 AND state='probing' AND lease_until>clock_timestamp()`, l.CloudFileID, l.Fingerprint, l.Token, l.Attempt, state, delay.Milliseconds(), f.Class, f.Reason)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
func (r Repository) RecoverExpired(parent context.Context) (int64, error) {
	if r.DB == nil {
		return 0, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	tag, err := r.DB.Exec(ctx, `WITH expired AS(SELECT cloud_file_id FROM media_probe_results WHERE state='probing' AND lease_until<=clock_timestamp()
 ORDER BY lease_until,cloud_file_id LIMIT 200 FOR UPDATE SKIP LOCKED)
 UPDATE media_probe_results p SET state='pending',next_attempt_at=clock_timestamp(),probing_started_at=NULL,lease_until=NULL,lease_token=NULL,
 last_error_class='retryable',last_error_reason='lease_expired',updated_at=now() FROM expired e WHERE p.cloud_file_id=e.cloud_file_id`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Current deliberately does NOT filter state='ready': pending/failed/manual
// attempts still retain the previous successful result and its own fingerprint.
func (r Repository) Current(parent context.Context, fileID string) (*Snapshot, error) {
	if r.DB == nil || !validID(fileID) {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	var s Snapshot
	var raw []byte
	err := r.DB.QueryRow(ctx, `SELECT result,result_fingerprint,probed_at FROM media_probe_results WHERE cloud_file_id=$1 AND result_fingerprint IS NOT NULL`, fileID).Scan(&raw, &s.Fingerprint, &s.ProbedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &s.Result); err != nil {
		return nil, ErrInvalid
	}
	return &s, nil
}
