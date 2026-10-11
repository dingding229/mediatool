package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

type retryFixture struct {
	ctx             context.Context
	engine          Engine
	record          Record
	runID, accountA string
	payload         []byte
	setPhase        func(int)
}

func retryFixtureFor(t *testing.T) *retryFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated media_tool_test required", err)
	}
	cleanup := `TRUNCATE subscriptions,integrations,resources,site_definitions,search_runs,download_tasks,outbox_events,jobs,media_items CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(context.Background(), cleanup) })
	var mu sync.Mutex
	phase := 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		current := phase
		mu.Unlock()
		if r.URL.Path == "/b" {
			if current == 1 {
				fmt.Fprint(w, `<html><body>Login</body></html>`)
			} else {
				fmt.Fprint(w, `<rss><channel/></rss>`)
			}
			return
		}
		if r.URL.Path == "/a" {
			id := "1"
			if current > 1 {
				id = "2"
			}
			fmt.Fprintf(w, `<rss><channel><item><title>Owned Movie (2026)</title><link>/download.php?id=%s</link></item></channel></rss>`, id)
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(server.Close)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	manager := integrations.NewManager(repo, config.Config{})
	t.Cleanup(func() { manager.Close() })
	manager.OutboundHTTP = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":1,"title":"Owned Movie","original_title":"Owned Movie","release_date":"2026-01-01"}`))}, nil
	})}
	account := func(path string) string {
		t.Helper()
		r, err := repo.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "Owned " + path, Enabled: true, Settings: map[string]any{"endpoint": server.URL, "definitionId": "owned_retry"}, Secrets: map[string]string{"rssUrl": server.URL + "/" + path}})
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	a, b := account("a"), account("b")
	if _, err = repo.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "Owned metadata", Enabled: true, Secrets: map[string]string{"token": "fixture"}}); err != nil {
		t.Fatal(err)
	}
	definition := fmt.Sprintf(`{"id":"owned_retry","name":"Owned","schema":"NexusPHP","domain":%q,"meta":{"has_hr":false},"requests":{}}`, server.URL)
	if _, err = db.Exec(ctx, `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition)VALUES('owned_retry','Owned','NexusPHP','fixture',$1,'{}')`, json.RawMessage(definition)); err != nil {
		t.Fatal(err)
	}
	engine := Engine{DB: db, Connections: manager}
	record, err := engine.Save(ctx, "", Input{MediaType: "movie", TMDBID: 1, Source: "rss", Enabled: true, IntervalMinutes: 30, Rules: Rules{AccountIDs: []string{a, b}, Keyword: "Owned Movie", IncludeHistory: true, MinSeeders: 1}})
	if err != nil {
		t.Fatal(err)
	}
	runID, err := engine.Queue(ctx, record.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"runId": runID})
	return &retryFixture{ctx: ctx, engine: engine, record: record, runID: runID, payload: payload, accountA: a, setPhase: func(value int) { mu.Lock(); phase = value; mu.Unlock() }}
}
func TestPostgresMultisiteRSSRetryCannotUnionOldFeedOrAdvanceItsSeenDigest(t *testing.T) {
	f := retryFixtureFor(t)
	ctx, engine, record, runID, payload, a := f.ctx, f.engine, f.record, f.runID, f.payload, f.accountA
	db := engine.DB
	var err error

	if err = engine.Process(ctx, jobs.Job{Payload: payload, Attempts: 1, MaxAttempts: 5}); !errors.Is(err, sites.ErrFeed) {
		t.Fatal("second account did not fail first collection", err)
	}
	var baseline int
	if err = db.QueryRow(ctx, `SELECT count(*)FROM subscription_rss_accounts WHERE subscription_id=$1`, record.ID).Scan(&baseline); err != nil || baseline != 0 {
		t.Fatal("failed multi-site collection committed baseline", baseline, err)
	}
	f.setPhase(2)
	if err = engine.Process(ctx, jobs.Job{Payload: payload, Attempts: 2, MaxAttempts: 5}); err != nil {
		t.Fatal(err)
	}
	run, err := engine.Run(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Decisions) != 1 {
		t.Fatal("retry evaluated a union of first attempt and refreshed feed", len(run.Decisions))
	}
	var tid string
	if err = db.QueryRow(ctx, `SELECT r.torrent_id FROM subscription_decisions d JOIN resources r ON r.id=d.resource_id WHERE d.run_id=$1`, runID).Scan(&tid); err != nil || tid != "2" {
		t.Fatal("retry evaluated removed item", err)
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT s.result_count FROM search_site_results s JOIN subscription_runs r ON r.search_run_id=s.run_id WHERE r.id=$1 AND s.account_id=$2`, runID, a).Scan(&count); err != nil || count != 1 {
		t.Fatal("RSS retry accumulated account count", count, err)
	}
	client, err := engine.Connections.Site(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	f.setPhase(1)
	oldItems, err := client.RSS(ctx, client.Account.Endpoint+"/a")
	if err != nil || len(oldItems) != 1 {
		t.Fatal(err)
	}
	var oldSeen bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_rss_seen WHERE subscription_id=$1 AND account_id=$2 AND guid_digest=$3)`, record.ID, a, oldItems[0].Digest).Scan(&oldSeen); err != nil || oldSeen {
		t.Fatal("retry advanced baseline for a removed, uncollected item", err)
	}
}

func TestPostgresCollectedRSSRetryKeepsFrozenItemsAfterDecisionStageFailure(t *testing.T) {
	f := retryFixtureFor(t)
	ctx, e, runID := f.ctx, f.engine, f.runID
	f.setPhase(2)
	// All feeds are valid and collection is durably frozen before coverage fails.
	if _, err := e.DB.Exec(ctx, `ALTER TABLE media_asset_bindings RENAME TO fixture_unavailable_bindings`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.DB.Exec(context.Background(), `ALTER TABLE IF EXISTS fixture_unavailable_bindings RENAME TO media_asset_bindings`)
	})
	if err := e.Process(ctx, jobs.Job{Payload: f.payload, Attempts: 1, MaxAttempts: 5}); err == nil {
		t.Fatal("coverage failure did not interrupt evaluation")
	}
	var phase string
	if err := e.DB.QueryRow(ctx, `SELECT collection_state FROM subscription_runs WHERE id=$1`, runID).Scan(&phase); err != nil || phase != "collected" {
		t.Fatal("complete collection not persisted", phase, err)
	}
	if _, err := e.DB.Exec(ctx, `ALTER TABLE fixture_unavailable_bindings RENAME TO media_asset_bindings`); err != nil {
		t.Fatal(err)
	}
	// Both feed responses would now be different/invalid if the retry refetched.
	f.setPhase(1)
	if err := e.Process(ctx, jobs.Job{Payload: f.payload, Attempts: 2, MaxAttempts: 5}); err != nil {
		t.Fatal("collected retry refetched or mixed new feed", err)
	}
	var tid string
	if err := e.DB.QueryRow(ctx, `SELECT r.torrent_id FROM subscription_decisions d JOIN resources r ON r.id=d.resource_id WHERE d.run_id=$1`, runID).Scan(&tid); err != nil || tid != "2" {
		t.Fatal("frozen item changed", tid, err)
	}
}

func TestPostgresChangedEmptyFeedCannotInitializeBaselineFromOldCredentials(t *testing.T) {
	f := retryFixtureFor(t)
	ctx, e := f.ctx, f.engine
	f.setPhase(2)
	if _, err := e.DB.Exec(ctx, `ALTER TABLE media_asset_bindings RENAME TO fixture_unavailable_bindings`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.DB.Exec(context.Background(), `ALTER TABLE IF EXISTS fixture_unavailable_bindings RENAME TO media_asset_bindings`)
	})
	if err := e.Process(ctx, jobs.Job{Payload: f.payload, Attempts: 1, MaxAttempts: 5}); err == nil {
		t.Fatal("coverage failure missing")
	}
	if _, err := e.DB.Exec(ctx, `ALTER TABLE fixture_unavailable_bindings RENAME TO media_asset_bindings`); err != nil {
		t.Fatal(err)
	}
	emptyAccount := f.record.Rules.AccountIDs[1]
	if _, err := e.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, emptyAccount); err != nil {
		t.Fatal(err)
	}
	if err := e.Process(ctx, jobs.Job{Payload: f.payload, Attempts: 2, MaxAttempts: 5}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal("changed empty source was not paused for a fresh evaluation", err)
	}
	var count int
	if err := e.DB.QueryRow(ctx, `SELECT count(*) FROM subscription_rss_accounts WHERE subscription_id=$1`, f.record.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("changed source initialized baseline", count, err)
	}
	if err := e.Process(ctx, jobs.Job{Payload: f.payload, Attempts: 3, MaxAttempts: 5}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal("generic retry bypassed needs-refresh state", err)
	}
}

func TestPostgresBothModeSearchFailureDoesNotHideEmptyRSSSourceRevision(t *testing.T) {
	f := retryFixtureFor(t)
	ctx, e := f.ctx, f.engine
	f.setPhase(2)
	if _, err := e.DB.Exec(ctx, `UPDATE subscriptions SET source='both' WHERE id=$1`, f.record.ID); err != nil {
		t.Fatal(err)
	}
	// The fixture has no search request: search must stay visibly failed, while
	// /a and the empty /b RSS sources are fetched and frozen before coverage fails.
	if _, err := e.DB.Exec(ctx, `ALTER TABLE media_asset_bindings RENAME TO fixture_unavailable_bindings`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.DB.Exec(context.Background(), `ALTER TABLE IF EXISTS fixture_unavailable_bindings RENAME TO media_asset_bindings`)
	})
	if err := e.Process(ctx, jobs.Job{Payload: f.payload, Attempts: 1, MaxAttempts: 5}); err == nil {
		t.Fatal("fixture coverage failure missing")
	}
	if _, err := e.DB.Exec(ctx, `ALTER TABLE fixture_unavailable_bindings RENAME TO media_asset_bindings`); err != nil {
		t.Fatal(err)
	}
	empty := f.record.Rules.AccountIDs[1]
	var state string
	var observed *time.Time
	if err := e.DB.QueryRow(ctx, `SELECT s.state,s.observed_at FROM search_site_results s JOIN subscription_runs r ON r.search_run_id=s.run_id WHERE r.id=$1 AND s.account_id=$2`, f.runID, empty).Scan(&state, &observed); err != nil || state != "failed" || observed == nil {
		t.Fatal("mixed-mode source proof/warning was not retained", state, err)
	}
	if _, err := e.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, empty); err != nil {
		t.Fatal(err)
	}
	if err := e.Process(ctx, jobs.Job{Payload: f.payload, Attempts: 2, MaxAttempts: 5}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal("search failure excluded a successful empty RSS proof from revalidation", err)
	}
	var count int
	if err := e.DB.QueryRow(ctx, `SELECT count(*) FROM subscription_rss_accounts WHERE subscription_id=$1`, f.record.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("old source proof committed RSS baseline", count, err)
	}
}

func TestPostgresBothModeCannotCommitRSSBaselineAfterSourceChangesDuringDecision(t *testing.T) {
	for _, change := range []string{"account_revision", "definition"} {
		t.Run(change, func(t *testing.T) {
			f := retryFixtureFor(t)
			ctx, e := f.ctx, f.engine
			f.setPhase(2)
			if _, err := e.DB.Exec(ctx, `UPDATE subscriptions SET source='both',rules=jsonb_set(rules,'{includeHistory}','false') WHERE id=$1`, f.record.ID); err != nil {
				t.Fatal(err)
			}
			block, err := e.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { block.Rollback(context.Background()) })
			if _, err = block.Exec(ctx, `LOCK TABLE media_asset_bindings IN ACCESS EXCLUSIVE MODE`); err != nil {
				t.Fatal(err)
			}
			processCtx, cancel := context.WithCancel(ctx)
			t.Cleanup(cancel)
			result := make(chan error, 1)
			go func() { result <- e.Process(processCtx, jobs.Job{Payload: f.payload, Attempts: 1, MaxAttempts: 5}) }()
			// The relation lock deterministically stops coverage AFTER the first
			// source validation, without runtime hooks or extra production writes.
			blocked := false
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND wait_event='relation' AND query LIKE 'SELECT b.season,b.episode,b.episodes FROM media_asset_bindings %')`).Scan(&blocked)
				if err != nil || blocked {
					break
				}
				select {
				case processErr := <-result:
					t.Fatal("evaluation exited before commit-window barrier", processErr)
				case <-time.After(10 * time.Millisecond):
				}
			}
			if err != nil || !blocked {
				t.Fatal("evaluation did not reach the verified post-validation barrier", err)
			}
			if change == "account_revision" {
				_, err = e.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, f.record.Rules.AccountIDs[1])
			} else {
				_, err = e.DB.Exec(ctx, `UPDATE site_definitions SET source_commit='changed-after-source-validation' WHERE id='owned_retry'`)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = block.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-result; !errors.Is(err, jobs.ErrOperatorRequired) {
				t.Fatal("source edit during decision committed old RSS baseline", err)
			}
			var initialized, seen int
			if err = e.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM subscription_rss_accounts WHERE subscription_id=$1),(SELECT count(*) FROM subscription_rss_seen WHERE subscription_id=$1)`, f.record.ID).Scan(&initialized, &seen); err != nil || initialized != 0 || seen != 0 {
				t.Fatal("source edit committed baseline/seen facts", initialized, seen, err)
			}
			var state, phase string
			if err = e.DB.QueryRow(ctx, `SELECT state,collection_state FROM subscription_runs WHERE id=$1`, f.runID).Scan(&state, &phase); err != nil || state != "needs_refresh" || phase != "needs_refresh" {
				t.Fatal("stale run not paused for fresh evaluation", state, phase, err)
			}
		})
	}
}
