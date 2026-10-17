package stillqueue

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/nativelibrary/catalog"
)

func (r *Repository) EnqueueAsset(parent context.Context, asset string) (bool, error) {
	if r == nil || r.db == nil || !validID(asset) {
		return false, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	var id int64
	err := r.db.QueryRow(ctx, `WITH eligible AS (`+catalog.ExposableAssetsSQL+`) SELECT n.id FROM eligible e
 CROSS JOIN LATERAL(SELECT coalesce(min(value::int),e.episode) first_episode FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(e.episodes)='array' THEN e.episodes ELSE '[]'::jsonb END) WHERE value~'^[0-9]{1,9}$') p
 JOIN native_item_ids n ON n.entity_type='episode' AND n.retired_at IS NULL AND n.internal_key='tmdb:'||e.tmdb_id||':s'||e.season||'e'||p.first_episode
 WHERE e.asset_id=$1 AND e.media_type='series' LIMIT 1`, asset).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return r.Enqueue(ctx, id)
}
func (r *Repository) ScheduleBatch(parent context.Context, after int64) (int64, int, error) {
	if r == nil || r.db == nil || after < 0 {
		return after, 0, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if _, err := r.RecoverExpired(ctx); err != nil {
		return after, 0, err
	}
	on, err := r.Enabled(ctx)
	if err != nil || !on {
		return after, 0, err
	}
	rows, err := r.db.Query(ctx, `SELECT id FROM native_item_ids WHERE entity_type='episode' AND retired_at IS NULL AND id>$1 ORDER BY id LIMIT 200`, after)
	if err != nil {
		return after, 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	err = errors.Join(err, rows.Err())
	rows.Close()
	if err != nil {
		return after, 0, err
	}
	count := 0
	for _, id := range ids {
		if _, err = r.Enqueue(ctx, id); err != nil {
			return after, count, err
		}
		after = id
		count++
	}
	return after, count, nil
}

// RetireSnapshots only removes references still explicitly skipped by metadata/
// eligibility. A concurrent fresh attempt keeps its old snapshot until replaced.
func (r *Repository) RetireSnapshots(parent context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	tag, err := r.db.Exec(ctx, `WITH chosen AS(SELECT item_id FROM media_stills WHERE state='skipped' AND last_error_reason IN('tmdb_still','source_unavailable') AND published_at IS NOT NULL LIMIT 200 FOR UPDATE SKIP LOCKED)
 UPDATE media_stills s SET relative_path='',digest='',width=0,height=0,size_bytes=0,result_asset_id=NULL,result_fingerprint=NULL,published_at=NULL,last_access_at=NULL
 FROM chosen c WHERE s.item_id=c.item_id`)
	return tag.RowsAffected(), err
}
