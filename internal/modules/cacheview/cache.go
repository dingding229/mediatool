package cacheview

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"regexp"
	"sort"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid cache request")

type Store struct {
	Redis *redis.Client
	DB    *pgxpool.Pool
}
type Entry struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Bytes      int    `json:"bytes"`
	TTLSeconds int64  `json:"ttlSeconds"`
	Query      string `json:"query"`
	State      string `json:"state"`
	Count      int    `json:"count"`
	Data       any    `json:"data,omitempty"`
}

var hexID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func prefix(kind string) (string, error) {
	switch kind {
	case "tmdb":
		return "media-tool:tmdb-cache:", nil
	case "search":
		return "media-tool:search-cache:", nil
	}
	return "", ErrInvalid
}
func (s Store) keys(ctx context.Context, kind string) ([]string, bool, error) {
	p, e := prefix(kind)
	if e != nil {
		return nil, false, e
	}
	set := map[string]bool{}
	var cursor uint64
	for n := 0; n < 100; n++ {
		keys, next, e := s.Redis.Scan(ctx, cursor, p+"*", 200).Result()
		if e != nil {
			return nil, false, e
		}
		for _, k := range keys {
			if hexID.MatchString(strings.TrimPrefix(k, p)) {
				set[k] = true
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	out := []string{}
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, cursor != 0, nil
}
func (s Store) List(ctx context.Context, kind string, page, limit int) ([]Entry, int, bool, error) {
	if page < 1 || page > 10000 || limit < 1 || limit > 100 {
		return nil, 0, false, ErrInvalid
	}
	keys, truncated, e := s.keys(ctx, kind)
	if e != nil {
		return nil, 0, false, e
	}
	out := []Entry{}
	start := min(len(keys), (page-1)*limit)
	end := min(len(keys), start+limit)
	p, _ := prefix(kind)
	for _, k := range keys[start:end] {
		x, e := s.Get(ctx, kind, strings.TrimPrefix(k, p), false)
		if errors.Is(e, redis.Nil) {
			continue
		}
		if e != nil {
			return nil, 0, false, e
		}
		out = append(out, x)
	}
	return out, len(keys), truncated, nil
}
func (s Store) Get(ctx context.Context, kind, id string, detail bool) (Entry, error) {
	p, e := prefix(kind)
	if e != nil || !hexID.MatchString(id) {
		return Entry{}, ErrInvalid
	}
	key := p + id
	length, e := s.Redis.StrLen(ctx, key).Result()
	if e != nil {
		return Entry{}, e
	}
	ttl, e := s.Redis.TTL(ctx, key).Result()
	if e != nil {
		return Entry{}, e
	}
	if ttl == -2*time.Nanosecond {
		return Entry{}, redis.Nil
	}
	seconds := int64(ttl / time.Second)
	if ttl < 0 {
		seconds = -1
	}
	x := Entry{ID: id, Key: key, Bytes: int(length), TTLSeconds: seconds, State: "cached"}
	if length > 1<<20 {
		x.State = "too_large"
		return x, nil
	}
	raw, e := s.Redis.GetRange(ctx, key, 0, 1<<20).Bytes()
	if e != nil {
		return Entry{}, e
	}
	if len(raw) > 1<<20 {
		x.State = "too_large"
		return x, nil
	}

	if kind == "tmdb" {
		var data any
		if json.Unmarshal(raw, &data) != nil {
			x.State = "invalid"
			return x, nil
		}
		if m, ok := data.(map[string]any); ok {
			x.Query, _ = m["query"].(string)
			if items, ok := m["items"].([]any); ok {
				x.Count = len(items)
			}
			if m["item"] != nil {
				x.Count = 1
			}
		} else if items, ok := data.([]any); ok {
			x.Count = len(items)
			x.State = "legacy_cached"
			if len(items) > 0 {
				if first, ok := items[0].(map[string]any); ok {
					x.Query, _ = first["title"].(string)
				}
			}
		}
		if detail {
			x.Data = data
		}
	} else {
		runID := string(raw)
		var items []byte
		e = s.DB.QueryRow(ctx, `SELECT keyword,state,(SELECT count(*) FROM search_run_resources WHERE run_id=r.id),(SELECT coalesce(json_agg(public_snapshot),'[]') FROM (SELECT public_snapshot FROM search_run_resources WHERE run_id=r.id ORDER BY resource_id LIMIT 50) q) FROM search_runs r WHERE id::text=$1`, runID).Scan(&x.Query, &x.State, &x.Count, &items)
		if e != nil {
			x.State = "snapshot_unavailable"
		}
		if detail {
			var results any
			json.Unmarshal(items, &results)
			x.Data = map[string]any{"searchRunId": runID, "resources": results, "previewLimit": 50}
		}
	}
	return x, nil
}
func (s Store) Delete(ctx context.Context, kind, id string) error {
	p, e := prefix(kind)
	if e != nil || !hexID.MatchString(id) {
		return ErrInvalid
	}
	return s.Redis.Del(ctx, p+id).Err()
}
func (s Store) Clear(ctx context.Context, kind string) (int, bool, error) {
	keys, truncated, e := s.keys(ctx, kind)
	if e != nil {
		return 0, false, e
	}
	deleted := 0
	for i := 0; i < len(keys); i += 100 {
		n, e := s.Redis.Del(ctx, keys[i:min(len(keys), i+100)]...).Result()
		if e != nil {
			return deleted, true, e
		}
		deleted += int(n)
	}
	return deleted, truncated, nil
}
