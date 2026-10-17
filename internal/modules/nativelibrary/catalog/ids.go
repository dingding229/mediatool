package catalog

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/nativelibrary"
	"time"
)

type IDSQL interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}
type IDQueries struct{ DB IDSQL }

func (q IDQueries) Query(ctx context.Context, sql string, args ...any) (nativelibrary.IDRows, error) {
	if q.DB == nil {
		return nil, nativelibrary.ErrSettingsUnavailable
	}
	return q.DB.Query(ctx, sql, args...)
}

// RefreshIDLifecycle updates at most 500 allocated entities. It does not allocate
// IDs, delete mappings or retire an entity which still has any exposable asset.
// The returned cursor may restart at zero after process interruption (idempotent).
func RefreshIDLifecycle(ctx context.Context, db *pgxpool.Pool, after int64) (next int64, count int, err error) {
	if db == nil || after < 0 {
		return after, 0, nativelibrary.ErrInvalidKey
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := db.Query(ctx, `WITH batch AS(SELECT id,entity_type,internal_key FROM native_item_ids WHERE id>$1 AND entity_type<>'library' ORDER BY id LIMIT 500),
 live AS(`+ExposableAssetsSQL+`), keys AS(
 SELECT CASE WHEN media_type='movie' THEN 'movie' ELSE 'series' END AS entity_type,'tmdb:'||tmdb_id AS internal_key FROM live WHERE tmdb_id>0
 UNION SELECT 'season','tmdb:'||tmdb_id||':s'||season FROM live WHERE media_type='series' AND season>=0
 UNION SELECT 'episode','tmdb:'||tmdb_id||':s'||season||'e'||coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(episodes)),episode)
 FROM live WHERE media_type='series' AND season>=0),
 changed AS(UPDATE native_item_ids n SET retired_at=CASE WHEN EXISTS(SELECT 1 FROM keys k WHERE k.entity_type=n.entity_type AND k.internal_key=n.internal_key) THEN NULL ELSE coalesce(n.retired_at,now()) END
 FROM batch b WHERE n.id=b.id RETURNING n.id)
 SELECT id FROM changed ORDER BY id`, after)
	if err != nil {
		return after, 0, err
	}
	defer rows.Close()
	next = after
	for rows.Next() {
		if err = rows.Scan(&next); err != nil {
			return next, count, err
		}
		count++
	}
	return next, count, rows.Err()
}

// EnsureAssetIDs runs in the assignment transaction. A failed allocation rolls
// back the assignment, so the same native.assign job can safely retry.
func EnsureAssetIDs(ctx context.Context, q IDSQL, assetID string) error {
	if q == nil {
		return nativelibrary.ErrSettingsUnavailable
	}
	rows, err := q.Query(ctx, `WITH live AS (`+ExposableAssetsSQL+`) SELECT media_type,tmdb_id,season,
 coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(episodes)),episode) FROM live WHERE asset_id=$1`, assetID)
	if err != nil {
		return err
	}
	keys := []nativelibrary.Key{}
	for rows.Next() {
		var kind string
		var tmdb int64
		var season, episode *int
		if err := rows.Scan(&kind, &tmdb, &season, &episode); err != nil {
			rows.Close()
			return err
		}
		root, err := nativelibrary.MovieKey(tmdb)
		if kind == "series" {
			root, err = nativelibrary.SeriesKey(tmdb)
		}
		if err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, root)
		if kind == "series" && season != nil && *season >= 0 {
			key, err := nativelibrary.SeasonKey(tmdb, *season)
			if err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, key)
			if episode != nil && *episode > 0 {
				key, err := nativelibrary.EpisodeKey(tmdb, *season, *episode)
				if err != nil {
					rows.Close()
					return err
				}
				keys = append(keys, key)
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	_, err = nativelibrary.EnsureIDs(ctx, IDQueries{DB: q}, keys)
	return err
}

// PreallocateIDBatch is the periodic repair for pre-existing assignments. It is
// bounded and resumes by selecting only still-missing keys, without a global lock.
func PreallocateIDBatch(ctx context.Context, db *pgxpool.Pool) (int, error) {
	if db == nil {
		return 0, nativelibrary.ErrSettingsUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := db.Query(ctx, `WITH live AS (`+ExposableAssetsSQL+`),bound AS(
 SELECT e.*,coalesce((SELECT min(value::int) FROM jsonb_array_elements_text(episodes)),episode) first_episode FROM live e JOIN native_asset_libraries al ON al.asset_id=e.asset_id),keys AS(
 SELECT DISTINCT k.kind,'tmdb:'||e.tmdb_id||CASE WHEN k.kind IN('season','episode') THEN ':s'||e.season ELSE '' END||CASE WHEN k.kind='episode' THEN 'e'||e.first_episode ELSE '' END internal_key
 FROM bound e CROSS JOIN(VALUES('movie'),('series'),('season'),('episode')) k(kind)
 WHERE e.tmdb_id>0 AND (e.media_type='movie' AND k.kind='movie' OR e.media_type='series' AND k.kind<>'movie' AND (k.kind='series' OR e.season>=0 AND (k.kind='season' OR e.first_episode>0))))
 SELECT k.kind,k.internal_key FROM keys k LEFT JOIN native_item_ids n ON n.entity_type=k.kind AND n.internal_key=k.internal_key WHERE n.id IS NULL ORDER BY k.kind,k.internal_key LIMIT 500`)
	if err != nil {
		return 0, err
	}
	keys := []nativelibrary.Key{}
	for rows.Next() {
		var k nativelibrary.Key
		if err := rows.Scan(&k.EntityType, &k.InternalKey); err != nil {
			rows.Close()
			return 0, err
		}
		keys = append(keys, k)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if _, err := nativelibrary.EnsureIDs(ctx, IDQueries{DB: db}, keys); err != nil {
		return 0, err
	}
	return len(keys), nil
}
