package library

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

type archiveFixture struct {
	ctx                                     context.Context
	db                                      *pgxpool.Pool
	e                                       Engine
	root, archive, archivePath, asset, next string
	raw                                     []byte
}

func newArchiveFixture(t *testing.T) archiveFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("isolated PG required")
	}
	db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`)
	t.Cleanup(func() { db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`) })
	vault, _ := secrets.New(strings.Repeat("a", 64))
	root := t.TempDir()
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{STRMOutputDir: root})
	t.Cleanup(m.Close)
	e := Engine{DB: db, Connections: m}
	j := Journal{DB: db, Vault: vault}
	storage, file, asset := jobs.UUID(), jobs.UUID(), jobs.UUID()
	db.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, storage)
	db.Exec(ctx, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/115/owned',1)`, file, storage)
	db.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, asset, file)
	raw := []byte("owned generated fixed URL\n")
	old, next := "Movies/old.strm", "TV/new.strm"
	if err = j.Write(ctx, root, old, asset, "asset:"+asset, "strm", raw); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	write, err := j.Stage(ctx, tx, root, next, asset, "asset:"+asset, "strm", raw)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := j.StageArchive(ctx, tx, root, old, asset, "strm", []string{write})
	if err != nil {
		t.Fatal(err)
	}
	tx.Commit(ctx)
	if err = j.Apply(ctx, write); err != nil {
		t.Fatal(err)
	}
	if err = j.Apply(ctx, archive); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,'fixture-key-reference')`, jobs.UUID(), asset, next); err != nil {
		t.Fatal(err)
	}
	var archivePath string
	db.QueryRow(ctx, `SELECT archive_relative FROM media_artifact_journal WHERE id=$1`, archive).Scan(&archivePath)
	return archiveFixture{ctx, db, e, root, archive, archivePath, asset, next, raw}
}
