package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/integrations"
	dbutil "media-tool/internal/platform/database"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestTorrentOnlyDeletionReadbackCommitRollbackNeverDeletesFiles(t *testing.T) {
	for _, mode := range []string{"unknown", "rollback", "confirmed"} {
		t.Run(mode, func(t *testing.T) {
			f := newSourceFixture(t)
			ctx := f.ctx
			db := f.engine.DB
			var present atomic.Bool
			present.Store(true)
			var deletes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/auth/login":
					w.Write([]byte("Ok."))
				case "/api/v2/app/version":
					w.Write([]byte("v5.0"))
				case "/api/v2/torrents/info":
					if !present.Load() {
						w.Write([]byte("[]"))
						return
					}
					json.NewEncoder(w).Encode([]map[string]any{{"hash": f.hash, "name": "fixture", "state": "stalledUP", "progress": 1, "save_path": "/downloads", "category": "media-tool", "tags": f.taskID}})
				case "/api/v2/torrents/delete":
					r.ParseForm()
					if r.Form.Get("deleteFiles") != "false" {
						t.Error("payload deletion requested")
					}
					deletes.Add(1)
					if mode != "unknown" {
						present.Store(false)
					}
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			var cid string
			if e := db.QueryRow(ctx, `SELECT integration_id::text FROM download_tasks WHERE id=$1`, f.taskID).Scan(&cid); e != nil {
				t.Fatal(e)
			}
			old, e := f.engine.Connections.Repository.Get(ctx, cid)
			if e != nil {
				t.Fatal(e)
			}
			_, e = f.engine.Connections.Repository.Save(ctx, cid, integrations.Input{Kind: "qbittorrent", Name: "fixture", Enabled: true, Revision: old.Record.Revision, Settings: map[string]any{"endpoint": server.URL, "downloadPath": "/downloads", "localPath": t.TempDir(), "category": "media-tool"}, Secrets: map[string]string{"username": "fixture", "password": "fixture"}})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = db.Exec(ctx, `UPDATE download_tasks SET info_hash=$3,download_state='imported',details=details||jsonb_build_object('endpoint',$2::text,'category','media-tool')WHERE id=$1`, f.taskID, server.URL, f.hash); e != nil {
				t.Fatal(e)
			}
			count := func() int {
				t.Helper()
				var n int
				if e := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type IN('download.task.deleted','download.files.deleted')AND payload->>'taskId'=$1`, f.taskID).Scan(&n); e != nil {
					t.Fatal(e)
				}
				return n
			}
			callCtx := ctx
			sentinel := errors.New("reject deletion final commit")
			if mode == "rollback" {
				callCtx = dbutil.WithCommitGuard(ctx, func(c context.Context, tx pgx.Tx) error {
					var n int
					if e := tx.QueryRow(c, `SELECT count(*)FROM outbox_events WHERE event_type='download.task.deleted'AND payload->>'taskId'=$1`, f.taskID).Scan(&n); e != nil {
						return e
					}
					if n != 1 || count() != 0 {
						t.Error("fact visible before commit")
					}
					return sentinel
				})
			}
			svc := DownloaderService{Engine: f.engine}
			e = svc.DeleteOwned(callCtx, f.taskID, false, func() error { return nil })
			if mode == "confirmed" {
				if e != nil || count() != 1 {
					t.Fatal("confirmed fact missing", e, count())
				}
			} else {
				if e == nil || count() != 0 {
					t.Fatal("unknown/rollback emitted success", e, count())
				}
			}
			if deletes.Load() != 1 {
				t.Fatal("unexpected deletion replay", deletes.Load())
			}
			var files int
			if e = db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type='download.files.deleted'AND payload->>'taskId'=$1`, f.taskID).Scan(&files); e != nil || files != 0 {
				t.Fatal("deleteFiles=false emitted file deletion", files, e)
			}
		})
	}
}
func TestConfirmedPayloadDeletionCommitRollback(t *testing.T) {
	f := newSourceFixture(t)
	ctx := f.ctx
	db := f.engine.DB
	if _, e := db.Exec(ctx, `INSERT INTO download_cleanup_requests(task_id,state,evidence)VALUES($1,'deleting','{}')`, f.taskID); e != nil {
		t.Fatal(e)
	}
	sentinel := errors.New("reject verified cleanup commit")
	guard := dbutil.WithCommitGuard(ctx, func(c context.Context, tx pgx.Tx) error {
		var n int
		if e := tx.QueryRow(c, `SELECT count(*) FROM outbox_events WHERE event_type IN('download.task.deleted','download.files.deleted')AND payload->>'taskId'=$1`, f.taskID).Scan(&n); e != nil {
			return e
		}
		if n != 2 {
			t.Error(n)
		}
		return sentinel
	})
	if e := f.engine.finishCleanupConfirmed(guard, f.taskID); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	var n int
	if e := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type IN('download.task.deleted','download.files.deleted')AND payload->>'taskId'=$1`, f.taskID).Scan(&n); e != nil || n != 0 {
		t.Fatal("rollback escaped", n, e)
	}
	if e := f.engine.finishCleanupConfirmed(ctx, f.taskID); e != nil {
		t.Fatal(e)
	}
	if e := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type IN('download.task.deleted','download.files.deleted')AND payload->>'taskId'=$1`, f.taskID).Scan(&n); e != nil || n != 2 {
		t.Fatal(n, e)
	}
}
