package business

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/messaging"
	"media-tool/internal/modules/networkproxy"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/pluginactor"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func platformFixture(t *testing.T) (AcquisitionHost, context.Context) {
	t.Helper()
	h, ctx := serverFixture(t)
	for _, table := range []string{"plugin_v2_manual_requests", "plugin_v2_runs", "notification_messages", "notification_deliveries", "ops_alerts", "goose_db_version"} {
		if _, e := h.DB.Exec(ctx, `CREATE TABLE `+pgx.Identifier{table}.Sanitize()+`(LIKE public.`+pgx.Identifier{table}.Sanitize()+` INCLUDING ALL)`); e != nil {
			t.Fatal(e)
		}
	}
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Fatal("Redis DB15 required")
	}
	opt, e := redis.ParseURL(raw)
	if e != nil || opt.DB != 15 {
		t.Fatal("Redis DB15 only")
	}
	h.Redis = redis.NewClient(opt)
	t.Cleanup(func() { _ = h.Redis.Close() })
	h.Network = networkproxy.NewHostClient(networkproxy.Repository{DB: h.DB, Vault: h.Vault})
	return h, ctx
}
func platformCall(t *testing.T, h AcquisitionHost, instance, method string, in any, p contracts.Permission, rev ...int64) (any, error) {
	t.Helper()
	spec, _ := b.Lookup(method)
	if p.Scope == "" {
		p.Scope = spec.Scope
	}
	raw, e := json.Marshal(in)
	if e != nil {
		t.Fatal(e)
	}
	call := contracts.HostCall{Method: method, Args: raw, OperationKey: jobs.UUID()}
	if len(rev) > 0 {
		call.ExpectedRevision = &rev[0]
	}
	return h.Call(pluginactor.With(context.Background(), instance, p.Scope, nil), call, p, func() error { return nil })
}
func requirePlatform[T any](t *testing.T, h AcquisitionHost, instance, method string, in any, rev ...int64) T {
	t.Helper()
	v, e := platformCall(t, h, instance, method, in, contracts.Permission{}, rev...)
	if e != nil {
		t.Fatalf("%s: %v", method, e)
	}
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	assertServerSafe(t, raw)
	return copyDTO[T](v)
}
func TestPlatformSettingsIntegrationsHealthAndSecretSentinels(t *testing.T) {
	h, ctx := platformFixture(t)
	instance := jobs.UUID()
	page := b.PageRequest{Page: 1, Size: 200}
	connection, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "local", Name: "token=SECRET_CANARY", Enabled: true, Settings: map[string]any{"rootPath": t.TempDir()}, Secrets: map[string]string{}})
	if e != nil {
		t.Fatal(e)
	}
	requirePlatform[b.Page[b.Connection]](t, h, instance, "integrations.list", page)
	requirePlatform[b.Connection](t, h, instance, "integrations.status", b.ID{ID: connection.ID})
	requirePlatform[b.ConnectionCheck](t, h, instance, "integrations.test", b.ID{ID: connection.ID})
	for _, method := range []string{"system.health", "maintenance.health"} {
		requirePlatform[b.HealthSummary](t, h, instance, method, b.Empty{})
	}
	requirePlatform[b.SystemInfo](t, h, instance, "system.info", b.Empty{})
	requirePlatform[[]b.SystemModule](t, h, instance, "system.modules", b.Empty{})
	requirePlatform[b.ProxySummary](t, h, instance, "networkproxy.status", b.Empty{})
	if _, e = h.DB.Exec(ctx, `INSERT INTO ops_alerts(code,state,message,hits)VALUES('disk','active','SECRET_CANARY /data/private https://secret.invalid',3)`); e != nil {
		t.Fatal(e)
	}
	requirePlatform[b.Page[b.AlertSummary]](t, h, instance, "maintenance.alerts", page)
	current := requirePlatform[b.SettingsView](t, h, instance, "settings.get", b.SettingKeys{Keys: []string{"searchCacheSeconds", "tmdbCacheSeconds"}})
	patch := b.SettingPatch{Revision: current.Revision, Values: map[string]json.RawMessage{"searchCacheSeconds": json.RawMessage(`600`)}}
	updated := requirePlatform[b.SettingsView](t, h, instance, "settings.patch", patch, current.Revision)
	if updated.Revision != current.Revision+1 {
		t.Fatal(updated)
	}
	for _, key := range []string{"access", "masterKey", "strmApiKey", "trustedProxyCidrs", "credentials", "open115"} {
		_, e = platformCall(t, h, instance, "settings.get", b.SettingKeys{Keys: []string{key}}, contracts.Permission{})
		mediaCode(t, e, contracts.PermissionDenied)
		_, e = platformCall(t, h, instance, "settings.patch", b.SettingPatch{Revision: updated.Revision, Values: map[string]json.RawMessage{key: json.RawMessage(`"SECRET_CANARY"`)}}, contracts.Permission{}, updated.Revision)
		mediaCode(t, e, contracts.PermissionDenied)
	}
	_, e = platformCall(t, h, instance, "settings.get", b.SettingKeys{Keys: []string{"tmdbCacheSeconds"}}, contracts.Permission{Resources: map[string][]string{"settingKeys": {"searchCacheSeconds"}}})
	mediaCode(t, e, contracts.PermissionDenied)
	denied := dbutil.WithCommitGuard(pluginactor.With(ctx, instance, "settings.write", nil), func(context.Context, pgx.Tx) error { return contracts.NewError(contracts.PermissionDenied) })
	raw, _ := json.Marshal(b.SettingPatch{Revision: updated.Revision, Values: map[string]json.RawMessage{"searchCacheSeconds": json.RawMessage(`500`)}})
	_, e = h.Call(denied, contracts.HostCall{Method: "settings.patch", Args: raw, ExpectedRevision: &updated.Revision}, contracts.Permission{Scope: "settings.write"}, func() error { return nil })
	if e == nil {
		t.Fatal("revoked settings committed")
	}
}
func TestPlatformOwnedJobsNotificationsAndUnknownRetry(t *testing.T) {
	h, ctx := platformFixture(t)
	instance, other := jobs.UUID(), jobs.UUID()
	page := b.PageRequest{Page: 1, Size: 200}
	d := messaging.Dispatcher{DB: h.DB, Connections: h.Connections}
	for _, owner := range []string{instance, other} {
		id := jobs.UUID()
		payload, _ := json.Marshal(map[string]string{"instanceId": owner, "serviceId": "tick", "secret": "SECRET_CANARY"})
		if _, e := h.DB.Exec(ctx, `INSERT INTO jobs(id,kind,payload)VALUES($1,'plugin.v2.service',$2)`, id, payload); e != nil {
			t.Fatal(e)
		}
		if _, e := h.DB.Exec(ctx, `INSERT INTO plugin_v2_manual_requests(key,instance_id,request_hash,job_id)VALUES($1,$2,'hash',$3)`, id, owner, id); e != nil {
			t.Fatal(e)
		}
	}
	list := requirePlatform[b.Page[b.OwnedJob]](t, h, instance, "jobs.list", b.JobQuery{PageRequest: page})
	if len(list.Items) != 1 {
		t.Fatal(list)
	}
	id := list.Items[0].ID
	requirePlatform[b.OwnedJob](t, h, instance, "jobs.get", b.ID{ID: id})
	if _, e := h.DB.Exec(ctx, `INSERT INTO plugin_v2_runs(id,instance_id,snapshot_id,generation,grant_revision,kind,handler,dedupe_key,request_hash,args_ciphertext,deadline_at)VALUES($1,$2,$3,$4,1,'service','fixture',$5,'hash',decode('01','hex'),now()+interval '1 hour')`, jobs.UUID(), instance, jobs.UUID(), jobs.UUID(), "service-job:"+id); e != nil {
		t.Fatal(e)
	}

	_, e := platformCall(t, h, other, "jobs.cancel", b.ID{ID: id}, contracts.Permission{})
	if e == nil {
		t.Fatal("cross-owner cancel")
	}
	requirePlatform[b.Receipt](t, h, instance, "jobs.cancel", b.ID{ID: id})
	var runState string
	if e = h.DB.QueryRow(ctx, `SELECT state FROM plugin_v2_runs WHERE dedupe_key=$1`, "service-job:"+id).Scan(&runState); e != nil || runState != "canceled" {
		t.Fatal(runState, e)
	}

	first := requirePlatform[b.Receipt](t, h, instance, "notifications.publish", b.Notice{Title: "fixture", Text: "local queued notice"})
	if first.ID == "" {
		t.Fatal(first)
	}
	telegram, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "telegram", Name: "fixture", Enabled: true, Settings: map[string]any{"chatId": "42"}, Secrets: map[string]string{"botToken": "123:SECRET_CANARY"}})
	if e != nil {
		t.Fatal(e)
	}
	if e = d.Prepare(ctx); e != nil {
		t.Fatal(e)
	}
	listDelivery := requirePlatform[b.Page[b.DeliverySummary]](t, h, instance, "notifications.deliveries", page)
	if len(listDelivery.Items) != 1 {
		t.Fatal(listDelivery)
	}
	delivery := listDelivery.Items[0].ID
	if _, e = h.DB.Exec(ctx, `UPDATE notification_deliveries SET state='outcome_unknown',last_error='SECRET_CANARY'WHERE id=$1`, delivery); e != nil {
		t.Fatal(e)
	}
	_, e = platformCall(t, h, other, "notifications.retry", b.DeliveryRetry{ID: delivery, AcceptDuplicateRisk: true}, contracts.Permission{})
	if e == nil {
		t.Fatal("cross-owner resend")
	}
	_, e = platformCall(t, h, instance, "notifications.retry", b.DeliveryRetry{ID: delivery}, contracts.Permission{})
	if e == nil {
		t.Fatal("unknown outcome retried without acceptance")
	}
	requirePlatform[b.Receipt](t, h, instance, "notifications.retry", b.DeliveryRetry{ID: delivery, AcceptDuplicateRisk: true})
	var target string
	if e = h.DB.QueryRow(ctx, `SELECT integration_id::text FROM notification_deliveries WHERE id=$1`, delivery).Scan(&target); e != nil || target != telegram.ID {
		t.Fatal(target, e)
	}
	requirePlatform[b.Page[b.DeliverySummary]](t, h, other, "notifications.deliveries", page)
}
func TestPlatformCacheNamespaceNoRawValuesAndFencedInvalidate(t *testing.T) {
	h, ctx := platformFixture(t)
	instance := jobs.UUID()
	var random [32]byte
	if _, e := rand.Read(random[:]); e != nil {
		t.Fatal(e)
	}
	id := hex.EncodeToString(random[:])
	key := "media-tool:tmdb-cache:" + id
	if e := h.Redis.Set(ctx, key, `{"query":"SECRET_CANARY","token":"SECRET_CANARY"}`, time.Minute).Err(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = h.Redis.Del(ctx, key).Err() })
	q := b.NamespaceQuery{Namespace: "tmdb", PageRequest: b.PageRequest{Page: 1, Size: 100}}
	requirePlatform[b.CacheSummary](t, h, instance, "cache.status", q)
	requirePlatform[b.CachePage](t, h, instance, "cache.list", q)
	for _, namespace := range []string{"*", "all", "sessions", "playback", "media-tool:tmdb-cache:"} {
		_, e := platformCall(t, h, instance, "cache.invalidate", b.CacheInvalidation{Namespace: namespace, IDs: []string{id}}, contracts.Permission{})
		mediaCode(t, e, contracts.InvalidParams)
	}
	denied := dbutil.WithEffectGuard(pluginactor.With(ctx, instance, "cache.invalidate", nil), func() error { return contracts.NewError(contracts.PermissionDenied) })
	raw, _ := json.Marshal(b.CacheInvalidation{Namespace: "tmdb", IDs: []string{id}})
	_, e := h.Call(denied, contracts.HostCall{Method: "cache.invalidate", Args: raw}, contracts.Permission{Scope: "cache.invalidate"}, func() error { return nil })
	if e == nil {
		t.Fatal("revoked redis delete")
	}
	if h.Redis.Exists(ctx, key).Val() != 1 {
		t.Fatal("unfenced delete")
	}
	requirePlatform[b.Count](t, h, instance, "cache.invalidate", b.CacheInvalidation{Namespace: "tmdb", IDs: []string{id}})
	if h.Redis.Exists(ctx, key).Val() != 0 {
		t.Fatal("not deleted")
	}
}
func TestPlatformHostHTTPExactOriginProxyNoBodyAndSSRF(t *testing.T) {
	h, ctx := platformFixture(t)
	var requests int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if strings.Contains(r.URL.Path, "redirect") {
			w.Header().Set("Location", "http://127.0.0.1/SECRET_CANARY")
			w.WriteHeader(302)
		}
		w.Header().Set("Set-Cookie", "SECRET_CANARY")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, strings.Repeat("SECRET_CANARY", 10000))
	}))
	defer proxy.Close()
	_, e := (networkproxy.Repository{DB: h.DB, Vault: h.Vault}).Save(ctx, networkproxy.Record{URL: strings.Replace(proxy.URL, "://", "://proxyuser:SECRET_CANARY@", 1), Revision: 1})
	if e != nil {
		t.Fatal(e)
	}
	h.Network.Policy.Lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	p := contracts.Permission{Scope: "network.proxy.use", Resources: map[string][]string{"hosts": {"http://public.fixture"}}}
	instance := jobs.UUID()
	for _, path := range []string{"/safe", "/redirect"} {
		v, e := platformCall(t, h, instance, "network.http", b.HostRequest{Profile: "host", Origin: "http://public.fixture", Path: path, Method: "GET"}, p)
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(v)
		assertServerSafe(t, raw)
	}
	if requests != 2 {
		t.Fatal("redirect followed", requests)
	}
	for _, in := range []b.HostRequest{{Profile: "direct", Origin: "http://public.fixture", Path: "/safe", Method: "GET"}, {Profile: "host", Origin: "http://public.fixture", Path: "//private.invalid", Method: "GET"}, {Profile: "host", Origin: "http://127.0.0.1", Path: "/safe", Method: "GET"}, {Profile: "host", Origin: "http://public.fixture", Path: "/safe", Method: "POST"}} {
		if _, e = platformCall(t, h, instance, "network.http", in, p); e == nil {
			t.Fatal("unsafe request accepted", in)
		}
	}
	h.Network.Policy.Lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	_, e = platformCall(t, h, instance, "network.http", b.HostRequest{Profile: "host", Origin: "http://public.fixture", Path: "/safe", Method: "GET"}, p)
	if e == nil {
		t.Fatal("private DNS accepted")
	}
}
