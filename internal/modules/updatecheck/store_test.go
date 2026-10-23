package updatecheck

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/buildinfo"
	"os"
	"strings"
	"testing"
	"time"
)

func isolatedService(t *testing.T) *Service {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test only")
	}
	c, e := pgxpool.ParseConfig(dsn)
	if e != nil || c.ConnConfig.Database != "media_tool_test" {
		t.Fatal("test database required")
	}
	base, e := pgxpool.NewWithConfig(t.Context(), c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(base.Close)
	schema := "f8a_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	if _, e = base.Exec(t.Context(), `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := base.Exec(context.Background(), `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`); e != nil {
			t.Error(e)
		}
	})
	c.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(t.Context(), c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	if _, e = db.Exec(t.Context(), `CREATE TABLE app_settings(LIKE public.app_settings INCLUDING ALL)`); e != nil {
		t.Fatal(e)
	}
	s := fixtureClient(t, nil, 0)
	s.DB = db
	s.Current = func() buildinfo.Info { return buildinfo.Info{Version: "0.27.0", Schema: 79} }
	return s
}
func TestPersistedToggleChecksOneTimeNoticeAndUntrustedInvalidation(t *testing.T) {
	s := isolatedService(t)
	ctx := t.Context()
	v, e := s.Get(ctx)
	if e != nil || !v.Enabled || v.IntervalHours != 12 {
		t.Fatal(v, e)
	}
	if _, e = s.SetEnabled(ctx, false); e != nil {
		t.Fatal(e)
	}
	v, e = s.Check(ctx)
	if e != nil || v.Enabled || !v.HasUpdate || v.Result.Status != "verified" {
		t.Fatal(v, e)
	}
	for i := 0; i < 3; i++ {
		n, e := s.TakeNotice(ctx)
		if e != nil || (n != nil) != (i == 0) {
			t.Fatal("notice repeated", i, n, e)
		}
	}
	// New process reads the persisted setting and the already claimed notice.
	next := New(s.DB, nil)
	next.Client = s.Client
	next.Current = s.Current
	n, e := next.TakeNotice(ctx)
	if e != nil || n != nil {
		t.Fatal(n, e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE app_settings SET value=jsonb_set(value,'{result,checkedAt}',to_jsonb($2::text)) WHERE key=$1`, settingsKey, time.Now().Add(-2*time.Minute).UTC().Format(time.RFC3339)); e != nil {
		t.Fatal(e)
	}
	mismatch := fixtureClient(t, func(u string, v map[string]any) {
		if u != latestURL {
			v["digest"] = "sha256:" + strings.Repeat("d", 64)
		}
	}, 0)
	s.Client = mismatch.Client
	v, e = s.Check(ctx)
	if e != nil || v.Result.Status != "untrusted" || v.HasUpdate || v.Result.Latest != nil || v.Enabled {
		t.Fatal("untrusted result must suppress any stale update", v, e)
	}
	if n, e = s.TakeNotice(ctx); e != nil || n != nil {
		t.Fatal(n, e)
	}
}
func TestCheckCadenceAndCrossProcessLock(t *testing.T) {
	s := isolatedService(t)
	ctx := t.Context()
	for _, c := range []struct {
		enabled bool
		age     time.Duration
		want    bool
	}{{true, 13 * time.Hour, true}, {true, 11 * time.Hour, false}, {false, 13 * time.Hour, false}} {
		v := State{Enabled: c.enabled, Result: Result{CheckedAt: time.Now().Add(-c.age)}}
		if shouldCheck(v, time.Now()) != c.want {
			t.Fatal(c)
		}
	}
	conn, e := s.DB.Acquire(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Release()
	var yes bool
	if e = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(193293,8)`).Scan(&yes); e != nil || !yes {
		t.Fatal(e)
	}
	defer func() {
		var v bool
		if e := conn.QueryRow(context.Background(), `SELECT pg_advisory_unlock(193293,8)`).Scan(&v); e != nil {
			t.Error(e)
		}
	}()
	if _, e = s.Check(ctx); e != ErrBusy {
		t.Fatal("parallel check must not use network", e)
	}
}
