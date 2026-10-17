package catalog

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/mediaprobe/probeinfo"
	"media-tool/internal/modules/nativelibrary"
)

// Exactly one bounded read for this page, never one query per asset/source.
// Do not restrict state='ready': pending/failed attempts retain a usable snapshot.
func hydrateProbeSources(ctx context.Context, db *pgxpool.Pool, sources []*nativelibrary.MovieSource) error {
	if len(sources) == 0 {
		return nil
	}
	if len(sources) > 20000 {
		return ErrInvalid
	}
	ids := make([]string, 0, len(sources))
	seen := map[string]bool{}
	for _, s := range sources {
		if !seen[s.ID] {
			ids = append(ids, s.ID)
			seen[s.ID] = true
		}
	}
	rows, err := db.Query(ctx, `/* native.probe.page */ WITH snapshots AS (
 SELECT a.id,p.result,octet_length(p.result::text) bytes FROM media_assets a
 JOIN media_probe_results p ON p.cloud_file_id=a.cloud_file_id
 WHERE a.id=ANY($1::uuid[]) AND p.probed_at IS NOT NULL AND p.result<>'{}'::jsonb)
 SELECT id::text,CASE WHEN sum(bytes) OVER()<=33554432 THEN result ELSE NULL END FROM snapshots LIMIT 20000`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	results := map[string]*probeinfo.Result{}
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		// Over-budget pages fail atomically instead of transferring unbounded
		// snapshot JSON or silently dropping some versions. Total <=32 MiB.
		if raw == nil {
			return nativelibrary.ErrSettingsUnavailable
		}
		var result probeinfo.Result
		// Corrupt/incompatible snapshots degrade to pre-probe behavior, not a 500.
		if len(raw) <= 4<<20 && json.Unmarshal(raw, &result) == nil && result.Validate() == nil {
			results[id] = &result
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, s := range sources {
		s.Probe = results[s.ID]
	}
	return nil
}
