package cacheview

import (
	"context"
	"encoding/json"
	"media-tool/internal/platform/cache"
	"media-tool/internal/platform/database"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresRedisCacheNamespacesTTLAndBoundedInspection(t *testing.T) {
	dsn, u := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dsn == "" || !strings.HasSuffix(u, "/15") {
		t.Skip("isolated PG and Redis15")
	}
	ctx := context.Background()
	db, e := database.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, "SELECT current_database()").Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	rc, e := cache.Open(u)
	if e != nil {
		t.Fatal(e)
	}
	defer rc.Close()
	s := Store{Redis: rc, DB: db}
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	keyA := "media-tool:tmdb-cache:" + a
	keyB := "media-tool:search-cache:" + b
	other := "media-tool:unrelated:owned"
	defer rc.Del(ctx, keyA, keyB, other)
	data, _ := json.Marshal(map[string]any{"query": "Owned", "items": []any{map[string]any{"title": "Owned Film"}}})
	rc.Set(ctx, keyA, data, time.Minute)
	rc.Set(ctx, keyB, "missing-run", time.Minute)
	rc.Set(ctx, other, "keep", time.Minute)
	entry, e := s.Get(ctx, "tmdb", a, true)
	if e != nil || entry.Query != "Owned" || entry.Count != 1 || entry.TTLSeconds < 50 || entry.Data == nil {
		t.Fatal(entry, e)
	}
	if e = s.Delete(ctx, "tmdb", "../../other"); e != ErrInvalid {
		t.Fatal(e)
	}
	if _, _, e = s.Clear(ctx, "other"); e != ErrInvalid {
		t.Fatal(e)
	}
	_, _, e = s.Clear(ctx, "tmdb")
	if e != nil {
		t.Fatal(e)
	}
	if rc.Exists(ctx, keyB, other).Val() != 2 {
		t.Fatal("cross-namespace deletion")
	}
	huge := strings.Repeat("x", (1<<20)+1)
	rc.Set(ctx, keyA, huge, time.Minute)
	entry, e = s.Get(ctx, "tmdb", a, true)
	if e != nil || entry.State != "too_large" || entry.Data != nil {
		t.Fatal(entry, e)
	}
}
