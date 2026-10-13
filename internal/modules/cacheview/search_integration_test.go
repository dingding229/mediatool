package cacheview

import (
	"context"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/cache"
	"media-tool/internal/platform/database"
	"os"
	"strings"
	"testing"
)

func TestSearchResultsVisibleWithoutRedisIndexAndScopedRemoval(t *testing.T) {
	dsn, u := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dsn == "" || !strings.HasSuffix(u, "/15") {
		t.Skip("isolated DB only")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, "SELECT current_database()").Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	rc, err := cache.Open(u)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	s := Store{DB: db, Redis: rc}
	id := jobs.UUID()
	defer db.Exec(ctx, `DELETE FROM search_runs WHERE id=$1`, id)
	_, err = db.Exec(ctx, `INSERT INTO search_runs(id,keyword,account_ids,state,media_context) VALUES($1,'Owned Cache','[]','partial','{"id":42,"posterPath":"/owned.jpg"}')`, id)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := s.Get(ctx, "search", id, true)
	if err != nil || entry.PosterPath != "/owned.jpg" || entry.State != "partial" || entry.Data == nil {
		t.Fatal(entry, err)
	}
	items, _, _, err := s.List(ctx, "search", 1, 100)
	found := false
	for _, x := range items {
		if x.ID == id {
			found = true
		}
	}
	if err != nil || !found {
		t.Fatal("partial search lost without Redis index", err)
	}
	if err = s.Delete(ctx, "search", id); err != nil {
		t.Fatal(err)
	}
	var kept bool
	db.QueryRow(ctx, `SELECT cache_hidden FROM search_runs WHERE id=$1`, id).Scan(&kept)
	if !kept {
		t.Fatal("record not hidden")
	}
	if _, err = s.Get(ctx, "search", id, true); err == nil {
		t.Fatal("hidden record exposed")
	}
}
