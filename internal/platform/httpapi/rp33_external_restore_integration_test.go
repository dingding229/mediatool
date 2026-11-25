package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/externalapi"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/maintenance"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/database"
)

func TestRP33ExternalSearchCannotCommitAcrossQueuedRestore(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test required")
	}
	if os.Getenv("MEDIA_TOOL_TEST_LOCKED") != "1" {
		t.Fatal("test-lock required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var dbname string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&dbname); err != nil || dbname != "media_tool_test" {
		t.Fatal("isolated database required")
	}
	keySigner, err := externalapi.NewSigner(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	api := &API{Deps: Dependencies{DB: db, ExternalSigner: keySigner}}
	key, err := api.apiKeys().Create(ctx, "RP33 fixture", []string{"search.run"}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	restore, backup, keyword, digest := jobs.UUID(), jobs.UUID(), jobs.UUID(), strings.Repeat("e", 64)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, step := range []struct{ sql, id string }{
			{`DELETE FROM jobs WHERE kind='search.run' AND payload->>'runId' IN (SELECT id::text FROM search_runs WHERE keyword=$1)`, keyword},
			{`DELETE FROM search_runs WHERE keyword=$1`, keyword},
			{`DELETE FROM audit_logs WHERE action='ops.web_restore.queued' AND details->>'id'=$1`, restore},
			{`DELETE FROM ops_web_restores WHERE id=$1`, restore},
			{`DELETE FROM ops_backups WHERE id=$1`, backup},
			{`DELETE FROM external_api_calls WHERE key_id=$1`, key.ID},
			{`DELETE FROM external_api_keys WHERE id=$1`, key.ID},
		} {
			if _, err := db.Exec(cleanup, step.sql, step.id); err != nil {
				t.Error(err)
			}
		}
	})
	if _, err = db.Exec(ctx, `INSERT INTO ops_backups(id,state,origin) VALUES($1,'ready','rp33-test')`, backup); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO ops_web_restores(id,backup_id,manifest_digest,protected) VALUES($1,$2,$3,'{}')`, restore, backup, digest); err != nil {
		t.Fatal(err)
	}

	entered, proceed, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := api.external("search.run", func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
			return
		case <-proceed:
		}
		id := jobs.UUID()
		err := func() error {
			ctx := r.Context()
			tx, err := db.Begin(ctx)
			if err != nil {
				return err
			}
			defer deferutil.Rollback(ctx, tx)
			if _, err = tx.Exec(ctx, `INSERT INTO search_runs(id,keyword,account_ids) VALUES($1,$2,'[]')`, id, keyword); err != nil {
				return err
			}
			return database.Commit(ctx, tx)
		}()
		if errors.Is(err, maintenance.ErrConflict) {
			maintenanceWritePaused(w)
			return
		}
		if err != nil {
			fail(w, 500, "fixture_failed", err.Error())
			return
		}
		write(w, 202, map[string]string{"id": id})
	})
	r := httptest.NewRequest("POST", "/api/external/v1/search", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+key.Token)
	w := httptest.NewRecorder()
	go func() { defer close(finished); h(w, r) }()
	defer func() { cancel(); <-finished }()
	select {
	case <-entered:
	case <-finished:
		t.Fatal("external request did not reach the test hook", w.Code, w.Body.String())
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	conn, err := db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	tryExclusive := func(want bool) {
		t.Helper()
		var locked bool
		if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(91220883)`).Scan(&locked); err != nil {
			t.Fatal(err)
		}
		if locked {
			if _, err := conn.Exec(ctx, `SELECT pg_advisory_unlock(91220883)`); err != nil {
				t.Fatal(err)
			}
		}
		if locked != want {
			t.Fatal("unexpected restore writer gate state", locked)
		}
	}
	tryExclusive(false)
	op := maintenance.Engine{DB: db}
	if queued, err := op.QueueWebRestore(ctx, restore, digest, "恢复 "+backup); err != nil || queued.State != "pending" {
		t.Fatal("could not queue restore", queued.State, err)
	}
	close(proceed)
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if w.Code != 423 || !strings.Contains(w.Body.String(), "maintenance_paused") {
		t.Fatal("expected explicit precommit rejection, never 202", w.Code, w.Body.String())
	}
	var exists bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM search_runs WHERE keyword=$1)`, keyword).Scan(&exists); err != nil || exists {
		t.Fatal("rejected request left a search receipt", err)
	}
	tryExclusive(true)
}
