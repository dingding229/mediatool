package idleupdate

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/onlineupdate"
	"media-tool/internal/modules/updatecheck"
	"media-tool/internal/platform/buildinfo"
	"os"
	"strings"
	"testing"
	"time"
)

type fakeUpdate struct {
	jobs                 []onlineupdate.Job
	starts, pulls        int
	startErr, errorState bool
	preflightErr         error
	available            bool
	recovery             []string
	target               updatecheck.Release
	onPrepare            func()
}

func (f *fakeUpdate) VerifyFresh(context.Context) (updatecheck.Release, error) { return f.target, nil }
func (f *fakeUpdate) IdleStatus(context.Context) (onlineupdate.Status, error) {
	return onlineupdate.Status{Available: f.available, Control: onlineupdate.Control{Recovery: f.recovery}}, nil
}
func (f *fakeUpdate) IdleTargetPreflight(_ context.Context, _ updatecheck.Release, pull bool) error {
	if pull {
		f.pulls++
		if f.onPrepare != nil {
			f.onPrepare()
		}
	}
	return f.preflightErr
}
func (f *fakeUpdate) JobsByActor(actor string) ([]onlineupdate.Job, error) {
	out := []onlineupdate.Job{}
	for _, j := range f.jobs {
		if j.Actor == actor {
			out = append(out, j)
		}
	}
	return out, nil
}
func (f *fakeUpdate) Start(_ context.Context, actor, version, digest string) (onlineupdate.Job, error) {
	f.starts++
	j := onlineupdate.Job{ID: newID(), Actor: actor, Phase: "queued", Target: f.target}
	if !f.errorState {
		f.jobs = append(f.jobs, j)
	}
	if f.startErr {
		return j, errors.New("lost response")
	}
	return j, nil
}
func fixture(t *testing.T) (*Service, *fakeUpdate, accounts.Account) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("serialized media_tool_test required")
	}
	if os.Getenv("MEDIA_TOOL_TEST_LOCKED") != "1" {
		t.Fatal("test lock required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated DB required")
	}
	base, e := pgxpool.NewWithConfig(t.Context(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(base.Close)
	name := "t14_" + newID()
	schema := pgx.Identifier{name}.Sanitize()
	_, e = base.Exec(t.Context(), `CREATE SCHEMA `+schema)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := base.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); e != nil {
			t.Error(e)
		}
	})
	cfg.ConnConfig.RuntimeParams["search_path"] = name
	db, e := pgxpool.NewWithConfig(t.Context(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	exec := func(sql string) {
		if _, e = db.Exec(t.Context(), sql); e != nil {
			t.Fatal(e)
		}
	}
	for _, table := range []string{"idle_update_policy", "idle_update_attempts", "idle_update_activity", "outbox_events", "audit_logs"} {
		exec(`CREATE TABLE ` + table + `(LIKE public.` + table + ` INCLUDING ALL)`)
	}
	exec(`CREATE TABLE administrator_accounts(id uuid PRIMARY KEY,credential_version bigint,must_change_password bool,temporary_password_expires_at timestamptz);
 CREATE TABLE native_playback_sessions(state text,last_progress_at timestamptz);
 CREATE TABLE jobs(state text,kind text);CREATE TABLE download_tasks(download_state text);CREATE TABLE manual_download_tasks(state text);CREATE TABLE transfer_tasks(state text);CREATE TABLE file_batches(state text);CREATE TABLE ops_backups(state text);CREATE TABLE ops_drills(state text);CREATE TABLE ops_web_restores(state text);CREATE TABLE ops_mode(singleton bool,paused bool);
 INSERT INTO idle_update_policy(singleton,start_time,end_time)VALUES(true,'00:00','23:59');INSERT INTO idle_update_activity VALUES(true,now()-interval '7 hours');INSERT INTO administrator_accounts VALUES('00000000-0000-0000-0000-000000000001',1,false,NULL)`)
	fake := &fakeUpdate{available: true, target: updatecheck.Release{Metadata: updatecheck.Metadata{Version: "0.41.0", Commit: strings.Repeat("a", 40), Digest: "sha256:" + strings.Repeat("b", 64), MinimumSchema: 79, Schema: 88, Platform: "linux/amd64", Repository: updatecheck.Repository}, Tag: "v0.41.0"}}
	s := New(db, fake, fake)
	s.Root = t.TempDir()
	s.Now = func() time.Time { return time.Date(2026, 10, 26, 3, 0, 0, 0, time.Local) }
	s.Current = func() buildinfo.Info { return buildinfo.Info{Version: "0.40.1", Schema: 88} }
	a := accounts.Account{ID: "00000000-0000-0000-0000-000000000001", Username: "fixture", Version: 1}
	if _, e = s.Save(t.Context(), SaveRequest{Enabled: true, StartTime: "00:00", EndTime: "23:59", QuietMinutes: 60, Revision: 1}, a); e != nil {
		t.Fatal(e)
	}
	return s, fake, a
}
func TestValidationWindowBoundaries(t *testing.T) {
	for _, m := range []int{14, 361} {
		if (SaveRequest{StartTime: "02:00", EndTime: "05:00", QuietMinutes: m}).Validate() == nil {
			t.Fatal(m)
		}
	}
	for _, clock := range []string{"25:00", "2:00", "02:60", ""} {
		if (SaveRequest{StartTime: clock, EndTime: "05:00", QuietMinutes: 60}).Validate() == nil {
			t.Fatal(clock)
		}
	}
	for _, tc := range []struct {
		h, m int
		want bool
	}{{22, 59, false}, {23, 0, true}, {0, 0, true}, {4, 59, true}, {5, 0, false}} {
		now := time.Date(2026, 10, 26, tc.h, tc.m, 0, 0, time.Local)
		if InWindow(now, "23:00", "05:00") != tc.want {
			t.Fatal(tc)
		}
	}
}
func TestAllConditionsReadOnlyAndActivitySessionTaskBoundaries(t *testing.T) {
	s, f, _ := fixture(t)
	ctx := t.Context()
	v, e := s.Check(ctx)
	if e != nil || !v.Ready || len(v.Conditions) != 11 {
		t.Fatal(v, e)
	}
	var n int
	s.DB.QueryRow(ctx, `SELECT count(*)FROM idle_update_attempts`).Scan(&n)
	if n != 0 || f.starts != 0 || f.pulls != 0 {
		t.Fatal("dry-run mutated execution")
	}
	cases := []struct {
		sql, key string
		pass     bool
	}{
		{`INSERT INTO native_playback_sessions VALUES('paused',now()-interval '4 minutes')`, "sessions", false},
		{`UPDATE native_playback_sessions SET last_progress_at=now()-interval '6 minutes'`, "sessions", true},
		{`UPDATE idle_update_activity SET last_issued_at=now()-interval '59 minutes'`, "quiet", false},
		{`UPDATE idle_update_activity SET last_issued_at=now()-interval '61 minutes'`, "quiet", true},
		{`INSERT INTO jobs VALUES('running','transfer.upload')`, "tasks", false},
		{`DELETE FROM jobs;INSERT INTO manual_download_tasks VALUES('uncertain')`, "tasks", false},
		{`DELETE FROM manual_download_tasks;INSERT INTO transfer_tasks VALUES('uploading')`, "tasks", false},
		{`DELETE FROM transfer_tasks;INSERT INTO download_tasks VALUES('downloading')`, "tasks", false},
		{`DELETE FROM download_tasks;UPDATE administrator_accounts SET credential_version=2`, "authorization", false},
	}
	for _, tc := range cases {
		if _, e = s.DB.Exec(ctx, tc.sql); e != nil {
			t.Fatal(e)
		}
		v, e = s.Check(ctx)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, c := range v.Conditions {
			if c.Key == tc.key {
				found = true
				if c.Passed != tc.pass {
					t.Fatal(tc, c)
				}
			}
		}
		if !found {
			t.Fatal(tc)
		}
	}
}
func TestEachVersionPerServerLocalDayAndReconcileSuccess(t *testing.T) {
	s, f, _ := fixture(t)
	ctx := t.Context()
	if e := s.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	if f.starts != 1 {
		t.Fatal(f.starts)
	}
	f.jobs[0].Phase = "succeeded"
	if e := s.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	if f.starts != 1 {
		t.Fatal("same-day retry")
	}
	p, _ := s.Get(ctx)
	if !p.Enabled {
		t.Fatal(p)
	}
	s.Now = func() time.Time { return time.Date(2026, 10, 27, 3, 0, 0, 0, time.Local) }
	if e := s.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	if f.starts != 2 {
		t.Fatal("next server day", f.starts)
	}
}
func TestFailureDisablesGrantAndNotificationIsDeduplicated(t *testing.T) {
	for _, state := range []string{"failed", "rolled_back", "manual_recovery"} {
		t.Run(state, func(t *testing.T) {
			s, f, a := fixture(t)
			ctx := t.Context()
			if e := s.Tick(ctx); e != nil {
				t.Fatal(e)
			}
			f.jobs[0].Phase = state
			if e := s.Tick(ctx); e != nil {
				t.Fatal(e)
			}
			p, _ := s.Get(ctx)
			if p.Enabled || p.Reason == "" {
				t.Fatal(p)
			}
			if e := s.Tick(ctx); e != nil {
				t.Fatal(e)
			}
			var n int
			s.DB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type='idle_update.disabled'`).Scan(&n)
			if n != 1 {
				t.Fatal(n)
			}
			p, e := s.Save(ctx, SaveRequest{Enabled: true, StartTime: p.StartTime, EndTime: p.EndTime, QuietMinutes: p.QuietMinutes, Revision: p.Revision}, a)
			if e != nil {
				t.Fatal(e)
			}
			if e = s.Tick(ctx); e != nil {
				t.Fatal(e)
			}
			if f.starts != 1 {
				t.Fatal("re-enable bypassed daily quota")
			}
			if !p.Enabled {
				t.Fatal(p)
			}
		})
	}
}
func TestCrashOrLostResponseNeverDuplicates(t *testing.T) {
	s, f, _ := fixture(t)
	f.startErr = true
	ctx := t.Context()
	if e := s.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	if f.starts != 1 {
		t.Fatal(f.starts)
	}
	f.jobs[0].Phase = "rolled_back"
	if e := s.Reconcile(ctx); e != nil {
		t.Fatal(e)
	}
	p, _ := s.Get(ctx)
	if p.Enabled {
		t.Fatal(p)
	}
	// Simulate a schema rollback restoring enabled configuration and losing the attempt row.
	if _, e := s.DB.Exec(ctx, `UPDATE idle_update_policy SET enabled=true;DELETE FROM idle_update_attempts`); e != nil {
		t.Fatal(e)
	}
	if e := s.Reconcile(ctx); e != nil {
		t.Fatal(e)
	}
	p, _ = s.Get(ctx)
	if p.Enabled {
		t.Fatal("rollback resurrected grant")
	}
}
func TestInterruptedPreparationAndConfigConflict(t *testing.T) {
	s, _, a := fixture(t)
	ctx := t.Context()
	p, _ := s.Get(ctx)
	if _, e := s.Save(ctx, SaveRequest{StartTime: p.StartTime, EndTime: p.EndTime, QuietMinutes: 60, Revision: 1}, a); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	entry := attempt{ID: newID(), Version: "0.41.0", Day: "2026-10-26", Timezone: time.Local.String(), Grant: p.GrantID, State: "preparing"}
	if e := s.persist(entry); e != nil {
		t.Fatal(e)
	}
	if e := s.Reconcile(ctx); e != nil {
		t.Fatal(e)
	}
	p, _ = s.Get(ctx)
	if p.Enabled {
		t.Fatal("unknown work must disable")
	}
}

func TestGateRecheckAndStartFailure(t *testing.T) {
	t.Run("activity during preparation", func(t *testing.T) {
		s, f, _ := fixture(t)
		f.onPrepare = func() {
			if _, e := s.DB.Exec(t.Context(), `UPDATE idle_update_activity SET last_issued_at=now()`); e != nil {
				t.Fatal(e)
			}
		}
		if e := s.Tick(t.Context()); e != nil {
			t.Fatal(e)
		}
		if f.starts != 0 {
			t.Fatal("started despite recent issuance")
		}
		p, _ := s.Get(t.Context())
		if !p.Enabled {
			t.Fatal("deferred condition revoked authorization")
		}
	})
	t.Run("preflight rejected", func(t *testing.T) {
		s, f, _ := fixture(t)
		f.preflightErr = onlineupdate.ErrImageConfigChanged
		if e := s.Tick(t.Context()); e == nil {
			t.Fatal("expected failed preflight")
		}
		p, _ := s.Get(t.Context())
		if p.Enabled || f.starts != 0 {
			t.Fatal(p, f.starts)
		}
	})
	t.Run("start rejected", func(t *testing.T) {
		s, f, _ := fixture(t)
		f.startErr = true
		f.errorState = true
		if e := s.Tick(t.Context()); e == nil {
			t.Fatal("expected rejection")
		}
		p, _ := s.Get(t.Context())
		if p.Enabled {
			t.Fatal(p)
		}
	})
	t.Run("day boundary", func(t *testing.T) {
		s, f, _ := fixture(t)
		f.onPrepare = func() { s.Now = func() time.Time { return time.Date(2026, 10, 27, 3, 0, 0, 0, time.Local) } }
		if e := s.Tick(t.Context()); e != nil {
			t.Fatal(e)
		}
		if f.starts != 0 {
			t.Fatal("cross-day attempt attributed to yesterday")
		}
	})
}
func TestBlockingConditionsDoNotConsumeAttempts(t *testing.T) {
	for _, condition := range []string{"disabled", "window", "version", "environment", "recovery", "revoked"} {
		t.Run(condition, func(t *testing.T) {
			s, f, _ := fixture(t)
			ctx := t.Context()
			switch condition {
			case "disabled":
				s.DB.Exec(ctx, `UPDATE idle_update_policy SET enabled=false`)
			case "window":
				s.Now = func() time.Time { return time.Date(2026, 10, 26, 23, 59, 0, 0, time.Local) }
			case "version":
				f.target.Version = "0.40.1"
			case "environment":
				f.available = false
			case "recovery":
				f.recovery = []string{"recovery-fixture"}
			case "revoked":
				s.DB.Exec(ctx, `UPDATE administrator_accounts SET credential_version=2`)
			}
			if e := s.Tick(ctx); e != nil {
				t.Fatal(e)
			}
			var n int
			if e := s.DB.QueryRow(ctx, `SELECT count(*)FROM idle_update_attempts`).Scan(&n); e != nil || n != 0 || f.starts != 0 || f.pulls != 0 {
				t.Fatal(condition, n, e, f.starts)
			}
		})
	}
}
