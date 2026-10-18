package business

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/settings"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func hostFixture(t *testing.T) (AcquisitionHost, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("media_tool_test only")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated database required")
	}
	ctx := context.Background()
	base, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	schema := "f3a_" + strings.ReplaceAll(jobs.UUID(), "-", "")
	if _, e = base.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	tables := []string{"integrations", "site_definitions", "site_template_loads", "site_metrics", "site_metric_schedule", "application_settings", "network_proxy_settings", "search_runs", "search_site_results", "search_run_resources", "resources", "download_tasks", "download_files", "transfer_tasks", "manual_download_tasks", "download_cleanup_requests", "subscriptions", "subscription_runs", "jobs", "outbox_events", "audit_logs"}
	for _, name := range tables {
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
		if _, e := db.Exec(ctx, `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`); e != nil {
			t.Error(e)
		}
		db.Close()
	})
	vault, e := secrets.New(strings.Repeat("a", 64))
	if e != nil {
		t.Fatal(e)
	}
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{})
	t.Cleanup(m.Close)
	if _, e = db.Exec(ctx, `INSERT INTO network_proxy_settings(id)VALUES('main')`); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(settings.Defaults())
	if _, e = db.Exec(ctx, `INSERT INTO application_settings(singleton,value,revision)VALUES(true,$1,1)`, raw); e != nil {
		t.Fatal(e)
	}
	return AcquisitionHost{DB: db, Vault: vault, Connections: m}, ctx
}
func TestAcquisitionHostRealFacadesBoundedSQLRedactionAndMutations(t *testing.T) {
	h, ctx := hostFixture(t)
	var stopped atomic.Int32
	var deleteFiles []string
	var ownedDeleted atomic.Bool
	ownedHash := strings.Repeat("a", 40)
	foreignHash := strings.Repeat("b", 40)
	ownedTask := jobs.UUID()
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = io.WriteString(w, "Ok.")
		case "/api/v2/app/version":
			_, _ = io.WriteString(w, "v5.0")
		case "/api/v2/torrents/stop", "/api/v2/torrents/start":
			stopped.Add(1)
			_, _ = io.WriteString(w, "")
		case "/api/v2/torrents/delete":
			_ = r.ParseForm()
			deleteFiles = append(deleteFiles, r.Form.Get("deleteFiles"))
			ownedDeleted.Store(true)
			_, _ = io.WriteString(w, "")
		case "/api/v2/torrents/info":
			if ownedDeleted.Load() && r.URL.Query().Get("hashes") == ownedHash {
				_, _ = io.WriteString(w, "[]")
				return
			}
			hash := r.URL.Query().Get("hashes")
			if hash == "" {
				hash = ownedHash
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{"hash": hash, "name": "Owned https://tracker.invalid?passkey=LEAK", "state": "stoppedDL", "progress": .5, "save_path": "/downloads", "content_path": "/downloads/Owned", "category": "media-tool", "tags": ownedTask, "size": 4}})
		case "/api/v2/torrents/files":
			_, _ = io.WriteString(w, `[{"index":0,"name":"Owned.mkv","size":4,"progress":0.5}]`)
		case "/api/v2/torrents/trackers":
			_, _ = io.WriteString(w, `[{"url":"https://tracker.invalid/LEAK/announce?passkey=LEAK","status":2,"msg":"token=LEAK"}]`)
		default:
			t.Errorf("unexpected QB %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer qb.Close()
	q, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "qbittorrent", Name: "fixture", Enabled: true, Settings: map[string]any{"endpoint": qb.URL, "downloadPath": "/downloads", "localPath": t.TempDir(), "category": "media-tool"}, Secrets: map[string]string{"username": "USER_SECRET", "password": "PASSWORD_SECRET"}})
	if e != nil {
		t.Fatal(e)
	}
	resource := jobs.UUID()
	if _, e = h.DB.Exec(ctx, `INSERT INTO resources(id,account_id,torrent_id,title,size_bytes,seeders,leechers,public_metadata,download_hint_ciphertext)VALUES($1,$2,'1','Owned',4,1,0,'{}',''::bytea)`, resource, q.ID); e != nil {
		t.Fatal(e)
	}
	details, _ := json.Marshal(map[string]any{"endpoint": qb.URL, "downloadPath": "/downloads", "localPath": "/host-private", "category": "media-tool", "media": map[string]any{"title": "Owned"}})
	if _, e = h.DB.Exec(ctx, `INSERT INTO download_tasks(id,resource_id,integration_id,media_type,tmdb_id,info_hash,details,download_state)VALUES($1,$2,$3,'movie',1,$4,$5,'downloading')`, ownedTask, resource, q.ID, ownedHash, details); e != nil {
		t.Fatal(e)
	}
	deniedArgs, _ := json.Marshal(b.AddResource{RunID: jobs.UUID(), ResourceID: resource, TMDBID: 1, MediaType: "movie"})
	if _, e := h.Call(ctx, contracts.HostCall{Method: "downloads.addResource", Args: deniedArgs}, contracts.Permission{Resources: map[string][]string{"downloaderIds": {jobs.UUID()}}}, func() error { return nil }); e == nil {
		t.Fatal("unselected default downloader admitted")
	}
	manualArgs, _ := json.Marshal(b.ManualSource{Magnet: "magnet:?xt=urn:btih:" + strings.Repeat("a", 40)})
	if _, e := h.Call(ctx, contracts.HostCall{Method: "downloads.manualCreate", Args: manualArgs}, contracts.Permission{Resources: map[string][]string{"siteIds": {jobs.UUID()}}}, func() error { return nil }); e == nil {
		t.Fatal("unattributed manual source bypassed site selector")
	}
	calls := 0
	call := func(method string, in any) (any, error) {
		t.Helper()
		raw, _ := json.Marshal(in)
		spec, _ := b.Lookup(method)
		return h.Call(ctx, contracts.HostCall{Method: method, Args: raw}, contracts.Permission{Scope: spec.Scope}, func() error { calls++; return nil })
	}
	for _, tt := range []struct {
		method string
		in     any
	}{{"downloads.list", b.PageRequest{Page: 1, Size: 1}}, {"downloads.get", b.ID{ID: ownedTask}}, {"downloads.progress", b.ID{ID: ownedTask}}, {"downloads.files", b.Selection{ID: ownedTask, PageRequest: b.PageRequest{Page: 1, Size: 1}}}, {"downloads.trackers", b.ID{ID: ownedTask}}, {"downloads.cleanup.preview", b.ID{ID: ownedTask}}, {"downloads.downloader.allHashes", b.PageRequest{Page: 1, Size: 1}}, {"downloads.downloader.inspect", b.ID{ID: foreignHash}}, {"downloads.downloader.setPaused", b.Paused{Hash: ownedHash, Paused: true}}} {
		v, e := call(tt.method, tt.in)
		if e != nil {
			t.Fatalf("%s: %v", tt.method, e)
		}
		raw, _ := json.Marshal(v)
		for _, secret := range []string{"LEAK", "PASSWORD_SECRET", "USER_SECRET", "/host-private", "/downloads"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("credential/path leak", tt.method, string(raw))
			}
		}
	}
	if stopped.Load() != 1 {
		t.Fatal("owned pause not issued")
	}
	if _, e = call("downloads.downloader.setPaused", b.Paused{Hash: foreignHash, Paused: true}); e == nil || stopped.Load() != 1 {
		t.Fatal("foreign torrent mutated", e)
	}
	if _, e = call("downloads.downloader.deleteOwned", b.DeleteRequest{ID: ownedHash, DeleteFiles: true}); e == nil {
		t.Fatal("non-imported or unknown HR admitted")
	}
	originalRun := jobs.UUID()
	if _, e = h.DB.Exec(ctx, `INSERT INTO search_runs(id,keyword,account_ids)VALUES($1,'Owned','[]')`, originalRun); e != nil {
		t.Fatal(e)
	}
	evidenceJSON := `{"metadata":{"hr":false}}`
	if _, e = h.DB.Exec(ctx, `INSERT INTO search_run_resources(run_id,resource_id,public_snapshot,download_hint_ciphertext,account_revision,definition_fingerprint,observed_at)VALUES($1,$2,$3,''::bytea,1,'fixture',now())`, originalRun, resource, evidenceJSON); e != nil {
		t.Fatal(e)
	}
	if _, e = h.DB.Exec(ctx, `UPDATE download_tasks SET download_state='imported',details=details||jsonb_build_object('searchRunId',$2::text)WHERE id=$1`, ownedTask, originalRun); e != nil {
		t.Fatal(e)
	}
	if _, e = call("downloads.downloader.deleteOwned", b.DeleteRequest{ID: ownedHash, DeleteFiles: true}); e != nil {
		t.Fatal(e)
	}
	var evidence string
	if e = h.DB.QueryRow(ctx, `SELECT evidence->>'hrSatisfied'FROM download_cleanup_requests WHERE task_id=$1`, ownedTask).Scan(&evidence); e != nil || evidence != "false" {
		t.Fatal("HR waiver", e, evidence)
	}
	if _, e = call("downloads.downloader.deleteOwned", b.DeleteRequest{ID: ownedHash, DeleteFiles: false}); e != nil {
		t.Fatal(e)
	}
	if len(deleteFiles) != 1 || deleteFiles[0] != "false" {
		t.Fatal("implicit payload delete", deleteFiles)
	}

	magnet := "magnet:?xt=urn:btih:" + strings.Repeat("c", 40) + "&dn=Owned&tr=https://tracker.invalid/LEAK"
	if _, e = call("downloads.manualCreate", b.ManualSource{Magnet: magnet}); e != nil {
		t.Fatal(e)
	}
	if _, e = call("downloads.manualList", b.PageRequest{Page: 1, Size: 200}); e != nil {
		t.Fatal(e)
	}
	if _, e = call("downloads.list", b.PageRequest{Page: 1, Size: 201}); e == nil {
		t.Fatal("unbounded")
	}
	if calls < 1 {
		t.Fatal("external admission not checked")
	}
}
func TestControlledSiteHostNeverExportsAuthAndTemplateSelection(t *testing.T) {
	h, ctx := hostFixture(t)
	if e := (sites.Repository{DB: h.DB}).LoadGoTemplates(ctx, []string{"hdsky"}); e != nil {
		t.Fatal(e)
	}
	var auth atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "session=COOKIE_SECRET" {
			auth.Add(1)
		}
		w.Header().Set("Set-Cookie", "COOKIE_RESPONSE_SECRET")
		_, _ = io.WriteString(w, "COOKIE_SECRET signed")
	}))
	defer fake.Close()
	account, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "owned", Enabled: true, Settings: map[string]any{"definitionId": "hdsky", "endpoint": fake.URL, "userId": "1"}, Secrets: map[string]string{"cookie": "session=COOKIE_SECRET"}})
	if e != nil {
		t.Fatal(e)
	}
	p := contracts.Permission{Scope: "sites.request", Resources: map[string][]string{"siteIds": {account.ID}, "hosts": {fake.URL}}}
	for _, method := range []string{"sites.request", "sites.checkin"} {
		raw, _ := json.Marshal(b.SiteRequest{AccountID: account.ID, Origin: fake.URL, Path: "/attendance.php", Method: "POST", SuccessMarker: "signed"})
		v, e := h.Call(ctx, contracts.HostCall{Method: method, Args: raw}, p, func() error { return nil })
		if e != nil {
			t.Fatal(e)
		}
		r := v.(b.SiteResponse)
		if !r.Matched || r.Body != "" {
			t.Fatal(r)
		}
	}
	if auth.Load() != 2 {
		t.Fatal("host did not attach account auth")
	}
	for _, method := range []string{"sites.list", "sites.templates.catalog", "sites.templates.loaded"} {
		raw, _ := json.Marshal(b.PageRequest{Page: 1, Size: 200})
		v, e := h.Call(ctx, contracts.HostCall{Method: method, Args: raw}, contracts.Permission{}, func() error { return nil })
		if e != nil {
			t.Fatal(method, e)
		}
		if method == "sites.list" {
			items := v.(b.Page[b.Site]).Items
			if len(items) != 1 || items[0].Origin != fake.URL || items[0].TemplateID != "hdsky" {
				t.Fatal("origin not usable", items)
			}
		}
		b, _ := json.Marshal(v)
		if strings.Contains(string(b), "COOKIE_SECRET") {
			t.Fatal("secret leaked")
		}
	}
}

func TestSearchAndSubscriptionFacadesSQLPagesRulesCASAndQueue(t *testing.T) {
	h, ctx := hostFixture(t)
	account := jobs.UUID()
	run := jobs.UUID()
	sub := jobs.UUID()
	if _, e := h.DB.Exec(ctx, `INSERT INTO search_runs(id,keyword,account_ids)VALUES($1,'Owned','[]')`, run); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 205; i++ {
		id := jobs.UUID()
		raw, _ := json.Marshal(map[string]any{"id": id, "accountId": account, "title": "Owned", "metadata": map[string]any{"hr": false, "downloadHint": "SECRET"}})
		if _, e := h.DB.Exec(ctx, `INSERT INTO search_run_resources(run_id,resource_id,public_snapshot,download_hint_ciphertext,account_revision,definition_fingerprint,observed_at)VALUES($1,$2,$3,''::bytea,1,'fixture',now())`, run, id, raw); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := h.DB.Exec(ctx, `INSERT INTO subscriptions(id,media_type,title,identity,enabled,rules,source,interval_seconds,next_run_at)VALUES($1,'movie','Owned','{"id":1,"mediaType":"movie","title":"Owned"}',true,'{"accountIds":[]}','search',1800,now())`, sub); e != nil {
		t.Fatal(e)
	}
	call := func(method string, in any) (any, error) {
		raw, _ := json.Marshal(in)
		return h.Call(ctx, contracts.HostCall{Method: method, Args: raw}, contracts.Permission{}, func() error { return nil })
	}
	for _, method := range []string{"search.get", "search.resources"} {
		v, e := call(method, b.Selection{ID: run, PageRequest: b.PageRequest{Page: 2, Size: 200, Query: "Owned"}})
		if e != nil {
			t.Fatal(e)
		}
		result := v.(b.SearchRun)
		if len(result.Resources) != 5 || result.HasMore {
			t.Fatal("wrong SQL page", len(result.Resources), result.HasMore)
		}
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), "SECRET") {
			t.Fatal("snapshot secret metadata exported")
		}
	}
	if v, e := call("search.resources", b.Selection{ID: run, PageRequest: b.PageRequest{Page: 1, Size: 10, Query: "NoMatch"}}); e != nil || len(v.(b.SearchRun).Resources) != 0 {
		t.Fatal(e, v)
	}
	for _, tt := range []struct {
		method string
		input  any
	}{{"search.list", b.PageRequest{Page: 1, Size: 200}}, {"subscriptions.list", b.PageRequest{Page: 1, Size: 200}}, {"subscriptions.get", b.ID{ID: sub}}, {"subscriptions.history", b.Selection{ID: sub, PageRequest: b.PageRequest{Page: 1, Size: 200}}}, {"subscriptions.queue", b.Queue{ID: sub, Preview: true}}} {
		if _, e := call(tt.method, tt.input); e != nil {
			t.Fatal(tt.method, e)
		}
	}
	raw, _ := json.Marshal(b.ID{ID: sub})
	rev := int64(99)
	if _, e := h.Call(ctx, contracts.HostCall{Method: "subscriptions.delete", Args: raw, ExpectedRevision: &rev}, contracts.Permission{}, func() error { return nil }); e == nil {
		t.Fatal("stale delete accepted")
	}
	rev = 1
	if _, e := h.Call(ctx, contracts.HostCall{Method: "subscriptions.delete", Args: raw, ExpectedRevision: &rev}, contracts.Permission{}, func() error { return nil }); e == nil {
		t.Fatal("active evaluation deleted")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSubscriptionCreateUpdateAndDeleteViaNativeTMDBValidation(t *testing.T) {
	h, ctx := hostFixture(t)
	var lookups atomic.Int32
	h.Connections.OutboundHTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.themoviedb.org" || !strings.Contains(r.URL.Path, "/movie/1") {
			t.Errorf("unexpected provider path %s", r.URL.Host)
		}
		lookups.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":1,"title":"Owned Film","original_title":"Owned Film","release_date":"2026-01-01","genres":[]}`))}, nil
	})}
	if _, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "owned", Enabled: true, Settings: map[string]any{"language": "zh-CN"}, Secrets: map[string]string{"token": "TMDB_FIXTURE_SECRET"}}); e != nil {
		t.Fatal(e)
	}
	if e := (sites.Repository{DB: h.DB}).LoadGoTemplates(ctx, []string{"hdsky"}); e != nil {
		t.Fatal(e)
	}
	account, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "ptsite", Name: "owned", Enabled: true, Settings: map[string]any{"definitionId": "hdsky", "endpoint": "https://owned.invalid", "userId": "1"}, Secrets: map[string]string{"cookie": "session=fixture"}})
	if e != nil {
		t.Fatal(e)
	}
	in := b.SubscriptionInput{MediaType: "movie", TMDBID: 1, Enabled: false, Source: "search", Rules: b.Rules{Keyword: "Owned Film", AccountIDs: []string{account.ID}}}
	raw, _ := json.Marshal(in)
	v, e := h.Call(ctx, contracts.HostCall{Method: "subscriptions.create", Args: raw}, contracts.Permission{}, func() error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	first := v.(b.Subscription)
	if first.Revision != 1 || first.Media.ID != 1 || lookups.Load() != 1 {
		t.Fatal(first)
	}
	in.ID = first.ID
	in.Rules.Keyword = "Owned"
	raw, _ = json.Marshal(in)
	rev := int64(1)
	v, e = h.Call(ctx, contracts.HostCall{Method: "subscriptions.update", Args: raw, ExpectedRevision: &rev}, contracts.Permission{}, func() error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	if v.(b.Subscription).Revision != 2 {
		t.Fatal("revision not incremented")
	}
	raw, _ = json.Marshal(b.ID{ID: first.ID})
	rev = 2
	if _, e = h.Call(ctx, contracts.HostCall{Method: "subscriptions.delete", Args: raw, ExpectedRevision: &rev}, contracts.Permission{}, func() error { return nil }); e != nil {
		t.Fatal(e)
	}
	var archived bool
	if e = h.DB.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM subscriptions WHERE id=$1`, first.ID).Scan(&archived); e != nil || !archived {
		t.Fatal(e, archived)
	}
}
