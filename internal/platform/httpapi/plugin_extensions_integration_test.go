package httpapi

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"media-tool/internal/modules/filemanager"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/logging"
	"media-tool/internal/platform/secrets"
	"media-tool/internal/platform/testworker"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func pluginHTTPFixture(t *testing.T) (context.Context, pluginv2.Store) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated test DB required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	base, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	schema := "plugin_http_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	q := pgx.Identifier{schema}.Sanitize()
	if _, e = base.Exec(ctx, `CREATE SCHEMA `+q); e != nil {
		t.Fatal(e)
	}
	rows, e := base.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename<>'goose_db_version' ORDER BY tablename`)
	if e != nil {
		t.Fatal(e)
	}
	names := []string{}
	for rows.Next() {
		var name string
		if e = rows.Scan(&name); e != nil {
			t.Fatal(e)
		}
		names = append(names, name)
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	rows.Close()
	for _, name := range names {
		if _, e = base.Exec(ctx, `CREATE TABLE `+pgx.Identifier{schema, name}.Sanitize()+` (LIKE public.`+pgx.Identifier{name}.Sanitize()+` INCLUDING ALL)`); e != nil {
			t.Fatal(e)
		}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e = db.Exec(context.Background(), `DROP SCHEMA `+q+` CASCADE`); e != nil {
			t.Error(e)
		}
		db.Close()
	})
	for _, sql := range []string{`INSERT INTO plugin_v2_journal_retention(singleton)VALUES(true)`, `INSERT INTO ops_mode(singleton)VALUES(true)`, `INSERT INTO application_settings(singleton)VALUES(true)`, `INSERT INTO directory_classification(singleton)VALUES(true)`} {
		if _, e = db.Exec(ctx, sql); e != nil {
			t.Fatal(e)
		}
	}
	vault, _ := secrets.New(strings.Repeat("a", 64))
	return ctx, pluginv2.Store{DB: db, Vault: vault, Root: filepath.Join(t.TempDir(), "plugins")}
}
func TestPluginRealBinaryHTTPServicesTransferAndDisable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("real worker Linux kernel required; run isolated static kernel suite")
	}
	binary := os.Getenv("MEDIA_TOOL_PLUGIN_BINARY")
	if binary == "" {
		t.Skip("real same-binary worker required")
	}
	if os.Getenv("MEDIA_TOOL_TEST_PLUGIN_UID") == "65532" && os.Getuid() != 65532 {
		t.Fatal("native plugin host must run as UID 65532; check Go test binary executor mapping")
	}
	if e := sandbox.Parent(); e != nil {
		t.Fatal(e)
	}
	ctx, s := pluginHTTPFixture(t)
	logs, e := logging.Open(t.TempDir(), "info")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := logs.Close(); e != nil {
			t.Error(e)
		}
	})
	logger, e := logging.NewRawControlled(io.MultiWriter(os.Stderr, logs), "info", &logs.Level)
	if e != nil {
		t.Fatal(e)
	}
	s.Logger = logger

	defs, e := pluginv2.Catalog()
	if e != nil {
		t.Fatal(e)
	}
	d := defs[0]
	scopes := []string{}
	for _, p := range d.Manifest.Permissions {
		scopes = append(scopes, p.Scope)
	}
	in, e := s.Apply(ctx, "admin", pluginv2.Request{Kind: pluginv2.Install, OperationKey: "real-http-install", PluginID: d.Manifest.ID, Fingerprint: d.Fingerprint, Scopes: scopes, Config: json.RawMessage(`{"scheduled":true,"recognizeTitle":"Chain Rewrite","moduleTitle":"Module Rewrite"}`), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	in, e = s.Apply(ctx, "admin", pluginv2.Request{Kind: pluginv2.Enable, OperationKey: "real-http-enable", InstanceID: in.ID, Fingerprint: d.Fingerprint, ExpectedRevision: in.Revision, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	live, stop := context.WithCancel(ctx)
	done := make(chan error, 2)
	go func() {
		done <- (pluginv2.Supervisor{Store: s, Executable: binary, Logger: logger, MaxWorkers: 1}).Run(live)
	}()
	go func() {
		done <- (jobs.Worker{Repository: jobs.Repository{DB: s.DB}, Handlers: map[string]jobs.Handler{"plugin.v2.service": s.ProcessService}, Owner: jobs.UUID(), Queue: "default", Lease: 3 * time.Second, PollInterval: 100 * time.Millisecond, Logger: logger}).Run(live)
	}()
	t.Cleanup(func() {
		stop()
		for range 2 {
			select {
			case e := <-done:
				if e != nil {
					t.Error(e)
				}
			case <-time.After(15 * time.Second):
				t.Error("runtime did not stop")
			}
		}
	})
	wait := func(label string, fn func() bool) {
		t.Helper()
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			if fn() {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatal("timed out:", label)
	}
	testworker.Ready(t, s.DB, in.ID, func(c context.Context) error { _, err := s.UI(c, in.ID, "form", ""); return err })
	c := cfg()
	c.SecretKey = strings.Repeat("a", 64)
	c.PluginDataRoot = s.Root
	server := httptest.NewServer(New(c, Dependencies{DB: s.DB, PluginInstances: s, AuthSessions: testSessionReader{}, Logger: logger}))
	t.Cleanup(server.Close)
	token, e := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	request := func(method, path string, admin bool, headers map[string]string, body string) (int, []byte) {
		t.Helper()
		req, e := http.NewRequestWithContext(ctx, method, server.URL+path, strings.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		if admin {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		response, e := server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, e := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, raw
	}

	// Real HTTP tool calls use the same release binary and live scope ceiling.
	toolURL := "/api/v1/plugins/" + in.ID + "/tools/call"
	toolHeaders := map[string]string{"Origin": c.PublicURL, "X-Requested-With": "media-tool"}
	if status, b := request("POST", toolURL, true, toolHeaders, `{"name":"hello.echo","input":{"message":"http-worker"},"operationKey":"http-tool-echo"}`); status != 200 || !strings.Contains(string(b), "http-worker") {
		t.Fatal("real HTTP tool", status, string(b))
	}
	if status, _ := request("POST", toolURL, true, toolHeaders, `{"name":"hello.remember","input":{"message":"no"},"operationKey":"http-tool-write","confirm":false}`); status != 403 {
		t.Fatal("tool effect confirmation", status)
	}
	if status, _ := request("POST", toolURL, true, toolHeaders, `{"name":"hello.echo","input":{"message":7},"operationKey":"http-tool-invalid"}`); status != 400 {
		t.Fatal("tool schema bypass", status)
	}
	// Real admin HTTP recognition runs the typed chain first, then a module override.
	if status, b := request("POST", "/api/v1/media/recognize-name", true, map[string]string{"Origin": c.PublicURL, "X-Requested-With": "media-tool"}, `{"name":"Owned.2026.mkv"}`); status != 200 || !strings.Contains(string(b), "Module Rewrite") {
		t.Fatal("NameRecognize/module", status, string(b))
	}
	for _, kind := range []string{"chain", "module"} {
		var count int
		if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE instance_id=$1 AND kind=$2 AND state='succeeded'`, in.ID, kind).Scan(&count); err != nil || count != 1 {
			t.Fatal("real hook missing", kind, count, err)
		}
	}
	for _, kind := range []string{"form", "page?key=overview", "dashboard?key=starts", "dashboard?key=events", "meta"} {
		if status, b := request("GET", "/api/v1/plugins/"+in.ID+"/ui/"+kind, true, nil, ""); status != 200 {
			t.Fatal("UI", kind, status, string(b))
		}
	}
	path := "/api/v1/plugin/" + in.ID
	if status, _ := request("GET", path+"/admin", false, nil, ""); status != 401 {
		t.Fatal("admin GET anonymous", status)
	}
	if status, b := request("GET", path+"/admin", true, nil, ""); status != 200 || !strings.Contains(string(b), "ticks") {
		t.Fatal(status, string(b))
	}
	if status, _ := request("POST", path+"/admin", true, nil, `{}`); status != 403 {
		t.Fatal("CSRF", status)
	}
	if status, _ := request("POST", path+"/admin", true, map[string]string{"Origin": c.PublicURL, "X-Requested-With": "media-tool"}, `{}`); status != 200 {
		items, _ := s.List(ctx, 0, 50)
		runs, _ := s.History(ctx, in.ID, 0, 50)
		t.Logf("worker states=%+v runs=%+v", items, runs)
		t.Fatal("authorized POST", status)
	}
	if status, _ := request("GET", path+"/public", false, nil, ""); status != 200 {
		t.Fatal("public", status)
	}
	key, e := s.CreateAPIKey(ctx, "admin", in.ID, "owned", []string{"apikey:GET", "bearer:GET"}, time.Now().Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	for _, x := range []struct{ route, header, value string }{{"apikey", "X-API-Key", key.Token}, {"bearer", "Authorization", "Bearer " + key.Token}} {
		if status, _ := request("GET", path+"/"+x.route, false, map[string]string{x.header: x.value}, ""); status != 200 {
			t.Fatal(x.route, status)
		}
	}
	if status, _ := request("GET", path+"/bearer", false, map[string]string{"Authorization": "Bearer " + c.STRMKey}, ""); status != 401 {
		t.Fatal("global key accepted", status)
	}
	var argsCipher []byte
	var callID string
	if e = s.DB.QueryRow(ctx, `SELECT id::text,args_ciphertext FROM plugin_v2_runs WHERE instance_id=$1 AND kind='api' ORDER BY created_at DESC LIMIT 1`, in.ID).Scan(&callID, &argsCipher); e != nil {
		t.Fatal(e)
	}
	decoded, e := s.Vault.Open("plugin:v2:call-args:"+callID, argsCipher)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(decoded["value"], key.Token) || strings.Contains(decoded["value"], token) {
		t.Fatal("credential forwarded to plugin")
	}
	if e = s.RevokeAPIKey(ctx, "admin", in.ID, key.ID); e != nil {
		t.Fatal(e)
	}
	if status, _ := request("GET", path+"/apikey", false, map[string]string{"X-API-Key": key.Token}, ""); status != 401 {
		t.Fatal("revoked key accepted", status)
	}
	wait("scheduled service writes KV", func() bool {
		data, e := s.Inspect(ctx, in.ID, "")
		if e != nil {
			return false
		}
		for _, v := range data.Entries {
			if v.Key == "ticks" && string(v.Value) != "0" {
				return true
			}
		}
		return false
	})
	// Real filemanager preview -> execute commits the original files.* outbox.
	directory := t.TempDir()
	if e = os.WriteFile(filepath.Join(directory, "Owned.mkv"), []byte("owned synthetic file"), 0600); e != nil {
		t.Fatal(e)
	}
	repo := integrations.Repository{DB: s.DB, Vault: s.Vault}
	connection, e := repo.Save(ctx, "", integrations.Input{Kind: "local", Name: "Owned fixture", Enabled: true, Settings: map[string]any{"rootPath": directory}})
	if e != nil {
		t.Fatal(e)
	}
	manager := integrations.NewManager(repo, c)
	t.Cleanup(manager.Close)
	engine := filemanager.Engine{DB: s.DB, Connections: manager}
	plan, e := engine.Preview(ctx, filemanager.Input{ConnectionID: connection.ID, Operation: "organize", Source: "Owned.mkv", MediaType: "movie", Title: "Owned", Year: "2026", Template: "{title} ({year}){ext}"})
	if e != nil {
		t.Fatal(e)
	}
	result, e := engine.Execute(ctx, plan.ID, plan.Digest, true)
	if e != nil || result.State != "succeeded" {
		t.Fatal(result, e)
	}
	var event string
	if e = s.DB.QueryRow(ctx, `SELECT id::text FROM outbox_events WHERE payload->>'operationId'=$1 AND event_type='files.succeeded'`, plan.ID).Scan(&event); e != nil {
		t.Fatal(e)
	}
	wait("TransferComplete delivered into real Go handler", func() bool {
		data, e := s.Inspect(ctx, in.ID, "")
		if e != nil {
			return false
		}
		for _, v := range data.Entries {
			if v.Key == "event-"+event && string(v.Value) == "true" {
				return true
			}
		}
		return false
	})
	if e = s.Bridge(ctx); e != nil {
		t.Fatal(e)
	}
	var operation string
	if e = s.DB.QueryRow(ctx, `SELECT payload->>'operationId' FROM plugin_v2_event_journal WHERE event_id=$1`, event).Scan(&operation); e != nil || operation != plan.ID {
		t.Fatal("business operation identity lost after versioned fact", operation, e)
	}
	if _, e = s.Replay(ctx, "admin", in.ID, 0, 200, false); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE instance_id=$1 AND kind='event'`, in.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal("duplicate delivery", n, e)
	}

	// Config generation changes cancel old definitions; interception skips the module.
	current, err := s.List(ctx, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Apply(ctx, "admin", pluginv2.Request{Kind: pluginv2.Configure, OperationKey: "intercept-config", InstanceID: in.ID, Fingerprint: d.Fingerprint, ExpectedRevision: current[0].Revision, Confirm: true, Config: json.RawMessage(`{"scheduled":false,"recognizeTitle":"Intercepted","moduleTitle":"Must not run","intercept":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	wait("intercept generation", func() bool { snap, err := s.Snapshot(ctx, in.ID); return err == nil && snap.Hash != "" })
	if status, b := request("POST", "/api/v1/media/recognize-name", true, map[string]string{"Origin": c.PublicURL, "X-Requested-With": "media-tool"}, `{"name":"Owned.2026.mkv"}`); status != 200 || !strings.Contains(string(b), "Intercepted") {
		t.Fatal("interception", status, string(b))
	}

	// Failure, malformed schema and a slow handler all degrade to built-in parse.
	for _, config := range []string{`{"scheduled":false,"chainError":true}`, `{"scheduled":false,"recognizeTitle":"` + strings.Repeat("x", 513) + `"}`, `{"scheduled":false,"chainDelay":true}`} {
		current, err := s.List(ctx, 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Apply(ctx, "admin", pluginv2.Request{Kind: pluginv2.Configure, OperationKey: jobs.UUID(), InstanceID: in.ID, Fingerprint: d.Fingerprint, ExpectedRevision: current[0].Revision, Confirm: true, Config: json.RawMessage(config)}); err != nil {
			t.Fatal(err)
		}
		wait("fallback generation", func() bool { _, err := s.Snapshot(ctx, in.ID); return err == nil })
		start := time.Now()
		status, b := request("POST", "/api/v1/media/recognize-name", true, map[string]string{"Origin": c.PublicURL, "X-Requested-With": "media-tool"}, `{"name":"Owned.2026.mkv"}`)
		if status != 200 || !strings.Contains(string(b), `"title":"Owned"`) || time.Since(start) > 2500*time.Millisecond {
			t.Fatal("bounded builtin fallback", status, string(b), time.Since(start))
		}
	}
	items, e := s.List(ctx, 0, 50)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(ctx, "admin", pluginv2.Request{Kind: pluginv2.Disable, OperationKey: "real-http-disable", InstanceID: in.ID, Fingerprint: d.Fingerprint, ExpectedRevision: items[0].Revision, Confirm: true}); e != nil {
		t.Fatal(e)
	}
	if status, _ := request("POST", path+"/admin", true, map[string]string{"Origin": c.PublicURL, "X-Requested-With": "media-tool"}, `{}`); status < 400 {
		t.Fatal("disabled admin API still callable", status)
	}
	if status, _ := request("GET", path+"/public", false, nil, ""); status < 400 {
		t.Fatal("disabled plugin still callable", status)
	}

	wait("worker fully reaped", func() bool {
		var clean bool
		return s.DB.QueryRow(ctx, `SELECT worker_pid=0 AND generation IS NULL AND owner_id IS NULL AND lease_until IS NULL FROM plugin_v2_instances WHERE id=$1`, in.ID).Scan(&clean) == nil && clean
	})
	other := jobs.UUID()
	logger.Info("other-instance-fixture", "module", "plugins", "instance", other)
	logPage, err := logs.Query(ctx, logging.Query{Page: 1, PageSize: 200, Module: "plugins", Instance: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	observed := map[string]bool{}
	for _, row := range logPage.Items {
		if row["instance"] != in.ID || row["module"] != "plugins" {
			t.Fatal("cross-instance log leak", row)
		}
		message, _ := row["msg"].(string)
		observed[message] = true
		if strings.HasPrefix(message, "Plugin tool call") {
			if _, ok := row["runId"].(string); !ok {
				t.Fatal("missing tool run identity")
			}
			if message == "Plugin tool call finished" && (row["result"] != "succeeded" || row["durationMs"] == nil) {
				t.Fatal("missing bounded tool result/timing metadata", row)
			}
		}

		raw, _ := json.Marshal(row)
		if strings.Contains(string(raw), "http-worker") || strings.Contains(string(raw), "http-tool-echo") {
			t.Fatal("tool input or operation key leaked", string(raw))
		}
	}
	for _, message := range []string{"Plugin operation committed", "Plugin worker ready", "Plugin worker reaped", "Plugin tool call started", "Plugin tool call finished"} {
		if !observed[message] {
			t.Fatal("missing INFO lifecycle/tool log", message)
		}
	}
	isolated, err := logs.Query(ctx, logging.Query{Page: 1, PageSize: 50, Module: "plugins", Instance: other})
	if err != nil || isolated.Total != 1 {
		t.Fatal("exact instance filter", isolated, err)
	}
	t.Log("native real worker NameRecognize -> module/intercept; form/page/multi-dashboard; real HTTP -> same-binary worker API; durable service -> KV; real organize -> TransferComplete; immediate disable executed")
}
