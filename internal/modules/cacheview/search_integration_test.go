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

func TestSearchHistoryKeywordFiltersBeforePaginationAndTotal(t *testing.T) {
	dsn, u := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dsn == "" || !strings.HasSuffix(u, "/15") {
		t.Skip("isolated DB and Redis15 only")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated database required", err)
	}
	rc, err := cache.Open(u)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	s := Store{DB: db, Redis: rc}
	token := jobs.UUID()
	ids := []string{}
	for _, suffix := range []string{"匹配一", "匹配二", "other"} {
		id := jobs.UUID()
		ids = append(ids, id)
		if _, err = db.Exec(ctx, `INSERT INTO search_runs(id,keyword,account_ids,state,media_context)VALUES($1,$2,'[]','succeeded','{}')`, id, token+suffix); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, id := range ids {
			db.Exec(ctx, `DELETE FROM search_runs WHERE id=$1`, id)
		}
	}()
	first, total, _, err := s.ListFiltered(ctx, "search", 1, 1, " "+token+"匹配 ")
	if err != nil || total != 2 || len(first) != 1 {
		t.Fatal(first, total, err)
	}
	second, total, _, err := s.ListFiltered(ctx, "search", 2, 1, token+"匹配")
	if err != nil || total != 2 || len(second) != 1 || first[0].ID == second[0].ID {
		t.Fatal(second, total, err)
	}
	if _, err = db.Exec(ctx, `UPDATE search_runs SET cache_hidden=true WHERE id=$1`, first[0].ID); err != nil {
		t.Fatal(err)
	}
	rows, total, _, err := s.ListFiltered(ctx, "search", 1, 25, token+"匹配")
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatal(rows, total, err)
	}
	rows, total, _, err = s.ListFiltered(ctx, "search", 1, 25, token+"不存在")
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatal(rows, total, err)
	}
	if _, _, _, err = s.ListFiltered(ctx, "search", 1, 25, strings.Repeat("a", 801)); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, _, _, err = s.ListFiltered(ctx, "tmdb", 1, 25, "keyword"); err != ErrInvalid {
		t.Fatal(err)
	}
}
