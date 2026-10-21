package catalog

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/nativelibrary"
)

type episodeImages struct{ Tag, Still, Poster string }

// One bounded query per page. No file, queue or process dependencies in catalog.
func episodeImagePage(ctx context.Context, db *pgxpool.Pool, ids []string) (map[string]episodeImages, error) {
	out := map[string]episodeImages{}
	if len(ids) == 0 {
		return out, nil
	}
	if len(ids) > 200 {
		return nil, ErrInvalid
	}
	numbers := make([]int64, len(ids))
	for i, id := range ids {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, ErrInvalid
		}
		numbers[i] = n
	}
	rows, err := db.Query(ctx, `/* native.still.page */
 WITH eligible AS (`+ExposableAssetsSQL+`),
 wanted AS MATERIALIZED (
 SELECT id,regexp_match(internal_key,'^tmdb:([0-9]{1,19}):s([0-9]{1,9})e([0-9]{1,9})$') p
 FROM native_item_ids WHERE id=ANY($1::bigint[]) AND entity_type='episode'),
 lang AS(SELECT coalesce((SELECT nullif(settings->>'language','') FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN') v),
 image_assets AS MATERIALIZED (
 SELECT q.asset_id,q.created_at,q.tmdb_id,q.season,
 coalesce((SELECT min(ev.value::int) FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(q.episodes)='array' THEN q.episodes ELSE '[]'::jsonb END) AS ev(value) WHERE ev.value~'^[0-9]{1,9}$'),q.episode) first_episode,
 CASE WHEN jsonb_typeof(q.asset_identity->'episodeDetails')='array' THEN q.asset_identity->'episodeDetails' ELSE '[]'::jsonb END episode_details
 FROM eligible q WHERE q.media_type='series'
 AND EXISTS(SELECT 1 FROM wanted w WHERE q.tmdb_id=CASE WHEN w.p[1]::numeric BETWEEN 1 AND 9223372036854775807 THEN w.p[1]::bigint END AND q.season=w.p[2]::int)
 AND EXISTS(SELECT 1 FROM native_asset_libraries al JOIN native_libraries l ON l.id=al.library_id WHERE al.asset_id=q.asset_id AND NOT l.hidden)),
 ranked AS MATERIALIZED (
 SELECT w.id,w.p,a.episode_details,row_number() OVER(PARTITION BY w.id ORDER BY a.created_at DESC,a.asset_id) rn
 FROM image_assets a JOIN wanted w ON a.tmdb_id=CASE WHEN w.p[1]::numeric BETWEEN 1 AND 9223372036854775807 THEN w.p[1]::bigint END
 AND a.season=w.p[2]::int AND a.first_episode=w.p[3]::int),
 asset_images AS MATERIALIZED (
 SELECT DISTINCT ON(a.id) a.id,ep->>'stillPath' still FROM ranked a
 CROSS JOIN LATERAL jsonb_array_elements(a.episode_details) ep
 WHERE a.rn<=100 AND ep->>'season'=a.p[2] AND ep->>'number'=a.p[3] AND coalesce(ep->>'stillPath','')<>''
 ORDER BY a.id,a.rn)
 SELECT w.id,coalesce(s.result_fingerprint,''),coalesce(s.digest,''),
 coalesce(nullif(e.payload->>'stillPath',''),(SELECT ep->>'stillPath' FROM jsonb_array_elements(CASE WHEN jsonb_typeof(d.payload->'episodes')='array' THEN d.payload->'episodes' ELSE '[]'::jsonb END) ep WHERE ep->>'number'=w.p[3] AND coalesce(ep->>'stillPath','')<>'' LIMIT 1),a.still,''),coalesce(d.payload->>'posterPath','')
 FROM wanted w CROSS JOIN lang LEFT JOIN media_stills s ON s.item_id=w.id AND s.published_at IS NOT NULL
 LEFT JOIN native_tmdb_details e ON e.tmdb_type='episode' AND e.tmdb_id=CASE WHEN w.p[1]::numeric BETWEEN 1 AND 9223372036854775807 THEN w.p[1]::bigint END AND e.season_number=w.p[2]::int AND e.episode_number=w.p[3]::int AND e.language=lang.v AND e.fetched_at IS NOT NULL
 LEFT JOIN native_tmdb_details d ON d.tmdb_type='season' AND d.tmdb_id=CASE WHEN w.p[1]::numeric BETWEEN 1 AND 9223372036854775807 THEN w.p[1]::bigint END AND d.season_number=w.p[2]::int AND d.language=lang.v AND d.fetched_at IS NOT NULL
 LEFT JOIN asset_images a ON a.id=w.id LIMIT 200`, numbers)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var fp, digest string
		var image episodeImages
		if err = rows.Scan(&id, &fp, &digest, &image.Still, &image.Poster); err != nil {
			return nil, err
		}
		image.Tag = nativelibrary.StillImageTag(fp, digest)
		out[strconv.FormatInt(id, 10)] = image
	}
	return out, rows.Err()
}
