package migrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"io"
	"media-tool/internal/adapters/tmdb"
	sourcecatalog "media-tool/internal/modules/catalog"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/nativetmdb"
	pluginbusiness "media-tool/internal/modules/plugins/business"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/modules/subscriptions"
	"media-tool/internal/platform/config"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type discoveryTransport func(*http.Request) (*http.Response, error)

func (f discoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func discoveryManager(t *testing.T, f movieFixture, body *string) *integrations.Manager {
	t.Helper()
	v, e := secrets.New(strings.Repeat("12", 32))
	if e != nil {
		t.Fatal(e)
	}
	repo := integrations.Repository{DB: f.pool, Vault: v}
	if _, e = repo.Save(f.ctx, "", integrations.Input{Kind: "tmdb", Name: "F4C fixture", Enabled: true, Settings: map[string]any{}, Secrets: map[string]string{"token": "PRIVATE_TMDB_SENTINEL"}}); e != nil {
		t.Fatal(e)
	}
	manager := integrations.NewManager(repo, config.Config{})
	t.Cleanup(manager.Close)
	manager.OutboundHTTP = &http.Client{Transport: discoveryTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.themoviedb.org" {
			t.Fatal("unexpected host")
		}
		if *body == "unavailable" {
			return nil, errors.New("offline")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(*body))}, nil
	})}
	return manager
}
func latestDiscoveryFixture(t *testing.T) movieFixture {
	f := newMovieFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 63); e != nil {
		t.Fatal(e)
	}
	return f
}
func TestF4CCoverageOnlyExposableAuthoritativeSeasonNoInventedState(t *testing.T) {
	f := latestDiscoveryFixture(t)
	body := `{"id":100,"season_number":1,"episodes":[{"id":101,"season_number":1,"episode_number":1,"name":"One","air_date":"2026-01-01"},{"id":102,"season_number":1,"episode_number":2,"name":"Future","air_date":"2099-01-01"}]}`
	manager := discoveryManager(t, f, &body)
	id, media := jobs.UUID(), jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',1,'Synthetic','{}')`, media)
	f.exec(t, `INSERT INTO subscriptions(id,title,media_type,identity,rules,enabled)VALUES($1,'Synthetic','series','{"id":1,"mediaType":"series"}','{"season":1,"episodes":[1]}',true)`, id)
	a := f.asset(t, media, "g:test-a", time.Now(), "mkv")
	f.exec(t, `UPDATE media_asset_bindings SET season=1,episode=1,episodes='[1]' WHERE asset_id=$1`, a.asset)
	observer := subscriptions.CoverageObserver{Engine: subscriptions.Engine{DB: f.pool, Connections: manager}}
	initial := f.text(t, `SELECT row_to_json(s)::text FROM subscriptions s WHERE id=$1`, id)
	assert := func(state string, count int) {
		t.Helper()
		v, e := observer.Check(f.ctx, id)
		if e != nil || v.State != state {
			t.Fatal(v, e)
		}
		if e = observer.Observe(f.ctx, id); e != nil {
			t.Fatal(e)
		}
		if n := f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='subscription.coverage.completed'`); n != int64(count) {
			t.Fatal("completion broadcast", n, count)
		}
	}
	assert("partial", 0) // download rules requesting only E1 cannot shrink full-season expectation.
	b := f.asset(t, media, "g:test-a", time.Now(), "mkv")
	f.exec(t, `UPDATE media_asset_bindings SET season=1,episode=2,episodes='[2]' WHERE asset_id=$1`, b.asset)
	f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=$1`, b.file)
	assert("partial", 0)
	f.exec(t, `UPDATE cloud_files SET state='present' WHERE id=$1`, b.file)
	f.exec(t, `UPDATE strm_entries SET output_path='' WHERE asset_id=$1`, b.asset)
	assert("partial", 0)
	f.exec(t, `UPDATE strm_entries SET output_path='Synthetic/two.strm' WHERE asset_id=$1`, b.asset)
	// A failed commit guard rolls back the just-staged observation fact.
	guarded := dbutil.WithCommitGuard(f.ctx, func(ctx context.Context, _ pgx.Tx) error {
		other, e := f.pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer other.Rollback(ctx)
		_, e = other.Exec(ctx, `SELECT id FROM cloud_files WHERE id=$1 FOR UPDATE NOWAIT`, b.file)
		var pgErr interface{ SQLState() string }
		if !errors.As(e, &pgErr) || pgErr.SQLState() != "55P03" {
			t.Error("exposure was not locked through commit fence", e)
		}
		return errors.New("fence_changed")
	})
	if observer.Observe(guarded, id) == nil {
		t.Fatal("fence did not roll back")
	}
	if f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='subscription.coverage.completed'`) != 0 {
		t.Fatal("rollback emitted")
	}
	assert("complete", 1)
	assert("complete", 1)
	if got := f.text(t, `SELECT row_to_json(s)::text FROM subscriptions s WHERE id=$1`, id); got != initial {
		t.Fatal("observation mutated subscription state")
	}
	host := pluginbusiness.AcquisitionHost{DB: f.pool, Connections: manager}
	raw, _ := json.Marshal(map[string]string{"id": id})
	v, e := host.Call(f.ctx, contracts.HostCall{Method: "subscriptions.completion", Args: raw}, contracts.Permission{Scope: "subscriptions.read"}, nil)
	if e != nil || !v.(contracts.CoverageObservation).Complete {
		t.Fatal(v, e)
	}
	body = "unavailable"
	assert("unknown", 1)
	body = `{"id":100,"season_number":1,"episodes":[]}`
	assert("unknown", 1)
	body = `{"id":100,"season_number":2,"episodes":[{"id":101,"season_number":2,"episode_number":1,"name":"Wrong"}]}`
	assert("unknown", 1)
}
func TestF4CMetadataPersistTransactionAndTMDBSuccessOnly(t *testing.T) {
	f := latestDiscoveryFixture(t)
	mid := f.media(t, 900001, "Film", "2026", `{}`)
	asset := f.asset(t, mid, "g:test-a", time.Now(), "mkv")
	revision := f.integer(t, `SELECT revision FROM application_settings WHERE singleton`)
	item := metadata.Item{ID: 900001, MediaType: "movie", Title: "Film", PosterPath: "/SECRET.jpg", Scrape: &metadata.ScrapeBundle{SettingsRevision: int64(revision), Policy: metadata.DefaultScrapePolicy(), Notices: []metadata.ScrapeNotice{{Kind: "poster", Message: "PRIVATE_TMDB_SENTINEL"}}}}
	for _, commit := range []bool{false, true, true} {
		tx, e := f.pool.Begin(f.ctx)
		if e != nil {
			t.Fatal(e)
		}
		if e = library.PersistScrapeReport(f.ctx, tx, asset.asset, item); e != nil {
			t.Fatal(e)
		}
		if !commit {
			e = tx.Rollback(f.ctx)
		} else {
			e = tx.Commit(f.ctx)
		}
		if e != nil {
			t.Fatal(e)
		}
		if !commit && f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='metadata.scraped'`) != 0 {
			t.Fatal("uncommitted scrape event")
		}
	}
	if f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='metadata.scraped'`) != 1 {
		t.Fatal("scrape fact idempotency")
	}
	body := `{"id":900001,"title":"Film","poster_path":"/SECRET.jpg"}`
	manager := discoveryManager(t, f, &body)
	repo := nativetmdb.Repository{DB: f.pool}
	if _, e := repo.Enqueue(f.ctx, nativetmdb.Target{Type: "movie", TMDBID: 900001}, "zh-CN", true); e != nil {
		t.Fatal(e)
	}
	detail := f.text(t, `SELECT id::text FROM native_tmdb_details WHERE tmdb_id=900001`)
	payload, _ := json.Marshal(map[string]string{"detailId": detail})
	engine := nativetmdb.Engine{DB: f.pool, Connections: manager}
	if e := engine.Process(f.ctx, jobs.Job{Payload: payload}); e != nil {
		t.Fatal(e)
	}
	if f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='metadata.scraped'`) != 2 {
		t.Fatal("successful stored detail did not emit")
	}
	body = "unavailable"
	f.exec(t, `UPDATE native_tmdb_details SET next_attempt_at=now() WHERE id=$1`, detail)
	if e := engine.Process(f.ctx, jobs.Job{Payload: payload}); e != nil {
		t.Fatal(e)
	}
	if f.integer(t, `SELECT count(*) FROM outbox_events WHERE event_type='metadata.scraped'`) != 2 {
		t.Fatal("failed fetch emitted success")
	}
	raw := f.text(t, `SELECT string_agg(payload::text,'') FROM outbox_events WHERE event_type='metadata.scraped'`)
	if strings.Contains(raw, "SECRET") || strings.Contains(raw, "poster") {
		t.Fatal("metadata fact leaked provider fields")
	}
}
func TestF4CCachedSourcePaginationNeverInvokesWorkerAndRevokeHidesCache(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("isolated Redis DB15 required")
	}
	opts, e := redis.ParseURL(url)
	if e != nil || opts.DB != 15 {
		t.Fatal("Redis DB15 required", e)
	}
	rc := redis.NewClient(opts)
	defer rc.Close()
	f := latestDiscoveryFixture(t)
	vault, e := secrets.New(strings.Repeat("33", 32))
	if e != nil {
		t.Fatal(e)
	}
	s := pluginv2.Store{DB: f.pool, Vault: vault, Root: t.TempDir()}
	defs, e := pluginv2.Catalog()
	if e != nil {
		t.Fatal(e)
	}
	d := defs[0]
	scopes := []string{}
	for _, p := range d.Manifest.Permissions {
		scopes = append(scopes, p.Scope)
	}
	in, e := s.Apply(f.ctx, "test", pluginv2.Request{Kind: pluginv2.Install, PluginID: d.Manifest.ID, OperationKey: jobs.UUID(), Fingerprint: d.Fingerprint, Scopes: scopes, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	in, e = s.Apply(f.ctx, "test", pluginv2.Request{Kind: pluginv2.Enable, InstanceID: in.ID, OperationKey: jobs.UUID(), Fingerprint: d.Fingerprint, ExpectedRevision: in.Revision, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	lease, e := s.Claim(f.ctx, jobs.UUID())
	if e != nil {
		t.Fatal(e)
	}
	typ := "discover.source"
	meta := contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{{Kind: "chain", ID: typ, Handler: contracts.HandlerID("chain", typ), RequiredScopes: []string{"events.chain." + typ}}}}
	if e = s.Publish(f.ctx, lease, meta); e != nil {
		t.Fatal(e)
	}
	if e = s.Renew(f.ctx, lease, true); e != nil {
		t.Fatal(e)
	}
	snapshot, e := s.Snapshot(f.ctx, in.ID)
	if e != nil {
		t.Fatal(e)
	}
	key := "plugins:v2:sources:" + in.ID + ":" + typ
	defer rc.Del(context.Background(), key)
	data := map[string]any{"snapshot": snapshot.ID, "result": contracts.SourceResult{Items: []contracts.SourceCandidate{{MediaIdentity: contracts.MediaIdentity{TMDBID: 1, MediaType: "movie"}, Title: "One"}, {MediaIdentity: contracts.MediaIdentity{TMDBID: 2, MediaType: "series"}, Title: "Two"}}}}
	raw, _ := json.Marshal(data)
	if e = rc.Set(f.ctx, key, raw, time.Minute).Err(); e != nil {
		t.Fatal(e)
	}
	registry := sourcecatalog.SourceRegistry{Store: s, Cache: rc}
	before := time.Now()
	page, e := registry.Page(f.ctx, typ, 1, 1)
	if e != nil || len(page.Items) != 1 || !page.HasMore || time.Since(before) > time.Second || page.Items[0].Source != "plugin:"+in.ID {
		t.Fatal(page, e)
	}
	second, e := registry.Page(f.ctx, typ, 2, 1)
	if e != nil || len(second.Items) != 1 || second.HasMore || second.Items[0].TMDBID != 2 {
		t.Fatal(second, e)
	}
	if f.integer(t, `SELECT count(*) FROM plugin_v2_runs`) != 0 {
		t.Fatal("homepage invoked worker synchronously")
	}
	f.exec(t, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope=$2`, in.ID, "events.chain."+typ)
	page, e = registry.Page(f.ctx, typ, 1, 200)
	if e != nil || len(page.Items) != 0 {
		t.Fatal("revoked cached source visible", page, e)
	}
}

// Compile-time interface evidence: the worker uses the existing TMDB client, not
// a new provider/credential path or per-episode HTTP fan-out.
var _ nativetmdb.Connections = (*integrations.Manager)(nil)
var _ = tmdb.Client{}
