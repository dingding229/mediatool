package probequeue

import (
	"context"
	"media-tool/internal/modules/nativelibrary/catalog"
	"time"
)

var ErrDisabled = disabledError{}

type disabledError struct{}

func (disabledError) Error() string { return "native probing disabled" }
func (r Repository) Enabled(parent context.Context) (bool, error) {
	if r.DB == nil {
		return false, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()
	var enabled bool
	err := r.DB.QueryRow(ctx, `SELECT enabled AND probe_enabled FROM native_server_settings WHERE singleton`).Scan(&enabled)
	return enabled, err
}

// At most 200 candidates per call; the caller retains the last successful UUID
// cursor. Recovery is DB-only and may also converge expired leases while off.
func (r Repository) ScheduleBatch(parent context.Context, after string) (string, int, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if _, err := r.RecoverExpired(ctx); err != nil {
		return after, 0, err
	}
	enabled, err := r.Enabled(ctx)
	if err != nil || !enabled {
		return after, 0, err
	}
	rows, err := r.DB.Query(ctx, `WITH visible AS (`+catalog.ExposableAssetsSQL+`) SELECT v.asset_id::text FROM visible v
 JOIN cloud_files f ON f.id=v.cloud_file_id LEFT JOIN media_probe_results p ON p.cloud_file_id=f.id
 WHERE v.asset_id::text>$1 AND (p.cloud_file_id IS NULL OR p.file_fingerprint<>`+currentFingerprint+` OR (p.state='pending' AND p.next_attempt_at<=clock_timestamp())
 OR (p.state='ready' AND coalesce(p.result->>'schema_version','0')='0'))
 ORDER BY v.asset_id LIMIT 200`, after)
	if err != nil {
		return after, 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return after, 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return after, 0, err
	}
	count := 0
	for _, id := range ids {
		if _, err := r.Enqueue(ctx, id); err != nil {
			return after, count, err
		}
		after = id
		count++
	}
	return after, count, nil
}
