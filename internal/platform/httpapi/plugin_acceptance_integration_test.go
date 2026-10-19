package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	pluginbusiness "media-tool/internal/modules/plugins/business"
	v2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"media-tool/internal/modules/settings"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/outbox"
	"media-tool/internal/platform/testworker"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func acceptanceWait(t *testing.T, label string, fn func() bool) {
	t.Helper()
	until := time.Now().Add(15 * time.Second)
	for time.Now().Before(until) {
		if fn() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("acceptance wait", label)
}
func acceptanceRuntime(t *testing.T, id string, config json.RawMessage) (context.Context, v2.Store, v2.Definition, v2.Instance, *integrations.Manager) {
	t.Helper()
	if runtime.GOOS != "linux" || os.Getenv("MEDIA_TOOL_PLUGIN_BINARY") == "" {
		t.Skip("native same-app worker gate required")
	}
	if os.Getenv("MEDIA_TOOL_TEST_PLUGIN_UID") == "65532" && os.Getuid() != 65532 {
		t.Fatal("dedicated native worker UID required")
	}
	if e := sandbox.Parent(); e != nil {
		t.Fatal(e)
	}
	ctx, s := pluginHTTPFixture(t)
	m := integrations.NewManager(integrations.Repository{DB: s.DB, Vault: s.Vault}, cfg())
	t.Cleanup(m.Close)
	raw, _ := json.Marshal(settings.Defaults())
	if _, e := s.DB.Exec(ctx, `UPDATE application_settings SET value=$1`, raw); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(ctx, `INSERT INTO network_proxy_settings(id)VALUES('main')`); e != nil {
		t.Fatal(e)
	}
	s.Business = pluginbusiness.AcquisitionHost{DB: s.DB, Vault: s.Vault, Connections: m}
	s.Logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	defs, e := v2.Catalog()
	if e != nil {
		t.Fatal(e)
	}
	var d v2.Definition
	for _, v := range defs {
		if v.Manifest.ID == id {
			d = v
		}
	}
	if d.Manifest.ID == "" {
		t.Fatal("missing acceptance definition")
	}
	scopes := []string{}
	for _, p := range d.Manifest.Permissions {
		scopes = append(scopes, p.Scope)
	}
	in, e := s.Apply(ctx, "admin", v2.Request{Kind: v2.Install, PluginID: id, Scopes: scopes, Fingerprint: d.Fingerprint, Config: config, OperationKey: jobs.UUID(), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	// Configuration can be changed before enable, without constructing a worker.
	return ctx, s, d, in, m
}
func startAcceptance(t *testing.T, ctx context.Context, s v2.Store, d v2.Definition, in v2.Instance) v2.Instance {
	t.Helper()
	next, e := s.Apply(ctx, "admin", v2.Request{Kind: v2.Enable, InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	run, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- (v2.Supervisor{Store: s, Executable: os.Getenv("MEDIA_TOOL_PLUGIN_BINARY"), Logger: s.Logger, MaxWorkers: 1}).Run(run)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(15 * time.Second):
			t.Error("worker reaper timed out")
		}
	})
	testworker.Ready(t, s.DB, in.ID, func(c context.Context) error { _, e := s.UI(c, in.ID, "form", ""); return e })
	return next
}
func runAcceptanceService(t *testing.T, ctx context.Context, s v2.Store, id, service string) error {
	t.Helper()
	job, e := s.RunService(ctx, "admin", id, service, jobs.UUID())
	if e != nil {
		return e
	}
	var payload json.RawMessage
	if e = s.DB.QueryRow(ctx, `SELECT payload FROM jobs WHERE id=$1`, job).Scan(&payload); e != nil {
		return e
	}
	err := s.ProcessService(ctx, jobs.Job{ID: job, Payload: payload})
	// Test driver acknowledges the durable job exactly as a worker would; no other
	// kinds are consumed, so native refresh remains queued without external IO.
	state := "succeeded"
	if err != nil {
		state = "failed"
	}
	if _, e := s.DB.Exec(ctx, `UPDATE jobs SET state=$2,updated_at=now() WHERE id=$1`, job, state); e != nil {
		t.Fatal(e)
	}
	if err == nil {
		state := ""
		acceptanceWait(t, "durable service run finished", func() bool {
			e := s.DB.QueryRow(ctx, `SELECT state FROM plugin_v2_runs WHERE dedupe_key=$1`, "service-job:"+job).Scan(&state)
			return e == nil && state != "pending" && state != "running"
		})
		if state != "succeeded" {
			return fmt.Errorf("service terminal state: %s", state)
		}
	}
	return err
}
func stopAcceptance(t *testing.T, ctx context.Context, s v2.Store, d v2.Definition, id string) {
	t.Helper()
	var rev int64
	if e := s.DB.QueryRow(ctx, `SELECT revision FROM plugin_v2_instances WHERE id=$1`, id).Scan(&rev); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Apply(ctx, "admin", v2.Request{Kind: v2.Disable, InstanceID: id, ExpectedRevision: rev, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true}); e != nil {
		t.Fatal(e)
	}
	acceptanceWait(t, "no worker references", func() bool {
		var ok bool
		return s.DB.QueryRow(ctx, `SELECT desired_state='disabled' AND worker_pid=0 AND generation IS NULL AND owner_id IS NULL AND lease_until IS NULL FROM plugin_v2_instances WHERE id=$1`, id).Scan(&ok) == nil && ok
	})
}
func TestAcceptanceRealWorkerSiteSignInAndChallenge(t *testing.T) {
	for _, challenge := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "captcha"}[challenge], func(t *testing.T) {
			ctx, s, d, in, m := acceptanceRuntime(t, "site-checkin", json.RawMessage(`{}`))
			var calls atomic.Int32
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Cookie") != "session=SYNTHETIC_COOKIE_SECRET" {
					t.Error("missing host-only credential")
				}
				w.Header().Set("Set-Cookie", "SYNTHETIC_RESPONSE_SECRET")
				if challenge {
					_, _ = w.Write([]byte("captcha"))
				} else {
					_, _ = w.Write([]byte("SYNTHETIC_COOKIE_SECRET signed"))
				}
			}))
			defer fake.Close()
			if e := (sites.Repository{DB: s.DB}).LoadGoTemplates(ctx, []string{"hdsky"}); e != nil {
				t.Fatal(e)
			}
			account, e := m.Repository.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "synthetic", Enabled: true, Settings: map[string]any{"definitionId": "hdsky", "endpoint": fake.URL, "userId": "1"}, Secrets: map[string]string{"cookie": "session=SYNTHETIC_COOKIE_SECRET"}})
			if e != nil {
				t.Fatal(e)
			}
			raw, _ := json.Marshal(map[string]any{"active": true, "accountId": account.ID, "origin": fake.URL, "path": "/attendance.php", "method": "GET", "successMarker": "signed", "cron": "0 8 * * *"})
			in, e = s.Apply(ctx, "admin", v2.Request{Kind: v2.Configure, InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true, Config: raw})
			if e != nil {
				t.Fatal(e)
			}
			startAcceptance(t, ctx, s, d, in)
			err := runAcceptanceService(t, ctx, s, in.ID, "checkin")
			if !challenge && err != nil {
				t.Fatal(err)
			}
			if challenge {
				acceptanceWait(t, "needs_action and reaped", func() bool {
					var ok bool
					return s.DB.QueryRow(ctx, `SELECT desired_state='disabled' AND runtime_state='needs_action' AND worker_pid=0 AND generation IS NULL FROM plugin_v2_instances WHERE id=$1`, in.ID).Scan(&ok) == nil && ok
				})
			} else {
				if e = runAcceptanceService(t, ctx, s, in.ID, "checkin"); e != nil {
					t.Fatal(e)
				}
				stopAcceptance(t, ctx, s, d, in.ID)
			}
			if calls.Load() != 1 {
				t.Fatal("duplicate external effect", calls.Load())
			}
			inspect, e := s.Inspect(ctx, in.ID, "")
			if e != nil {
				t.Fatal(e)
			}
			data, _ := json.Marshal(inspect.Entries)
			if strings.Contains(string(data), "SYNTHETIC_COOKIE_SECRET") || strings.Contains(string(data), "SYNTHETIC_RESPONSE_SECRET") {
				t.Fatal("credential escaped host")
			}
			var notices int
			if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type='plugin.notice' AND payload->>'instanceId'=$1`, in.ID).Scan(&notices); e != nil || notices != 1 {
				t.Fatal(notices, e)
			}
			t.Log("same app worker -> live scope/fence -> real controlled synthetic HTTP -> notification outbox; no credential payload; challenge yields own instance")
		})
	}
}
func TestAcceptanceRealWorkerImportNotifyMergeAndRefresh(t *testing.T) {
	ctx, s, d, in, m := acceptanceRuntime(t, "library-refresh-notify", json.RawMessage(`{"active":true,"provider":"builtin","intervalSeconds":60}`))
	if _, e := m.Repository.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "synthetic metadata", Enabled: true, Secrets: map[string]string{"token": "SYNTHETIC_TMDB_SECRET"}}); e != nil {
		t.Fatal(e)
	}
	storage, file, asset, media := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO storage_connections(id,name,provider)VALUES($1,'synthetic','clouddrive2')`, []any{storage}},
		{`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/synthetic/Owned.mkv',4)`, []any{file, storage}},
		{`INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, []any{asset, file}},
		{`INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,'movie',42,'Synthetic','2026','{}')`, []any{media}},
		{`INSERT INTO media_asset_bindings(asset_id,media_id)VALUES($1,$2)`, []any{asset, media}},
		{`INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,'synthetic.strm','synthetic-reference')`, []any{jobs.UUID(), asset}},
		{`INSERT INTO native_libraries(id,item_id,group_id,name,collection_type,directory,sort_order,policy_revision)VALUES('g:movie',100001,'movie','Synthetic','movies','Movies',0,1)`, nil},
		{`INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision)VALUES($1,'g:movie','classification',1,1)`, []any{asset}},
	}
	for _, q := range statements {
		if _, e := s.DB.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	startAcceptance(t, ctx, s, d, in)
	emit := func(tx pgx.Tx, version string) error {
		return outbox.EmitVersionTx(ctx, tx, "library.organized", asset, version, contracts.TransferCompleted{OperationID: file, AssetID: asset, State: "succeeded"})
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = emit(tx, "rolled-back"); e != nil {
		t.Fatal(e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	for _, version := range []string{"import-v1", "import-v1", "import-v2"} {
		tx, e = s.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if e = emit(tx, version); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	acceptanceWait(t, "two unique import notices", func() bool {
		var n int
		return s.DB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type='plugin.notice' AND payload->>'instanceId'=$1 AND payload->>'title'='媒体入库已确认'`, in.ID).Scan(&n) == nil && n == 2
	})
	if e = runAcceptanceService(t, ctx, s, in.ID, "flush"); e != nil {
		t.Fatal(e)
	}
	if e = runAcceptanceService(t, ctx, s, in.ID, "flush"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Replay(ctx, "admin", in.ID, 0, 200, false); e != nil {
		t.Fatal(e)
	}
	var effects, refreshes, imports int
	for _, q := range []struct {
		sql string
		id  string
		out *int
	}{
		{`SELECT count(*) FROM plugin_v2_kv WHERE instance_id=$1 AND key LIKE '.host/%'`, in.ID, &effects},
		{`SELECT count(*) FROM plugin_v2_event_journal WHERE event_type='library.organized' AND payload->>'assetId'=$1`, asset, &imports},
	} {
		if e = s.DB.QueryRow(ctx, q.sql, q.id).Scan(q.out); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM native_tmdb_details WHERE tmdb_id=42`).Scan(&refreshes); e != nil {
		t.Fatal(e)
	}
	if effects != 3 || refreshes != 1 || imports != 2 {
		t.Fatal("expected two notices, one coalesced refresh, no rolled-back or duplicate event", effects, refreshes, imports)
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='mediaserver.refresh'`, in.ID); e != nil {
		t.Fatal(e)
	}
	if e = runAcceptanceService(t, ctx, s, in.ID, "flush"); e == nil {
		t.Fatal("revoked scope accepted")
	}
	stopAcceptance(t, ctx, s, d, in.ID)
	t.Log("same app worker -> typed TransferComplete -> durable receipts -> native queued refresh + existing notification outbox; merge/dedupe/revocation verified")
}
