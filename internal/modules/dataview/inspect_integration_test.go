package dataview

import (
	"context"
	"encoding/json"
	"media-tool/internal/platform/cache"
	"media-tool/internal/platform/database"
	"os"
	"strings"
	"testing"
)

func TestPostgresAndRedisReadOnlyData(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated DB required")
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
		t.Fatal("test DB required")
	}
	e := Engine{DB: db}
	tables, err := e.Tables(ctx)
	if err != nil || len(tables) < 10 {
		t.Fatal(err)
	}
	result, err := e.Rows(ctx, "application_settings", 1, 25)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "readOnly") {
		t.Fatal(result)
	}
	for _, table := range []string{"administrator_accounts; DROP TABLE jobs", "pg_authid", "../secrets"} {
		if _, err = e.Rows(ctx, table, 1, 25); err == nil {
			t.Fatal("unknown table accepted")
		}
	}
	u := os.Getenv("TEST_REDIS_URL")
	if u == "" {
		t.Skip("Redis DB15 required")
	}
	if !strings.HasSuffix(u, "/15") {
		t.Fatal("Redis test DB15 required")
	}
	rc, err := cache.Open(u)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	e.Redis = rc
	key := "media-tool:owned-dataview-fixture"
	defer rc.Del(ctx, key)
	rc.Set(ctx, key, `{"title":"owned","apiKey":"hidden-key","url":"https://cdn.example/film?sign=private"}`, 0)
	value, err := e.RedisValue(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(value)
	if strings.Contains(string(raw), "hidden-key") || strings.Contains(string(raw), "sign=private") {
		t.Fatal("Redis leak")
	}
	if _, err = e.RedisKeys(ctx, "invalid", "*"); err == nil {
		t.Fatal("bad cursor")
	}
}
