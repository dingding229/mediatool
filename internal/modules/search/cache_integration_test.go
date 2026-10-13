package search

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/cache"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"strings"
	"testing"
)

func TestPostgresRedisSearchCacheScopeAndInvalidation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	u := os.Getenv("TEST_REDIS_URL")
	if dsn == "" || u == "" {
		t.Skip("isolated DB/Redis required")
	}
	if !strings.HasSuffix(u, "/15") {
		t.Fatal("Redis test DB15 required")
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
		t.Fatal("test DB")
	}
	db.Exec(ctx, "TRUNCATE integrations CASCADE")
	defer db.Exec(ctx, "TRUNCATE integrations CASCADE")
	rc, err := cache.Open(u)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	v, _ := secrets.New(strings.Repeat("4", 64))
	repo := integrations.Repository{DB: db, Vault: v}
	manager := integrations.NewManager(repo, config.Config{})
	defer manager.Close()
	_, err = db.Exec(ctx, `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition,source_repository)VALUES('owned-cache','Owned','NexusPHP','owned-commit','{}','{}','dingding229/mediatool-site') ON CONFLICT(id)DO UPDATE SET definition='{}',common_definition='{}'`)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM site_definitions WHERE id='owned-cache'`)
	account, err := repo.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "Owned cache", Enabled: true, Settings: map[string]any{"definitionId": "owned-cache", "endpoint": "https://owned-cache.example.invalid"}, Secrets: map[string]string{"cookie": "private-fixture-cookie"}})
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{DB: db, Vault: v, Connections: manager, Redis: rc}
	key, ttl, err := engine.cacheKey(ctx, "Owned Movie", []string{account.ID})
	if err != nil || ttl == 0 {
		t.Fatal(err)
	}
	defer rc.Del(ctx, key)
	run, hit, err := engine.StartCached(ctx, "Owned Movie", []string{account.ID}, false)
	if err != nil || hit {
		t.Fatal(run, hit, err)
	}
	db.Exec(ctx, `UPDATE search_runs SET state='succeeded',finished_at=now() WHERE id=$1`, run)
	db.Exec(ctx, `UPDATE search_site_results SET state='succeeded' WHERE run_id=$1`, run)
	engine.rememberCache(ctx, run)
	var stored string
	db.QueryRow(ctx, `SELECT cache_key FROM search_runs WHERE id=$1`, run).Scan(&stored)
	if stored != key {
		t.Fatal("cache key not persisted")
	}
	raw, _ := json.Marshal(account)
	if strings.Contains(string(raw), "private-fixture-cookie") || strings.Contains(key, "private-fixture-cookie") {
		t.Fatal("key leaks cookie")
	}
	// Empty positive results may be reused; Get's snapshot contract still decides validity.
	id, hit, err := engine.StartCached(ctx, "Owned Movie", []string{account.ID}, false)
	if err != nil {
		t.Fatal(err)
	}
	if id != run || !hit {
		t.Fatal("positive cache missed", id, hit)
	}
	db.Exec(ctx, `UPDATE search_runs SET state='partial' WHERE id=$1`, run)
	db.Exec(ctx, `UPDATE search_site_results SET state='partial',message='仅声明单页' WHERE run_id=$1`, run)
	engine.rememberCache(ctx, run)
	id, hit, err = engine.StartCached(ctx, "Owned Movie", []string{account.ID}, false)
	if err != nil || !hit || id != run {
		t.Fatal("declared partial result not cacheable", id, hit, err)
	}
	db.Exec(ctx, `UPDATE search_site_results SET state='failed' WHERE run_id=$1`, run)
	rc.Del(ctx, key)
	engine.rememberCache(ctx, run)
	if rc.Exists(ctx, key).Val() != 0 {
		t.Fatal("failed auth cached")
	}
	db.Exec(ctx, `UPDATE search_site_results SET state='partial' WHERE run_id=$1`, run)
	different, _, err := engine.cacheKey(ctx, "Another Movie", []string{account.ID})
	if err != nil || different == key {
		t.Fatal("query not partitioned")
	}
	db.Exec(ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, account.ID)
	changed, _, err := engine.cacheKey(ctx, "Owned Movie", []string{account.ID})
	if err != nil || changed == key {
		t.Fatal("revision not partitioned")
	}
	db.Exec(ctx, `UPDATE integrations SET enabled=false WHERE id=$1`, account.ID)
	if _, _, err = engine.StartCached(ctx, "Owned Movie", []string{account.ID}, false); err == nil {
		t.Fatal("disabled account cache bypass")
	}
}
