package library

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/playbackgrants"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSTRMBatchRepairKeepsBodyAndGrantRejectsManualFiles(t *testing.T) {
	ctx, e, _ := scanWorkbenchFixture(t)
	root := t.TempDir()
	e.Connections.Config.STRMOutputDir = root
	e.Connections.Config.PublicURL = "https://owned.invalid"
	storage, file, asset := jobs.UUID(), jobs.UUID(), jobs.UUID()
	for _, s := range []struct {
		sql  string
		args []any
	}{{`INSERT INTO storage_connections(id,name,provider)VALUES($1,'Owned','clouddrive2')`, []any{storage}}, {`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/115/Media/Owned.mkv',1)`, []any{file, storage}}, {`INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, []any{asset, file}}} {
		if _, err := e.DB.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	grants := playbackgrants.Service{DB: e.DB, Vault: e.Connections.Repository.Vault, PublicURL: e.Connections.Config.PublicURL}
	link, id, err := grants.Prepare(ctx, tx, asset, "/115/Media/Owned.mkv")
	if err != nil {
		t.Fatal(err)
	}
	relative := "Movies/Owned/Owned.strm"
	_, err = tx.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,$4)`, jobs.UUID(), asset, relative, playbackgrants.SecretFreeKeyRef(id))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	j := Journal{DB: e.DB, Vault: e.Connections.Repository.Vault}
	body := []byte(link + "\n")
	if err = j.Write(ctx, root, relative, asset, "asset:"+asset, "strm", body); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "Movies/Owned/Owned.nfo"), []byte("owned NFO"), 0600); err != nil {
		t.Fatal(err)
	}
	var grantBefore string
	e.DB.QueryRow(ctx, `SELECT md5(to_jsonb(g)::text) FROM playback_grants g WHERE asset_id=$1`, asset).Scan(&grantBefore)
	batch, err := e.PreviewSTRMRepair(ctx, []string{asset})
	if err != nil || batch.Items[0].State != "ready" {
		t.Fatal(batch, err)
	}
	if err = os.Remove(filepath.Join(root, relative)); err != nil {
		t.Fatal(err)
	}
	batch, err = e.PreviewSTRMRepair(ctx, []string{asset})
	if err != nil || batch.Items[0].State != "missing" {
		t.Fatal(batch, err)
	}
	if _, err = e.ApproveSTRMRepair(ctx, batch.ID, batch.Digest, false); err == nil {
		t.Fatal("unconfirmed")
	}
	if _, err = e.ApproveSTRMRepair(ctx, batch.ID, strings.Repeat("a", 64), true); err == nil {
		t.Fatal("wrong digest")
	}
	batch, err = e.ApproveSTRMRepair(ctx, batch.ID, batch.Digest, true)
	if err != nil || batch.State != "running" {
		t.Fatal(batch, err)
	}
	payload, _ := json.Marshal(map[string]string{"batchId": batch.ID})
	if err = e.ProcessSTRMRepair(ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if err = e.ProcessSTRMRepair(ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, relative))
	if string(raw) != string(body) {
		t.Fatal("body changed")
	}
	var grantAfter string
	e.DB.QueryRow(ctx, `SELECT md5(to_jsonb(g)::text) FROM playback_grants g WHERE asset_id=$1`, asset).Scan(&grantAfter)
	if grantBefore != grantAfter {
		t.Fatal("grant rotated")
	}
	// Legacy erased journal can rebuild only when the existing grant body hash matches.
	e.DB.Exec(ctx, `UPDATE media_artifact_journal SET content_ciphertext=NULL WHERE asset_id=$1 AND state='applied'`, asset)
	os.Remove(filepath.Join(root, relative))
	batch, err = e.PreviewSTRMRepair(ctx, []string{asset})
	if err != nil || batch.Items[0].State != "missing" {
		t.Fatal(batch, err)
	}
	e.ApproveSTRMRepair(ctx, batch.ID, batch.Digest, true)
	os.WriteFile(filepath.Join(root, relative), []byte("manual content"), 0600)
	payload, _ = json.Marshal(map[string]string{"batchId": batch.ID})
	if err = e.ProcessSTRMRepair(ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(root, relative))
	if string(raw) != "manual content" {
		t.Fatal("manual overwritten")
	}
	batch, _ = e.STRMRepairBatch(ctx, batch.ID)
	if batch.State != "partial" || batch.Items[0].State != "blocked" {
		t.Fatal(batch)
	}
	nfo, _ := os.ReadFile(filepath.Join(root, "Movies/Owned/Owned.nfo"))
	if string(nfo) != "owned NFO" {
		t.Fatal("NFO changed")
	}
	// Symlinks and missing original evidence never become write targets.
	os.Remove(filepath.Join(root, relative))
	os.Symlink("Owned.nfo", filepath.Join(root, relative))
	batch, err = e.PreviewSTRMRepair(context.Background(), []string{asset})
	if err != nil || batch.Items[0].State != "blocked" {
		t.Fatal(batch, err)
	}
}
