package search

import (
	"context"
	"encoding/json"
	"errors"
)

// HostRun avoids GetFiltered's in-memory candidate scan and Get's unrestricted
// site-result collection. Query filtering/pagination remain in bounded SQL.
func (e Engine) HostRun(ctx context.Context, id string, page, limit int, query string) (Run, error) {
	var out Run
	if page < 1 || page > 1000 || limit < 1 || limit > 200 || len(query) > 800 {
		return out, errors.New("invalid page")
	}
	if err := e.DB.QueryRow(ctx, `SELECT id::text,keyword,state,created_at,finished_at,media_context FROM search_runs WHERE id=$1 AND NOT cache_hidden`, id).Scan(&out.ID, &out.Keyword, &out.State, &out.CreatedAt, &out.FinishedAt, &out.Media); err != nil {
		return out, err
	}
	if err := e.DB.QueryRow(ctx, `SELECT count(*)FROM search_run_resources WHERE run_id=$1 AND ($2='' OR public_snapshot->>'title' ILIKE '%'||$2||'%')`, id, query).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := e.DB.Query(ctx, `SELECT public_snapshot FROM search_run_resources WHERE run_id=$1 AND ($4='' OR public_snapshot->>'title' ILIKE '%'||$4||'%')ORDER BY resource_id LIMIT $2 OFFSET $3`, id, limit, (page-1)*limit, query)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.Resources = []Resource{}
	for rows.Next() {
		var raw []byte
		var r Resource
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		if err = json.Unmarshal(raw, &r); err != nil {
			return out, err
		}
		out.Resources = append(out.Resources, r)
	}
	return out, rows.Err()
}
