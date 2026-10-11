package library

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/platform/artifacts"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPostgresArtifactJournalRecoveryAndForeignFileProtection(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("isolated test DB required")
	}
	cleanup := `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), cleanup)
	storage, file, asset, other := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
	for _, s := range []struct {
		sql  string
		args []any
	}{{`INSERT INTO storage_connections(id,name,provider)VALUES($1,'fixture','clouddrive2')`, []any{storage}}, {`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/115/file',1)`, []any{file, storage}}, {`INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2),($3,$2)`, []any{asset, file, other}}} {
		if _, err = db.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	vault, _ := secrets.New(strings.Repeat("a", 64))
	j := Journal{DB: db, Vault: vault}
	root := t.TempDir()
	content := []byte("SECRET-FILE-GRANT\n")
	tx, _ := db.Begin(ctx)
	_, err = j.Stage(ctx, tx, root, "Movie/file.strm", asset, "asset:"+asset, "strm", content)
	if err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	if _, err = os.Stat(filepath.Join(root, "Movie/file.strm")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rollback wrote filesystem")
	}
	tx, _ = db.Begin(ctx)
	id, err := j.Stage(ctx, tx, root, "Movie/file.strm", asset, "asset:"+asset, "strm", content)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var encrypted []byte
	db.QueryRow(ctx, `SELECT content_ciphertext FROM media_artifact_journal WHERE id=$1`, id).Scan(&encrypted)
	if strings.Contains(string(encrypted), "SECRET") {
		t.Fatal("secret stored raw")
	}
	// Simulate crash after rename but before manifest database commit.
	if err = artifacts.Write(root, "Movie/file.strm", content); err != nil {
		t.Fatal(err)
	}
	if err = j.Apply(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err = j.Apply(ctx, id); err != nil {
		t.Fatal("idempotent recovery", err)
	}
	changed := []byte("new grant\n")
	tx, _ = db.Begin(ctx)
	id, err = j.Stage(ctx, tx, root, "Movie/file.strm", asset, "asset:"+asset, "strm", changed)
	if err != nil {
		t.Fatal(err)
	}
	tx.Commit(ctx)
	if err = os.WriteFile(filepath.Join(root, "Movie/file.strm"), []byte("manual edit"), 0640); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(j.Apply(ctx, id), ErrArtifactConflict) {
		t.Fatal("foreign file overwritten")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "Movie/file.strm"))
	if string(raw) != "manual edit" {
		t.Fatal("manual edit lost")
	}
	os.Remove(filepath.Join(root, "Movie/file.strm"))
	if err = j.Apply(ctx, id); err != nil {
		t.Fatal(err)
	}
	tx, _ = db.Begin(ctx)
	_, err = j.Stage(ctx, tx, root, "Movie/file.strm", other, "asset:"+other, "strm", changed)
	tx.Rollback(ctx)
	if !errors.Is(err, ErrArtifactConflict) {
		t.Fatal("ownership changed")
	}
	// Shared show metadata writes are safe for multiple episode assets.
	season, episode := 1, 1
	show := metadata.Item{MediaType: "series", ID: 9, Title: "Owned show"}
	if err = WriteNFO(ctx, db, vault, root, "TV/Show/Season 01/E01.strm", asset, show, &season, &episode); err != nil {
		t.Fatal(err)
	}
	show.Overview = "updated"
	episode = 2
	if err = WriteNFO(ctx, db, vault, root, "TV/Show/Season 01/E02.strm", other, show, &season, &episode); err != nil {
		t.Fatal("shared tvshow rejected", err)
	}
	// Integrity check on the encrypted pending payload uses the same AAD.
	tx, _ = db.Begin(ctx)
	id, err = j.Stage(ctx, tx, root, "bad.nfo", asset, "asset:"+asset, "nfo", []byte("valid"))
	if err != nil {
		t.Fatal(err)
	}
	tx.Commit(ctx)
	wrong, _ := vault.Seal(id, map[string]string{"content": base64.StdEncoding.EncodeToString([]byte("tampered"))})
	db.Exec(ctx, `UPDATE media_artifact_journal SET content_ciphertext=$2 WHERE id=$1`, id, wrong)
	if err = j.Apply(ctx, id); err == nil {
		t.Fatal("corrupt journal accepted")
	}
	sum := sha256.Sum256(changed)
	var digest []byte
	db.QueryRow(ctx, `SELECT content_digest FROM managed_media_artifacts WHERE relative_path='Movie/file.strm'`).Scan(&digest)
	if string(digest) != string(sum[:]) {
		t.Fatal("wrong manifest digest")
	}
}

func TestArchiveJournalWaitsForRealNewOutputsAndRecoversRenameBeforeCommit(t *testing.T) {
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
	var databaseName string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&databaseName)
	if databaseName != "media_tool_test" {
		t.Fatal("isolated media_tool_test required")
	}
	db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`)
	defer db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`)
	storage, file, asset := jobs.UUID(), jobs.UUID(), jobs.UUID()
	db.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, storage)
	db.Exec(ctx, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/115/owned',1)`, file, storage)
	db.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, asset, file)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	j := Journal{DB: db, Vault: vault}
	root := t.TempDir()
	old := "Movies/old.strm"
	next := "TV/new.strm"
	content := []byte("owned fixed URL\n")
	if err = j.Write(ctx, root, old, asset, "asset:"+asset, "strm", content); err != nil {
		t.Fatal(err)
	}
	db.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,'owned-ref')`, jobs.UUID(), asset, old)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	write, err := j.Stage(ctx, tx, root, next, asset, "asset:"+asset, "strm", content)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := j.StageArchive(ctx, tx, root, old, asset, "strm", []string{write})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(j.Apply(ctx, archive), ErrArtifactDependency) {
		t.Fatal("old output archived before new write")
	}
	if _, err = os.Stat(filepath.Join(root, old)); err != nil {
		t.Fatal("old output vanished early")
	}
	if err = j.Apply(ctx, write); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, next), []byte("manual replacement"), 0640)
	if !errors.Is(j.Apply(ctx, archive), ErrArtifactDependency) {
		t.Fatal("database-applied but altered new output accepted")
	}
	os.WriteFile(filepath.Join(root, next), content, 0640)
	var archivePath string
	db.QueryRow(ctx, `SELECT archive_relative FROM media_artifact_journal WHERE id=$1`, archive).Scan(&archivePath)
	sum := sha256.Sum256(content)
	if err = archiveFile(root, old, archivePath, sum[:]); err != nil {
		t.Fatal(err)
	}
	// Process died after filesystem rename: source DB manifest/entry still exists.
	if err = j.Apply(ctx, archive); err != nil {
		t.Fatal("archive journal recovery", err)
	}
	if err = j.Apply(ctx, archive); err != nil {
		t.Fatal("archive replay", err)
	}
	var count int
	db.QueryRow(ctx, `SELECT count(*) FROM strm_entries WHERE asset_id=$1 AND output_path=$2`, asset, old).Scan(&count)
	if count != 0 {
		t.Fatal("old database entry survived committed archive")
	}
	saved, _ := os.ReadFile(filepath.Join(root, archivePath))
	if string(saved) != string(content) {
		t.Fatal("archive bytes not retained")
	}
}
