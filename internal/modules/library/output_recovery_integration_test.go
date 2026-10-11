package library

import (
	"context"
	"encoding/json"
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

func TestCandidateRecoveryDoesNotConfirmLegacySparseConsumerLayout(t *testing.T) {
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
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("isolated PG required")
	}
	cleanup := `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`
	db.Exec(ctx, cleanup)
	defer db.Exec(ctx, cleanup)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{STRMOutputDir: t.TempDir()})
	defer m.Close()
	storage, file, asset, media, candidate := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
	item := metadata.Item{ID: 10, MediaType: "series", Title: "Owned"}
	raw, _ := json.Marshal(item)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, []any{storage}},
		{`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/115/owned',1)`, []any{file, storage}},
		{`INSERT INTO media_assets(id,cloud_file_id,media_identity)VALUES($1,$2,$3)`, []any{asset, file, raw}},
		{`INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',10,'Owned',$2) ON CONFLICT(media_type,tmdb_id)DO UPDATE SET metadata=excluded.metadata RETURNING id`, []any{media, raw}},
	} {
		if strings.Contains(q.sql, "RETURNING") {
			if err = db.QueryRow(ctx, q.sql, q.args...).Scan(&media); err != nil {
				t.Fatal(err)
			}
		} else if _, err = db.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	db.Exec(ctx, `INSERT INTO media_asset_bindings(asset_id,media_id,season,episode,episodes)VALUES($1,$2,1,1,'[1,3]')`, asset, media)
	db.Exec(ctx, `INSERT INTO library_candidates(id,file_id,asset_id,state)VALUES($1,$2,$3,'artifacts_pending')`, candidate, file, asset)
	old := "TV/Owned/Season 01/Owned - S01E01E03.strm"
	db.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,'owned')`, jobs.UUID(), asset, old)
	j := Journal{DB: db, Vault: vault}
	if err = j.Write(ctx, m.Config.STRMOutputDir, strings.TrimSuffix(old, ".strm")+".nfo", asset, "asset:"+asset, "nfo", []byte("owned")); err != nil {
		t.Fatal(err)
	}
	e := Engine{DB: db, Connections: m}
	if err = e.FinalizeRecovered(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	db.QueryRow(ctx, `SELECT state FROM library_candidates WHERE id=$1`, candidate).Scan(&state)
	if state != "needs_output_reconciliation" {
		t.Fatal("old sparse chain marked confirmed", state)
	}
	list, err := e.CandidatePage(ctx, 50, 0, "needs_confirmation")
	if err != nil || len(list) != 1 || list[0].AssetID != asset {
		t.Fatal("blocked candidate hidden from operator", err)
	}
	db.Exec(ctx, `DELETE FROM strm_entries WHERE asset_id=$1`, asset)
	for _, tag := range []string{"S01E01", "S01E03"} {
		db.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,'owned')`, jobs.UUID(), asset, "TV/Owned/Season 01/Owned - "+tag+".strm")
	}
	for _, tag := range []string{"S01E01", "S01E03"} {
		p := "TV/Owned/Season 01/Owned - " + tag + ".strm"
		if err = j.Write(ctx, m.Config.STRMOutputDir, p, asset, "asset:"+asset, "strm", []byte("https://owned.fixture/fixed\n")); err != nil {
			t.Fatal(err)
		}
	}

	if err = e.FinalizeRecovered(ctx); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(ctx, `SELECT state FROM library_candidates WHERE id=$1`, candidate).Scan(&state)
	if state != "confirmed" {
		t.Fatal("corrected exact coverage could not finalize", state)
	}
}
