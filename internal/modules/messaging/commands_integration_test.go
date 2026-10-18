package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"media-tool/internal/adapters/telegram"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/platform/config"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type ingressTransport func(*http.Request) (*http.Response, error)

func (f ingressTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type ingressFixture struct {
	s          pluginv2.Store
	i          CommandIngress
	manager    *integrations.Manager
	c          integrations.Credentials
	l          pluginv2.Lease
	ctx        context.Context
	instance   pluginv2.Instance
	definition pluginv2.Definition
}

func newIngressFixture(t *testing.T, claim bool) ingressFixture {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated media_tool_test required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("only media_tool_test")
	}
	base, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	schema := "ingress_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	if _, e = base.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	tables := []string{"app_settings", "integrations", "audit_logs", "outbox_events", "jobs", "notification_messages", "notification_deliveries", "plugin_v2_definitions", "plugin_v2_instances", "plugin_v2_configs", "plugin_v2_kv", "plugin_v2_grants", "plugin_v2_operations", "plugin_v2_snapshots", "plugin_v2_extensions", "plugin_v2_schedules", "plugin_v2_event_journal", "plugin_v2_subscriptions", "plugin_v2_runs", "plugin_v2_api_keys", "plugin_v2_api_limits", "plugin_v2_manual_requests"}
	for _, table := range tables {
		if _, e = base.Exec(ctx, `CREATE TABLE `+pgx.Identifier{schema, table}.Sanitize()+` (LIKE public.`+table+` INCLUDING ALL)`); e != nil {
			t.Fatal(e)
		}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 4
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := pool.Exec(ctx, `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`); e != nil {
			t.Error(e)
		}
		pool.Close()
	})
	vault, e := secrets.New(strings.Repeat("42", 32))
	if e != nil {
		t.Fatal(e)
	}
	s := pluginv2.Store{DB: pool, Vault: vault, Root: filepath.Join(t.TempDir(), "plugins")}
	repo := integrations.Repository{DB: pool, Vault: vault}
	manager := integrations.NewManager(repo, config.Config{})
	t.Cleanup(manager.Close)
	saved, e := repo.Save(ctx, "", integrations.Input{Kind: "telegram", Name: "Owned fixture", Enabled: true, Settings: map[string]any{"chatId": "10", "inboundEnabled": true, "allowedChatIds": "10", "allowedUserIds": "20"}, Secrets: map[string]string{"botToken": "BOT_TOKEN_PRIVATE_SENTINEL"}})
	if e != nil {
		t.Fatal(e)
	}
	c, e := repo.Get(ctx, saved.ID)
	if e != nil {
		t.Fatal(e)
	}
	defs, e := pluginv2.Catalog()
	if e != nil {
		t.Fatal(e)
	}
	d := defs[0]
	scopes := []string{}
	for _, p := range d.Manifest.Permissions {
		scopes = append(scopes, p.Scope)
	}
	in, e := s.Apply(ctx, "fixture", pluginv2.Request{Kind: pluginv2.Install, PluginID: d.Manifest.ID, Scopes: scopes, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Config: json.RawMessage(`{"scheduled":false}`), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	in, e = s.Apply(ctx, "fixture", pluginv2.Request{Kind: pluginv2.Enable, InstanceID: in.ID, ExpectedRevision: in.Revision, Fingerprint: d.Fingerprint, OperationKey: jobs.UUID(), Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	var l pluginv2.Lease
	if claim {
		l, e = s.Claim(ctx, jobs.UUID())
		if e != nil {
			t.Fatal(e)
		}
		m := contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{{Kind: "command", ID: "hello", Cmd: "hello", Handler: contracts.HandlerID("command", "hello"), RequiredScopes: []string{"commands.register"}}}}
		if e = s.Publish(ctx, l, m); e != nil {
			t.Fatal(e)
		}
		if e = s.Renew(ctx, l, true); e != nil {
			t.Fatal(e)
		}
	}
	clock := time.Now().UTC()
	i := CommandIngress{DB: pool, Connections: manager, Plugins: s, Owner: jobs.UUID(), Clock: func() time.Time { return clock }}
	return ingressFixture{s: s, i: i, manager: manager, c: c, l: l, ctx: ctx, instance: in, definition: d}
}
func (f ingressFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if e := f.s.DB.QueryRow(f.ctx, query, args...).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func message(id, chat, user int64, text string, now time.Time) telegram.Update {
	m := &telegram.Inbound{Date: now.Unix(), Text: text, From: telegram.Sender{ID: user}}
	m.Chat.ID = chat
	return telegram.Update{ID: id, Message: m}
}
func (f ingressFixture) transport(t *testing.T, updates *[]telegram.Update, calls *int) {
	t.Helper()
	f.manager.OutboundHTTP = &http.Client{Transport: ingressTransport(func(r *http.Request) (*http.Response, error) {
		*calls++
		if r.URL.Host != "api.telegram.org" {
			t.Fatal("unexpected upstream")
		}
		if strings.HasSuffix(r.URL.Path, "/answerCallbackQuery") {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":true}`)), Header: http.Header{}}, nil
		}
		if !strings.HasSuffix(r.URL.Path, "/getUpdates") {
			t.Fatal("unexpected method")
		}
		var in struct {
			Offset  int64 `json:"offset"`
			Limit   int   `json:"limit"`
			Timeout int   `json:"timeout"`
		}
		if e := json.NewDecoder(r.Body).Decode(&in); e != nil || in.Limit != 10 || in.Timeout != 15 {
			t.Fatal(in, e)
		}
		raw, _ := json.Marshal(map[string]any{"ok": true, "result": *updates})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw))), Header: http.Header{}}, nil
	})}
}
func TestIngressAllowlistLimitsDurableOffsetAndLiveCommandScopes(t *testing.T) {
	f := newIngressFixture(t, true)
	updates := []telegram.Update{}
	calls := 0
	f.transport(t, &updates, &calls)
	updates = []telegram.Update{message(1, 99, 20, "/hello", f.i.now()), message(2, 10, 99, "/hello", f.i.now()), message(3, 10, 20, "/not_registered", f.i.now()), message(4, 10, 20, "/hello "+strings.Repeat("x", 201), f.i.now()), message(5, 10, 20, "/hello", f.i.now().Add(-time.Hour))}
	if _, e := f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs`) != 0 {
		t.Fatal("untrusted ingress")
	}
	updates = []telegram.Update{message(6, 10, 20, "/hello password=NEVER_PLUGIN_SECRET https://private/secret", f.i.now())}
	if _, e := f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs WHERE kind='command'`) != 1 || f.count(t, `SELECT count(*) FROM outbox_events WHERE event_type='messaging.inbound'`) != 1 {
		t.Fatal("command not queued atomically")
	}
	var id string
	var cipher []byte
	if e := f.s.DB.QueryRow(f.ctx, `SELECT id::text,args_ciphertext FROM plugin_v2_runs`).Scan(&id, &cipher); e != nil {
		t.Fatal(e)
	}
	values, e := f.s.Vault.Open("plugin:v2:call-args:"+id, cipher)
	if e != nil || strings.Contains(values["value"], "NEVER_PLUGIN_SECRET") || strings.Contains(values["value"], "https://") {
		t.Fatal("command secret leaked", e)
	}
	restarted := f.i
	restarted.Owner = jobs.UUID()
	if _, e = restarted.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs WHERE kind='command'`) != 1 {
		t.Fatal("restart re-executed update_id")
	}
	updates = []telegram.Update{}
	for id := int64(7); id < 17; id++ {
		updates = append(updates, message(id, 10, 20, "/hello", f.i.now()))
	}
	if _, e = f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs WHERE kind='command'`) != 5 {
		t.Fatal("per-actor 5/min exceeded")
	}
	if _, e = f.s.DB.Exec(f.ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='commands.register'`, f.l.ID); e != nil {
		t.Fatal(e)
	}
	updates = []telegram.Update{message(17, 10, 20, "/hello", f.i.now())}
	if _, e = f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs`) != 5 {
		t.Fatal("revoked command routed")
	}
	var raw []byte
	if e = f.s.DB.QueryRow(f.ctx, `SELECT jsonb_agg(payload) FROM outbox_events`).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "BOT_TOKEN") || strings.Contains(string(raw), "chatId") || strings.Contains(string(raw), "NEVER_PLUGIN_SECRET") {
		t.Fatal("event payload leaked")
	}
}
func TestIngressDisabledEmptyAllowlistsAndPollLeaseMakeNoExternalConnection(t *testing.T) {
	f := newIngressFixture(t, true)
	updates := []telegram.Update{}
	calls := 0
	f.transport(t, &updates, &calls)
	for _, settings := range []string{`{"chatId":"10"}`, `{"chatId":"10","inboundEnabled":true,"allowedChatIds":"10","allowedUserIds":""}`, `{"chatId":"10","inboundEnabled":true,"allowedChatIds":"","allowedUserIds":"20"}`} {
		if _, e := f.s.DB.Exec(f.ctx, `UPDATE integrations SET settings=$2,revision=revision+1 WHERE id=$1`, f.c.Record.ID, json.RawMessage(settings)); e != nil {
			t.Fatal(e)
		}
		if active, e := f.i.PollOnce(f.ctx); e != nil || active || calls != 0 {
			t.Fatal("unconfigured poll connected", active, calls, e)
		}
	}
	if _, e := f.s.DB.Exec(f.ctx, `UPDATE integrations SET settings=$2,revision=revision+1 WHERE id=$1`, f.c.Record.ID, json.RawMessage(`{"chatId":"10","inboundEnabled":true,"allowedChatIds":"10","allowedUserIds":"20"}`)); e != nil {
		t.Fatal(e)
	}
	c, e := f.i.configuration(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, ok, e := f.i.acquire(f.ctx, c); e != nil || !ok {
		t.Fatal(e)
	}
	other := f.i
	other.Owner = jobs.UUID()
	if _, e = other.PollOnce(f.ctx); e != nil || calls != 0 {
		t.Fatal("two poll owners", calls, e)
	}
	f.i.release(f.ctx, c)
	if _, e = f.s.DB.Exec(f.ctx, `DELETE FROM integrations`); e != nil {
		t.Fatal(e)
	}
	if active, e := f.i.PollOnce(f.ctx); e != nil || active || calls != 0 {
		t.Fatal("missing Telegram connected")
	}
}
func TestIngressCallbackSignatureExpiryOneUseAndTargetBinding(t *testing.T) {
	f := newIngressFixture(t, true)
	updates := []telegram.Update{}
	calls := 0
	f.transport(t, &updates, &calls)
	data, e := f.i.IssueAction(f.ctx, 10, 20, "hello", "demo")
	if e != nil || len(data) > 64 || strings.Contains(data, "BOT_TOKEN") {
		t.Fatal(data, e)
	}
	callback := func(id int64, user int64, value string) telegram.Update {
		u := message(id, 10, user, "", f.i.now())
		return telegram.Update{ID: id, Callback: &telegram.Callback{ID: "fixture", From: telegram.Sender{ID: user}, Message: u.Message, Data: value}}
	}
	updates = []telegram.Update{callback(1, 99, data), callback(2, 20, data+"x")}
	if _, e = f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs`) != 0 {
		t.Fatal("invalid callback routed")
	}
	updates = []telegram.Update{callback(3, 20, data)}
	if _, e = f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	updates = []telegram.Update{callback(4, 20, data)}
	if _, e = f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs`) != 1 || f.count(t, `SELECT count(*) FROM outbox_events WHERE event_type='messaging.action.received'`) != 1 {
		t.Fatal("nonce replay")
	}
	expired, e := f.i.IssueAction(f.ctx, 10, 20, "hello", "")
	if e != nil {
		t.Fatal(e)
	}
	parts := strings.Split(expired, ".")
	parts[2] = "1"
	body := strings.Join(parts[:3], ".")
	updates = []telegram.Update{callback(5, 20, body+"."+actionMAC(f.c, body))}
	if _, e = f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs`) != 1 {
		t.Fatal("expired nonce routed")
	}
	live, e := f.i.IssueAction(f.ctx, 10, 20, "hello", "")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DB.Exec(f.ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, f.c.Record.ID); e != nil {
		t.Fatal(e)
	}
	updates = []telegram.Update{callback(6, 20, live)}
	if _, e = f.i.PollOnce(f.ctx); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs`) != 1 {
		t.Fatal("changed configuration accepted old callback")
	}
}
func TestIngressCommitRollbackAndGlobalRateFence(t *testing.T) {
	f := newIngressFixture(t, true)
	if _, ok, e := f.i.acquire(f.ctx, f.c); e != nil || !ok {
		t.Fatal(e)
	}
	defer f.i.release(f.ctx, f.c)
	blocked := dbutil.WithCommitGuard(f.ctx, func(context.Context, pgx.Tx) error { return errors.New("rollback fixture") })
	u := message(1, 10, 20, "/hello", f.i.now())
	if e := f.i.Receive(blocked, f.c, u); e == nil {
		t.Fatal("commit fence should reject")
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs`) != 0 || f.count(t, `SELECT count(*) FROM outbox_events WHERE event_type='messaging.inbound'`) != 0 {
		t.Fatal("rolled back command emitted")
	}
	if e := f.i.Receive(f.ctx, f.c, u); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs`) != 1 {
		t.Fatal("rolled back offset was advanced")
	}
	if _, e := f.s.DB.Exec(f.ctx, `UPDATE app_settings SET value=jsonb_set(value,'{used}','60'::jsonb) WHERE key=$1`, pollKey(f.c)); e != nil {
		t.Fatal(e)
	}
	if e := f.i.Receive(f.ctx, f.c, message(2, 10, 20, "/hello", f.i.now())); e != nil {
		t.Fatal(e)
	}
	if f.count(t, `SELECT count(*) FROM plugin_v2_runs`) != 1 {
		t.Fatal("global rate limit bypassed")
	}
}
