package acquisition

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestEmbyAliasConfirmationRequiresEveryOwnedPathNotAssetCount(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated PG required")
	}
	cleanup := `TRUNCATE integrations,download_tasks,media_items,media_assets,storage_connections,cloud_files CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, cleanup)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{})
	defer m.Close()
	all := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Items" || r.Header.Get("X-Emby-Token") != "owned-token" || r.URL.Query().Has("SearchTerm") {
			t.Error("episode confirmation used fuzzy series-title search or wrong API")
		}
		rows := []map[string]any{{"Id": "owned-episode-1", "Path": "/strm/TV/Owned/Season 01/Owned - S01E01.strm", "MediaSources": []map[string]string{{"Id": "source-1", "Path": "https://fixed.fixture/source"}}}}
		if all {
			rows = append(rows, map[string]any{"Id": "owned-episode-3", "Path": "/strm/TV/Owned/Season 01/Owned - S01E03.strm", "MediaSources": []map[string]string{{"Id": "source-3", "Path": "https://fixed.fixture/source"}}})
		}
		json.NewEncoder(w).Encode(map[string]any{"Items": rows, "TotalRecordCount": len(rows)})
	}))
	defer server.Close()
	c, err := m.Repository.Save(ctx, "", integrations.Input{Kind: "emby", Name: "owned fixture", Enabled: true, Settings: map[string]any{"endpoint": server.URL, "strmPathPrefix": "/strm"}, Secrets: map[string]string{"apiKey": "owned-token"}})
	if err != nil {
		t.Fatal(err)
	}
	resource, task, file, storage, cloud, asset, media := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
	// Isolated confirmation-state fixture only, not a PT search/download claim.
	hint, _ := vault.Seal(resource, map[string]string{"url": "https://owned.fixture/torrent"})
	queries := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO resources(id,account_id,torrent_id,title,size_bytes,download_hint_ciphertext)VALUES($1,$2,'owned','Owned',1,$3)`, []any{resource, c.ID, hint}},
		{`INSERT INTO download_tasks(id,resource_id,integration_id,download_state,details)VALUES($1,$2,$3,'refresh_requested',$4)`, []any{task, resource, c.ID, func() []byte {
			raw, _ := json.Marshal(taskDetails{Media: metadata.Item{ID: 1, MediaType: "series", Title: "Owned"}})
			return raw
		}()}},
		{`INSERT INTO download_files(id,task_id,file_index,relative_path,size_bytes)VALUES($1,$2,0,'Owned.S01E01E03.mkv',1)`, []any{file, task}},
		{`INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, []any{storage}},
		{`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/115/owned',1)`, []any{cloud, storage}},
		{`INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, []any{asset, cloud}},
		{`INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',1,'Owned','{}')`, []any{media}},
		{`INSERT INTO media_asset_bindings(asset_id,media_id,download_file_id,season,episode,episodes)VALUES($1,$2,$3,1,1,'[1,3]')`, []any{asset, media, file}},
	}
	for _, q := range queries {
		if _, err = db.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"01", "03"} {
		if _, err = db.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,'owned-ref')`, jobs.UUID(), asset, "TV/Owned/Season 01/Owned - S01E"+n+".strm"); err != nil {
			t.Fatal(err)
		}
	}
	payload, _ := json.Marshal(map[string]string{"taskId": task})
	e := Engine{DB: db, Connections: m}
	job := jobs.Job{Payload: payload}
	if err = e.ConfirmEmby(ctx, job); err == nil {
		t.Fatal("one visible alias falsely completed entire import")
	}
	all = true
	if err = e.ConfirmEmby(ctx, job); err != nil {
		t.Fatal("both aliases did not complete import", err)
	}
	var count int
	db.QueryRow(ctx, `SELECT count(*) FROM emby_item_mappings WHERE asset_id=$1`, asset).Scan(&count)
	if count != 2 {
		t.Fatal("multiple Emby items not mapped to shared asset", count)
	}
	var state string
	db.QueryRow(ctx, `SELECT download_state FROM download_tasks WHERE id=$1`, task).Scan(&state)
	if state != "imported" {
		t.Fatal("alias pipeline not complete")
	}
}
