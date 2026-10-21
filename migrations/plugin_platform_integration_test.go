package migrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/messaging"
	"media-tool/internal/modules/settings"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/pluginactor"
	"media-tool/internal/platform/safeerrors"
	"strings"
	"testing"
)

func TestF4DSettingsDiffIsTransactionalAndNeverContainsSecurityValues(t *testing.T) {
	f := latestDiscoveryFixture(t)
	repo := settings.Repository{DB: f.pool}
	before, e := repo.Get(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	input := before.Value
	input.SearchCacheSeconds += 1
	blocked := dbutil.WithCommitGuard(f.ctx, func(context.Context, pgx.Tx) error { return errors.New("test rollback") })
	if _, e = repo.Save(blocked, input, before.Revision); e == nil {
		t.Fatal("guard should roll back")
	}
	if f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='settings.changed'`) != 0 {
		t.Fatal("rolled back settings emitted")
	}
	if _, e = repo.Save(f.ctx, input, before.Revision); e != nil {
		t.Fatal(e)
	}
	name := f.text(t, `SELECT server_name FROM native_server_settings WHERE singleton`)
	rev := f.integer(t, `SELECT revision FROM native_server_settings WHERE singleton`)
	if _, e = settings.SaveNativeManagement(blocked, f.pool, settings.NativeManagement{ServerName: "password=NEVER_PLUGIN_SECRET", Enabled: true, Revision: rev}); e == nil {
		t.Fatal("native guard should roll back")
	}
	if f.text(t, `SELECT server_name FROM native_server_settings WHERE singleton`) != name {
		t.Fatal("native rollback changed state")
	}
	if _, e = settings.SaveNativeManagement(f.ctx, f.pool, settings.NativeManagement{ServerName: "password=NEVER_PLUGIN_SECRET", Enabled: true, Revision: rev}); e != nil {
		t.Fatal(e)
	}
	raw := f.text(t, `SELECT jsonb_agg(payload)::text FROM outbox_events WHERE event_type='settings.changed'`)
	if strings.Contains(raw, "NEVER_PLUGIN_SECRET") || strings.Contains(raw, "password=") || strings.Contains(raw, "trustedProxy") {
		t.Fatal("settings values leaked")
	}
	if !strings.Contains(raw, "searchCacheSeconds") || !strings.Contains(raw, "serverName") || f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='settings.changed'`) != 2 {
		t.Fatal(raw)
	}
}
func TestF4DErrorBusAggregatesThrottlesExcludesPluginsAndCommitsWithTerminalJob(t *testing.T) {
	f := latestDiscoveryFixture(t)
	// These assertions execute the current runtime, not the historical DTOs.
	if _, err := f.provider.UpTo(f.ctx, 74); err != nil {
		t.Fatal(err)
	}
	bus := safeerrors.Bus{DB: f.pool}
	tx, e := f.pool.Begin(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = safeerrors.EmitTx(f.ctx, tx, safeerrors.Summary{Source: "system", Code: "dependency_unavailable"}); e != nil {
		t.Fatal(e)
	}
	if e = tx.Rollback(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.integer(t, `SELECT count(*) FROM app_settings WHERE key LIKE 'plugins.errors.%'`) != 0 {
		t.Fatal("rolled back aggregate")
	}
	for range 4 {
		if e = bus.Report(f.ctx, safeerrors.Summary{Source: "system", Code: "dependency_unavailable"}); e != nil {
			t.Fatal(e)
		}
	}
	if e = bus.Report(f.ctx, safeerrors.Summary{Source: "plugins", Code: "terminal_failure"}); e != nil {
		t.Fatal(e)
	}
	actor := pluginactor.With(f.ctx, jobs.UUID(), "jobs.read", nil)
	if e = bus.Report(actor, safeerrors.Summary{Source: "jobs", Code: "terminal_failure"}); e != nil {
		t.Fatal(e)
	}
	if e = bus.Flush(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='system.operation.failed'`) != 0 {
		t.Fatal("open minute emitted before aggregation")
	}
	f.exec(t, `UPDATE app_settings SET value=jsonb_set(value,'{window}',to_jsonb((now()-interval '2 minutes')::text)) WHERE key LIKE 'plugins.errors.%'`)
	for range 2 {
		if e = bus.Flush(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	if f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='system.operation.failed'`) != 1 || f.integer(t, `SELECT (payload->>'count')::int FROM outbox_events WHERE event_type='system.operation.failed'`) != 4 {
		t.Fatal("aggregate not exactly once")
	}
	for _, kind := range []string{"library.fixture", "plugin.v2.service"} {
		id, token := jobs.UUID(), jobs.UUID()
		f.exec(t, `INSERT INTO jobs(id,kind,state,attempts,max_attempts,lease_token,lease_until,locked_by)VALUES($1,$2,'running',1,1,$3,now()+interval '1 minute','fixture')`, id, kind, token)
		if e = (jobs.Repository{DB: f.pool}).Finish(f.ctx, jobs.Job{ID: id, Kind: kind, Attempts: 1, MaxAttempts: 1, LeaseToken: token}, errors.New("password=NEVER_PLUGIN_SECRET\nprivate stack")); e != nil {
			t.Fatal(e)
		}
	}
	if f.integer(t, `SELECT count(*) FROM app_settings WHERE key LIKE 'plugins.errors.jobs.terminal_failure.%'`) != 1 {
		t.Fatal("plugin terminal error entered error bus")
	}
	raw := f.text(t, `SELECT jsonb_agg(value)::text FROM app_settings WHERE key LIKE 'plugins.errors.%'`)
	if strings.Contains(raw, "NEVER_PLUGIN_SECRET") || strings.Contains(raw, "stack") {
		t.Fatal("raw error propagated")
	}
}
func TestF4DNotificationQueuedOnlyAfterCommitNotDeliveryOrRecursiveBus(t *testing.T) {
	f := latestDiscoveryFixture(t)
	body := json.RawMessage(`{"title":"private fixture","message":"password=NEVER_PLUGIN_SECRET"}`)
	f.exec(t, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'media.imported',$2,$3)`, jobs.UUID(), jobs.UUID(), body)
	d := messaging.Dispatcher{DB: f.pool}
	blocked := dbutil.WithCommitGuard(f.ctx, func(context.Context, pgx.Tx) error { return errors.New("test rollback") })
	if e := d.Prepare(blocked); e == nil {
		t.Fatal("guard should roll back prepare")
	}
	if f.integer(t, `SELECT count(*) FROM notification_messages`) != 0 || f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='notification.queued'`) != 0 {
		t.Fatal("rollback published notification")
	}
	for range 2 {
		if e := d.Prepare(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	if f.integer(t, `SELECT count(*) FROM notification_messages`) != 1 || f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='notification.queued'`) != 1 {
		t.Fatal("queued duplicated")
	}
	if f.integer(t, `SELECT count(*) FROM notification_deliveries WHERE state='sent'`) != 0 {
		t.Fatal("queued claimed delivered")
	}
	raw := f.text(t, `SELECT payload::text FROM outbox_events WHERE event_type='notification.queued'`)
	if strings.Contains(raw, "NEVER_PLUGIN_SECRET") || strings.Contains(raw, "message") {
		t.Fatal("private notification body leaked")
	}
	f.exec(t, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'plugin.notice',$2,$3)`, jobs.UUID(), jobs.UUID(), body)
	if e := d.Prepare(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='notification.queued' AND payload->>'origin'='plugin'`) != 1 {
		t.Fatal("missing recursion marker")
	}
}
