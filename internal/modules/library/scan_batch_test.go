package library

import (
	"context"
	"errors"
	"strings"
	"testing"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/storage"
)

func TestScanBatchFencesAndIdentityFallback(t *testing.T) {
	ctx, e, connection := scanWorkbenchFixture(t)
	if _, err := e.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'batch fixture','clouddrive2')`, connection); err != nil {
		t.Fatal(err)
	}
	scan, err := e.Start(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err = e.DB.QueryRow(ctx, `UPDATE library_scans SET state='running' WHERE id=$1 RETURNING integration_revision`, scan).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	// Long provider IDs cannot fit an ordinary btree; the expression index must
	// retain full-value comparisons, not make the hash an identity decision.
	file := storage.File{ID: strings.Repeat("provider", 1500), Name: "batch.mkv", Path: "/115/Media/batch.mkv", Size: 10, Hashes: map[uint32]string{2: strings.Repeat("a", 40)}}
	accepted := func(files []storage.File, want bool) {
		t.Helper()
		ok, err := e.indexNewBatch(ctx, scan, 1, connection, revision, files)
		if err != nil || ok != want {
			t.Fatal("batch", ok, err)
		}
	}
	accepted([]storage.File{file}, true)
	// Verify the fallback lookup can use the new fixed-width provider index.
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL enable_seqscan=off`); err != nil {
		t.Fatal(err)
	}
	var plan string
	if err = tx.QueryRow(ctx, `EXPLAIN (FORMAT JSON) SELECT id FROM cloud_files WHERE storage_id=$1 AND provider_file_id=$2 AND provider_file_id<>'' AND md5(provider_file_id)=md5($2)`, connection, file.ID).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "cloud_files_storage_provider_idx") {
		t.Fatal("provider identity index unavailable", plan)
	}

	var count int
	if err = e.DB.QueryRow(ctx, `SELECT count(*) FROM library_scan_seen WHERE scan_id=$1`, scan).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	accepted([]storage.File{file}, false)
	moved := file
	moved.Path = "/115/Media/moved.mkv"
	accepted([]storage.File{moved}, false)
	replacement := file
	replacement.ID = "replacement"
	accepted([]storage.File{replacement}, false)
	duplicate := file
	duplicate.Path = "/115/Media/duplicate.mkv"
	accepted([]storage.File{moved, duplicate}, false)
	// Existing files continue through the original move/replacement checks.
	var original string
	if err = e.DB.QueryRow(ctx, `SELECT id::text FROM cloud_files WHERE storage_id=$1 AND full_path=$2`, connection, file.Path).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if got, err := e.indexFile(ctx, connection, moved); err != nil || got != original {
		t.Fatal("move lost identity", got, err)
	}
	for _, change := range []struct {
		sql      string
		args     []any
		operator bool
	}{
		{`UPDATE library_scans SET generation=2 WHERE id=$1`, []any{scan}, true},
		{`UPDATE library_scans SET generation=1,state='paused' WHERE id=$1`, []any{scan}, true},
		{`UPDATE library_scans SET state='cancelled' WHERE id=$1`, []any{scan}, true},
		{`UPDATE library_scans SET state='running' WHERE id=$1`, []any{scan}, false},
	} {
		if _, err = e.DB.Exec(ctx, change.sql, change.args...); err != nil {
			t.Fatal(err)
		}
		if !change.operator {
			if _, err = e.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, connection); err != nil {
				t.Fatal(err)
			}
		}
		fresh := file
		fresh.ID = "new"
		fresh.Path = "/115/Media/new.mkv"
		ok, err := e.indexNewBatch(ctx, scan, 1, connection, revision, []storage.File{fresh})
		if ok || err == nil || change.operator && !errors.Is(err, jobs.ErrOperatorRequired) {
			t.Fatal("stale batch committed", ok, err)
		}
	}
	if err = e.DB.QueryRow(context.Background(), `SELECT count(*)FROM cloud_files WHERE full_path='/115/Media/new.mkv'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("stale write", count, err)
	}
	if scanMetadataBytes(storage.File{Hashes: map[uint32]string{2: strings.Repeat("a", scanBatchBytes)}}) <= scanBatchBytes {
		t.Fatal("byte budget ignores provider metadata")
	}
}
