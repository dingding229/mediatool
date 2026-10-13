package sitemetrics

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixture(t *testing.T) (context.Context, Engine, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	var n string
	db.QueryRow(ctx, "SELECT current_database()").Scan(&n)
	if n != "media_tool_test" {
		t.Fatal(n)
	}
	db.Exec(ctx, `TRUNCATE integrations,jobs CASCADE`)
	raw := []byte(`{"templateVersion":2,"id":"owned_scheduler","name":"Owned","domain":"https://owned.example","schema":"NexusPHP","requests":{"user_basic_info":{"parser":"JsonPath","method":"GET","path":"/metrics","fields":{"is_login":{"selector":"$.login"},"name":{"selector":"$.name"},"uploaded":{"selector":"$.uploaded"},"downloaded":{"selector":"$.downloaded"}}}}}`)
	if _, err = sites.DecodeStored(raw, nil); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO site_definitions(id,name,source_commit,definition,source_repository) VALUES('owned_scheduler','Owned','owned',$1,$2) ON CONFLICT(id) DO UPDATE SET definition=excluded.definition,source_repository=excluded.source_repository`, raw, sites.NativeRepository)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := secrets.New(strings.Repeat("9", 64))
	repo := integrations.Repository{DB: db, Vault: v}
	m := integrations.NewManager(repo, config.Config{})
	e := Engine{DB: db, Connections: m}
	acct, err := repo.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "Owned", Enabled: true, Settings: map[string]any{"definitionId": "owned_scheduler", "endpoint": "https://owned.example", "userAgent": "Owned-UA"}, Secrets: map[string]string{"cookie": "owned-cookie"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		m.Close()
		db.Exec(ctx, `TRUNCATE integrations,jobs CASCADE`)
		db.Exec(ctx, `DELETE FROM site_definitions WHERE id='owned_scheduler'`)
		db.Exec(ctx, `UPDATE site_metric_schedule SET enabled=true,interval_minutes=60,revision=1,next_run_at=now()+interval '1 hour',last_dispatched_at=NULL`)
		db.Close()
	})
	return ctx, e, acct.ID
}
func TestPostgresSchedulePersistentDeduplicatedAndConfigGuard(t *testing.T) {
	ctx, e, id := fixture(t)
	e.DB.Exec(ctx, `UPDATE site_metric_schedule SET enabled=true,interval_minutes=60,revision=1,next_run_at=now()-interval '1 minute'`)
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- e.Schedule(ctx) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	e.DB.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind='site.metrics.refresh'`).Scan(&count)
	if count != 1 {
		t.Fatal("concurrent scheduling duplicated", count)
	}
	e.DB.Exec(ctx, `UPDATE site_metric_schedule SET next_run_at=now()-interval '1 minute'`)
	if err := e.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	e.DB.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind='site.metrics.refresh'`).Scan(&count)
	if count != 1 {
		t.Fatal("active job duplicated")
	}
	s, err := e.GetSchedule(ctx)
	if err != nil || s.LastDispatchedAt == nil || !s.NextRunAt.After(time.Now()) {
		t.Fatal(s, err)
	}
	old := s
	s.Cron = "* * * * *"
	if _, err = e.SaveSchedule(ctx, s); !errors.Is(err, ErrInvalidSchedule) {
		t.Fatal(err)
	}
	s = old
	s.Enabled = false
	changed, err := e.SaveSchedule(ctx, s)
	if err != nil || changed.Revision != s.Revision+1 {
		t.Fatal(changed, err)
	}
	if _, err = e.SaveSchedule(ctx, old); !errors.Is(err, ErrScheduleChanged) {
		t.Fatal("lost revision guard", err)
	}
	e.Connections.OutboundHTTP = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		t.Error("disabled/old schedule contacted provider")
		return nil, errors.New("unexpected request")
	})}
	raw, _ := json.Marshal(map[string]any{"accountId": id, "scheduleRevision": old.Revision})
	if err = e.Process(ctx, jobs.Job{Payload: raw}); err != nil {
		t.Fatal(err)
	}
	e.DB.Exec(ctx, `UPDATE site_metric_schedule SET next_run_at=now()-interval '1 minute'`)
	if err = e.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
}
func TestPostgresCollectorLockSuccessFailureAndDisabledAccount(t *testing.T) {
	ctx, e, id := fixture(t)
	began := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var fail atomic.Bool
	var calls atomic.Int32
	e.Connections.OutboundHTTP = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		once.Do(func() { close(began) })
		<-release
		if fail.Load() {
			return nil, errors.New("owned provider failed")
		}
		if r.Header.Get("User-Agent") != "Owned-UA" || r.Header.Get("Cookie") != "owned-cookie" {
			t.Error("identity")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Request: r, Body: io.NopCloser(strings.NewReader(`{"login":true,"name":"owned","uploaded":100,"downloaded":5}`))}, nil
	})}
	done := make(chan error, 1)
	go func() { done <- e.Collect(ctx, id) }()
	<-began
	if err := e.Collect(ctx, id); !errors.Is(err, ErrBusy) {
		t.Fatal("concurrent collector accepted", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var before, after string
	e.DB.QueryRow(ctx, `SELECT metrics::text FROM site_metrics WHERE account_id=$1`, id).Scan(&before)
	if !strings.Contains(before, `"uploaded": 100`) {
		t.Fatal(before)
	}
	fail.Store(true)
	if err := e.Collect(ctx, id); !errors.Is(err, ErrCollection) {
		t.Fatal(err)
	}
	e.DB.QueryRow(ctx, `SELECT metrics::text FROM site_metrics WHERE account_id=$1`, id).Scan(&after)
	if before != after {
		t.Fatal("failure destroyed last success")
	}
	e.DB.Exec(ctx, `UPDATE integrations SET enabled=false WHERE id=$1`, id)
	n := calls.Load()
	if err := e.Collect(ctx, id); !errors.Is(err, ErrChanged) || calls.Load() != n {
		t.Fatal("disabled account contacted")
	}
}

func TestPostgresCollectorWorksWithSingleConnectionRepositoryPool(t *testing.T) {
	ctx, e, id := fixture(t)
	config := e.DB.Config()
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	e.DB = pool
	e.Connections.Repository.DB = pool
	e.Connections.OutboundHTTP = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Request: r, Body: io.NopCloser(strings.NewReader(`{"login":true,"name":"owned","uploaded":100,"downloaded":5}`))}, nil
	})}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = e.Collect(bounded, id); err != nil {
		t.Fatal("lock connection starved the repository pool", err)
	}
}
