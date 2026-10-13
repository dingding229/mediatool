package cacheview

import (
	"context"
	"encoding/json"
	"regexp"
	"time"
)

var uuidID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func (s Store) searchList(ctx context.Context, page, limit int) ([]Entry, int, bool, error) {
	var total int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM search_runs WHERE NOT cache_hidden`).Scan(&total); err != nil {
		return nil, 0, false, err
	}
	rows, err := s.DB.Query(ctx, `SELECT id::text FROM search_runs WHERE NOT cache_hidden ORDER BY created_at DESC,id DESC LIMIT $1 OFFSET $2`, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, false, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, 0, false, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, false, err
	}
	out := []Entry{}
	for _, id := range ids {
		x, err := s.searchEntry(ctx, id, false)
		if err != nil {
			return nil, 0, false, err
		}
		out = append(out, x)
	}
	return out, total, false, nil
}
func (s Store) searchEntry(ctx context.Context, id string, detail bool) (Entry, error) {
	if !uuidID.MatchString(id) {
		return Entry{}, ErrInvalid
	}
	var x Entry
	var media []byte
	var key string
	err := s.DB.QueryRow(ctx, `SELECT id::text,keyword,state,created_at,media_context,coalesce(cache_key,''),(SELECT count(*) FROM search_run_resources WHERE run_id=r.id),(SELECT coalesce(sum(octet_length(public_snapshot::text)),0) FROM search_run_resources WHERE run_id=r.id) FROM search_runs r WHERE id=$1 AND NOT cache_hidden`, id).Scan(&x.ID, &x.Query, &x.State, &x.CreatedAt, &media, &key, &x.Count, &x.Bytes)
	if err != nil {
		return x, err
	}
	var context map[string]any
	json.Unmarshal(media, &context)
	x.PosterPath, _ = context["posterPath"].(string)
	if key != "" {
		if cached, err := s.Redis.Get(ctx, key).Result(); err == nil && cached == id {
			ttl := s.Redis.TTL(ctx, key).Val()
			if ttl > 0 {
				x.TTLSeconds = max(1, int64(ttl/time.Second))
				x.Reusable = true
			}
		}
	}
	if detail {
		rows, err := s.DB.Query(ctx, `SELECT public_snapshot FROM search_run_resources WHERE run_id=$1 ORDER BY resource_id LIMIT 50`, id)
		if err != nil {
			return x, err
		}
		defer rows.Close()
		items := []json.RawMessage{}
		for rows.Next() {
			var raw []byte
			if err = rows.Scan(&raw); err != nil {
				return x, err
			}
			items = append(items, json.RawMessage(raw))
		}
		if rows.Err() != nil {
			return x, rows.Err()
		}
		x.Data = map[string]any{"searchRunId": id, "media": context, "resources": items, "previewLimit": 50}
	}
	return x, nil
}
