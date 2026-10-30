package search

import (
	"encoding/json"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/cache"
	"media-tool/internal/platform/config"
	"os"
	"strings"
	"testing"
)

func TestPostgresI29AliasSnapshotInvalidatesOldMediaCache(t *testing.T) {
	u := os.Getenv("TEST_REDIS_URL")
	if u == "" {
		t.Skip("isolated Redis required")
	}
	if !strings.HasSuffix(u, "/15") {
		t.Fatal("Redis DB15 only")
	}
	ctx, e, account, _ := snapshotSearchFixture(t)
	if _, err := e.DB.Exec(ctx, `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition,source_repository)VALUES('owned_snapshot','Owned','NexusPHP','fixture','{}','{}','dingding229/mediatool-site')`); err != nil {
		t.Fatal(err)
	}
	rc, err := cache.Open(u)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	e.Redis = rc
	manager := integrations.NewManager(integrations.Repository{DB: e.DB, Vault: e.Vault}, config.Config{})
	defer manager.Close()
	e.Connections = manager
	old := i29Media()
	old.CandidateTitles = nil
	old.IMDBID = ""
	id, hit, err := e.StartMediaCached(ctx, "嫁金钗", []string{account}, false, old)
	if err != nil || hit {
		t.Fatal(id, hit, err)
	}
	if _, err = e.DB.Exec(ctx, `UPDATE search_runs SET state='succeeded',finished_at=now() WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	e.DB.Exec(ctx, `UPDATE search_site_results SET state='succeeded' WHERE run_id=$1`, id)
	e.rememberCache(ctx, id)
	var key string
	e.DB.QueryRow(ctx, `SELECT cache_key FROM search_runs WHERE id=$1`, id).Scan(&key)
	defer rc.Del(ctx, key)
	again, hit, err := e.StartMediaCached(ctx, "嫁金钗", []string{account}, false, old)
	if err != nil || !hit || again != id {
		t.Fatal(again, hit, err)
	}
	fresh, hit, err := e.StartMediaCached(ctx, "嫁金钗", []string{account}, false, i29Media())
	if err != nil || hit || fresh == id {
		t.Fatal("old snapshot reused", fresh, hit, err)
	}
	var before, after []byte
	e.DB.QueryRow(ctx, `SELECT media_context FROM search_runs WHERE id=$1`, id).Scan(&before)
	e.DB.QueryRow(ctx, `SELECT media_context FROM search_runs WHERE id=$1`, fresh).Scan(&after)
	var snap map[string]any
	json.Unmarshal(after, &snap)
	if !strings.Contains(string(after), "Where the Mask Ends") || strings.Contains(string(before), "candidateTitles") || snap["imdbId"] != "tt42576855" {
		t.Fatal("snapshot not frozen separately")
	}
}
