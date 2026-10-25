package migrations_test

import (
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/modules/sitemetrics"
	"media-tool/internal/modules/subscriptions"
	"testing"
)

func TestT9MigrationNullRoundTrip(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 85); e != nil {
		t.Fatal(e)
	}
	f.exec(t, `INSERT INTO integrations(id,kind,name,enabled,credentials_ciphertext) VALUES('09000000-0000-4000-8000-000000000001','clouddrive2','T9 test',true,''::bytea)`)
	f.exec(t, `INSERT INTO library_scan_schedules(id,integration_id,root,enabled,interval_seconds,cron_expression,next_run_at) VALUES('09000000-0000-4000-8000-000000000002','09000000-0000-4000-8000-000000000001','/115/T9',true,300,'0 0 0 1 1 * 2020',NULL)`)
	f.exec(t, `INSERT INTO subscriptions(id,title,media_type,enabled,next_run_at) VALUES('09000000-0000-4000-8000-000000000003','T9','movie',true,NULL)`)
	f.exec(t, `UPDATE site_metric_schedule SET enabled=true,next_run_at=NULL,cron_expression='0 0 0 1 1 * 2020'`)
	f.exec(t, `UPDATE ops_policy SET value=value||'{"backupEnabled":true,"backupCron":"0 0 0 1 1 * 2020"}',next_backup_at=NULL`)
	if n := f.integer(t, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND ((table_name IN ('subscriptions','library_scan_schedules','site_metric_schedule') AND column_name='next_run_at') OR (table_name='ops_policy' AND column_name='next_backup_at')) AND is_nullable='YES'`); n != 4 {
		t.Fatal(n)
	}
	f.exec(t, `UPDATE ops_policy SET next_sample_at=now()+interval '1 day'`)
	pool := catalogTestPool(t, f)
	defer pool.Close()
	// Exercise all dispatchers against real NULL rows, not only SQL predicates.
	if e := (library.Engine{DB: pool}).Schedule(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := (subscriptions.Engine{DB: pool}).Schedule(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := (maintenance.Engine{DB: pool}).Schedule(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := (sitemetrics.Engine{DB: pool}).Schedule(f.ctx); e != nil {
		t.Fatal(e)
	}
	rows, e := (library.Engine{DB: pool}).Schedules(f.ctx)
	if e != nil || len(rows) != 1 || rows[0].NextRun != nil {
		t.Fatal(rows, e)
	}
	if n := f.integer(t, `SELECT count(*) FROM jobs`); n != 0 {
		t.Fatal("NULL schedules must not enqueue", n)
	}
	if _, e := f.provider.Down(f.ctx); e != nil {
		t.Fatal(e)
	}
	if n := f.integer(t, `SELECT count(*) FROM site_metric_schedule WHERE NOT enabled AND next_run_at IS NOT NULL AND cron_expression='0 0 0 1 1 * 2020'`); n != 1 {
		t.Fatal(n)
	}
	if n := f.integer(t, `SELECT count(*) FROM ops_policy WHERE value->>'backupEnabled'='false' AND next_backup_at IS NOT NULL AND value->>'backupCron'='0 0 0 1 1 * 2020'`); n != 1 {
		t.Fatal(n)
	}
	if n := f.integer(t, `SELECT count(*) FROM library_scan_schedules WHERE NOT enabled AND next_run_at IS NOT NULL AND cron_expression='0 0 0 1 1 * 2020'`); n != 1 {
		t.Fatal(n)
	}
	if n := f.integer(t, `SELECT count(*) FROM subscriptions WHERE id='09000000-0000-4000-8000-000000000003' AND NOT enabled AND next_run_at IS NOT NULL`); n != 1 {
		t.Fatal(n)
	}
	if _, e := f.provider.UpTo(f.ctx, 85); e != nil {
		t.Fatal(e)
	}
}
