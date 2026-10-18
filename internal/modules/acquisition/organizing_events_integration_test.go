package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/library"
	dbutil "media-tool/internal/platform/database"
	"os"
	"path/filepath"
	"testing"
)

func TestTerminalOrganizingAndSubtitleFactsExcludeRetryUnknownAndRollback(t *testing.T) {
	f := newSourceFixture(t)
	ctx := f.ctx
	db := f.engine.DB
	var connection string
	if e := db.QueryRow(ctx, `SELECT integration_id::text FROM download_tasks WHERE id=$1`, f.taskID).Scan(&connection); e != nil {
		t.Fatal(e)
	}
	file, transfer := jobs.UUID(), jobs.UUID()
	queries := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO download_files(id,task_id,file_index,relative_path,size_bytes,local_path)VALUES($1,$2,0,'owned.mkv',1,'fixture')`, []any{file, f.taskID}},
		{`INSERT INTO transfer_tasks(id,download_file_id,integration_id,destination,device_id,state)VALUES($1,$2,$3,'/fixture','fixture','pending')`, []any{transfer, file, connection}},
		{`INSERT INTO storage_connections(id,name,provider)VALUES($1,'fixture','clouddrive2')`, []any{connection}},
		{`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/fixture/owned',1)`, []any{file, connection}},
		{`INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$1)`, []any{file}},
	}
	for _, q := range queries {
		if _, e := db.Exec(ctx, q.q, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	root := t.TempDir()
	journal := library.Journal{DB: db, Vault: f.engine.Connections.Repository.Vault}
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	intent, e := journal.Stage(ctx, tx, root, "owned.srt", file, "asset:"+file, "subtitle", []byte("expected"))
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "owned.srt"), []byte("foreign contents"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = journal.Apply(ctx, intent); !errors.Is(e, library.ErrArtifactConflict) {
		t.Fatal(e)
	}
	payload, _ := json.Marshal(map[string]string{"transferId": transfer})
	create := func(max int, transferState string) jobs.Job {
		t.Helper()
		id, token := jobs.UUID(), jobs.UUID()
		if _, e := db.Exec(ctx, `UPDATE transfer_tasks SET state=$2 WHERE id=$1`, transfer, transferState); e != nil {
			t.Fatal(e)
		}
		if _, e := db.Exec(ctx, `INSERT INTO jobs(id,kind,payload,state,attempts,max_attempts,lease_token,lease_until,locked_by)VALUES($1,'transfer.upload',$2,'running',1,$3,$4,now()+interval '1 minute','fixture')`, id, payload, max, token); e != nil {
			t.Fatal(e)
		}
		return jobs.Job{ID: id, Kind: "transfer.upload", Payload: payload, LeaseToken: token, Attempts: 1, MaxAttempts: max}
	}
	count := func(id string) int {
		t.Helper()
		var n int
		if e := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type IN('library.organize.failed','library.subtitles.failed') AND payload->>'operationId'=$1`, id).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	repo := jobs.Repository{DB: db}
	for _, tc := range []struct {
		max   int
		state string
		err   error
	}{{2, "artifacts_pending", errors.New("retry")}, {1, "uncertain", errors.New("unknown")}, {1, "starting", errors.New("unknown")}, {1, "artifacts_pending", jobs.ErrOperatorRequired}} {
		j := create(tc.max, tc.state)
		if e = repo.Finish(ctx, j, tc.err); e != nil {
			t.Fatal(e)
		}
		if count(j.ID) != 0 {
			t.Fatal("intermediate/unknown emitted terminal failure")
		}
	}
	j := create(1, "artifacts_pending")
	sentinel := errors.New("rollback terminal transaction")
	guarded := dbutil.WithCommitGuard(ctx, func(c context.Context, tx pgx.Tx) error {
		var n int
		if e := tx.QueryRow(c, `SELECT count(*) FROM outbox_events WHERE event_type IN('library.organize.failed','library.subtitles.failed') AND payload->>'operationId'=$1`, j.ID).Scan(&n); e != nil {
			return e
		}
		if n != 2 || count(j.ID) != 0 {
			t.Error("terminal event visible before commit", n)
		}
		return sentinel
	})
	if e = repo.Finish(guarded, j, errors.New("NEVER_PAYLOAD_SECRET")); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	if count(j.ID) != 0 {
		t.Fatal("rollback published terminal failure")
	}
	if e = repo.Finish(ctx, j, errors.New("NEVER_PAYLOAD_SECRET")); e != nil {
		t.Fatal(e)
	}
	if count(j.ID) != 2 {
		t.Fatal("missing proven terminal copy/organize failure")
	}
	if e = repo.Finish(ctx, j, errors.New("duplicate")); !errors.Is(e, jobs.ErrLeaseLost) {
		t.Fatal("stale finish replay", e)
	}
	if raw, e := os.ReadFile(filepath.Join(root, "owned.srt")); e != nil || string(raw) != "foreign contents" {
		t.Fatal("conflict protection lost", e)
	}
}
