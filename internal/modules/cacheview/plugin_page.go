package cacheview

import (
	"context"
	"strings"
	"time"
)

type SummaryEntry struct {
	ID, State         string
	Bytes, TTLSeconds int64
}

// PluginPage never GETs cached values, media payloads, download hints or queries.
func (s Store) PluginPage(ctx context.Context, kind string, page, limit int) ([]SummaryEntry, int, bool, error) {
	if page < 1 || page > 1000 || limit < 1 || limit > 200 {
		return nil, 0, false, ErrInvalid
	}
	if kind == "search" {
		var total int
		if e := s.DB.QueryRow(ctx, `SELECT count(*)FROM search_runs WHERE NOT cache_hidden`).Scan(&total); e != nil {
			return nil, 0, false, e
		}
		rows, e := s.DB.Query(ctx, `SELECT id::text,state FROM search_runs WHERE NOT cache_hidden ORDER BY created_at DESC,id LIMIT $1 OFFSET $2`, limit, (page-1)*limit)
		if e != nil {
			return nil, 0, false, e
		}
		defer rows.Close()
		out := []SummaryEntry{}
		for rows.Next() {
			var v SummaryEntry
			if e = rows.Scan(&v.ID, &v.State); e != nil {
				return nil, 0, false, e
			}
			out = append(out, v)
		}
		return out, total, false, rows.Err()
	}
	if kind != "tmdb" {
		return nil, 0, false, ErrInvalid
	}
	keys, truncated, e := s.keys(ctx, kind)
	if e != nil {
		return nil, 0, false, e
	}
	prefix, _ := prefix(kind)
	start := min(len(keys), (page-1)*limit)
	end := min(len(keys), start+limit)
	out := []SummaryEntry{}
	for _, k := range keys[start:end] {
		size, e := s.Redis.StrLen(ctx, k).Result()
		if e != nil {
			return nil, 0, false, e
		}
		ttl, e := s.Redis.TTL(ctx, k).Result()
		if e != nil {
			return nil, 0, false, e
		}
		if ttl == -2*time.Nanosecond {
			continue
		}
		out = append(out, SummaryEntry{ID: strings.TrimPrefix(k, prefix), State: "cached", Bytes: size, TTLSeconds: int64(ttl / time.Second)})
	}
	return out, len(keys), truncated, nil
}
