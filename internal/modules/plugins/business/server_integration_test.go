package business

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"github.com/jackc/pgx/v5"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/admin"
	"media-tool/internal/modules/mediaprobe/stillqueue"
	"media-tool/internal/modules/mediaprobe/stillstore"
	"media-tool/internal/modules/mediausers"
	nativecatalog "media-tool/internal/modules/nativelibrary/catalog"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/pluginactor"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func serverFixture(t *testing.T) (AcquisitionHost, context.Context) {
	t.Helper()
	h, ctx := mediaFixture(t)
	for _, table := range []string{"media_users", "media_user_devices", "media_user_tokens", "media_user_sessions", "media_user_item_data", "native_playback_sessions", "media_playback_events", "media_playback_ips", "media_probe_results", "media_stills", "player_access_events"} {
		if _, e := h.DB.Exec(ctx, `CREATE TABLE `+pgx.Identifier{table}.Sanitize()+`(LIKE public.`+pgx.Identifier{table}.Sanitize()+` INCLUDING ALL)`); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := h.DB.Exec(ctx, `CREATE FUNCTION native_probe_fingerprint(bigint,text,jsonb)RETURNS text LANGUAGE sql IMMUTABLE AS 'SELECT public.native_probe_fingerprint($1,$2,$3)'`); e != nil {
		t.Fatal(e)
	}
	stills, e := stillqueue.New(h.DB, stillqueue.Config{})
	if e != nil {
		t.Fatal(e)
	}
	h.Processing = &admin.Repository{DB: h.DB, Stills: stills}
	realRoot, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	store, e := stillstore.Open(filepath.Join(realRoot, "native-media", "stills"))
	if e != nil {
		t.Fatal(e)
	}
	cache, e := stillstore.NewCache(store, stills)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := cache.Close(); e != nil {
			t.Error(e)
		}
	})
	h.Collector = testCollector{cache}
	return h, ctx
}

type testCollector struct{ c *stillstore.Cache }

func (c testCollector) Collect(ctx context.Context) (stillstore.GCReport, error) {
	return c.c.GC(ctx, 200)
}
func serverCall(t *testing.T, h AcquisitionHost, method string, in any, libraries []string, rev ...int64) (any, error) {
	t.Helper()
	raw, e := json.Marshal(in)
	if e != nil {
		t.Fatal(e)
	}
	spec, ok := b.Lookup(method)
	if !ok {
		t.Fatal(method)
	}
	p := contracts.Permission{Scope: spec.Scope}
	if libraries != nil {
		p.Resources = map[string][]string{"libraryIds": libraries}
	}
	ctx := pluginactor.With(context.Background(), "host-issued-instance", p.Scope, libraries)
	call := contracts.HostCall{Method: method, Args: raw, OperationKey: jobs.UUID()}
	if len(rev) > 0 {
		call.ExpectedRevision = &rev[0]
	}
	return h.Call(ctx, call, p, func() error { return nil })
}
func requireServer[T any](t *testing.T, h AcquisitionHost, method string, in any, libraries []string, rev ...int64) T {
	t.Helper()
	v, e := serverCall(t, h, method, in, libraries, rev...)
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
func assertServerSafe(t *testing.T, raw []byte) {
	t.Helper()
	text := strings.ToLower(string(raw))
	for _, sentinel := range []string{"http://", "https://", "/115/", "/root/", "/data/", "secret_canary", "pickcode", `"url"`, `"streamurls"`, `"filepath"`, `"token"`, `"passwordhash"`, `"clientip"`, `"useragent"`, `"headers"`} {
		if strings.Contains(text, sentinel) {
			t.Fatalf("unsafe DTO: %s", raw)
		}
	}
}
func seedServerCatalog(t *testing.T, h *AcquisitionHost, ctx context.Context) (string, string) {
	t.Helper()
	asset, _ := seedMedia(t, h, ctx)
	if _, e := h.DB.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,'Owned.strm','PRIVATE_REFERENCE')`, jobs.UUID(), asset); e != nil {
		t.Fatal(e)
	}
	if _, e := h.DB.Exec(ctx, `INSERT INTO native_libraries(id,item_id,group_id,name,collection_type,directory,sort_order,policy_revision,hidden)VALUES('g:movie',100001,'movie','Owned','movies','Movies',0,1,false),('g:other',100002,'other','Other','movies','Other',1,1,false)`); e != nil {
		t.Fatal(e)
	}
	if _, e := h.DB.Exec(ctx, `INSERT INTO native_asset_libraries(asset_id,library_id,source,policy_revision,asset_revision)VALUES($1,'g:movie','classification',1,1)`, asset); e != nil {
		t.Fatal(e)
	}
	return asset, "g:movie"
}
func TestServerCatalogTrustedActorLibraryIsolationAndNoPlaybackURLs(t *testing.T) {
	h, ctx := serverFixture(t)
	asset, lib := seedServerCatalog(t, &h, ctx)
	page := b.PageRequest{Page: 1, Size: 100}
	requireServer[[]b.ServerProvider](t, h, "mediaserver.providers", b.Empty{}, nil)
	q := b.ServerQuery{Provider: "builtin", PageRequest: page}
	libs := requireServer[b.Page[b.ServerLibrary]](t, h, "mediaserver.libraries", q, []string{lib})
	if len(libs.Items) != 1 || libs.Items[0].ID != lib {
		t.Fatal(libs)
	}
	for _, method := range []string{"mediaserver.items", "catalog.list"} {
		v := requireServer[b.Page[b.ServerItem]](t, h, method, q, []string{lib})
		if len(v.Items) != 1 || v.Items[0].ID != asset {
			t.Fatal(v)
		}
	}
	q.ID = asset
	requireServer[b.ServerItem](t, h, "catalog.item", q, []string{lib})
	q.LibraryID = "g:other"
	_, e := serverCall(t, h, "catalog.item", q, []string{lib})
	mediaCode(t, e, contracts.PermissionDenied)
	q.LibraryID = ""
	_, e = serverCall(t, h, "catalog.item", q, []string{"g:other"})
	mediaCode(t, e, contracts.NotFound)
	// Supplying even a real user's identity cannot turn this admin projection into
	// a member principal, nor broaden the host-owned selector list.
	for _, key := range []string{"userId", "visibleLibraries", "actor", "access"} {
		raw := []byte(fmt.Sprintf(`{"provider":"builtin","page":1,"size":100,"%s":"forged"}`, key))
		_, e = h.Call(pluginactor.With(ctx, "host", "mediaserver.read", []string{lib}), contracts.HostCall{Method: "catalog.list", Args: raw}, contracts.Permission{Scope: "mediaserver.read"}, nil)
		mediaCode(t, e, contracts.InvalidParams)
	}
	native := nativecatalog.Repository{DB: h.DB}
	if _, _, e = native.PluginItems(ctx, "", "", "", "", 0, 10); e == nil {
		t.Fatal("no actor accepted")
	}
	for _, provider := range []string{"builtin", "emby"} {
		requireServer[b.Receipt](t, h, "mediaserver.refresh", b.ServerRefresh{Provider: provider, AssetID: asset}, []string{lib})
	}
	_, e = serverCall(t, h, "mediaserver.refresh", b.ServerRefresh{Provider: "builtin", AssetID: asset}, []string{"g:other"})
	if e == nil {
		t.Fatal("cross-library refresh")
	}
	// Membership is rechecked at commit if an asset is reassigned after preflight.
	guarded, e := native.PluginRefreshContext(pluginactor.With(ctx, "host", "mediaserver.refresh", []string{lib}), asset)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.DB.Exec(ctx, `UPDATE native_asset_libraries SET library_id='g:other'WHERE asset_id=$1`, asset); e != nil {
		t.Fatal(e)
	}
	tx, e := h.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = dbutil.Commit(guarded, tx); e == nil {
		t.Fatal("stale membership committed")
	}
	_ = tx.Rollback(ctx)
	var credentials int
	if e = h.DB.QueryRow(ctx, `SELECT (SELECT count(*)FROM media_users)+(SELECT count(*)FROM media_user_tokens)`).Scan(&credentials); e != nil || credentials != 0 {
		t.Fatal("fabricated member", e)
	}
	settings := requireServer[b.ServerConfiguration](t, h, "mediaserver.settings.get", b.Empty{}, nil)
	settings.ServerName = "Owned server"
	updated := requireServer[b.ServerConfiguration](t, h, "mediaserver.settings.update", settings, nil, settings.Revision)
	if updated.Revision != settings.Revision+1 {
		t.Fatal(updated)
	}
}

func TestServerUserManagementRevocationSummariesAndSensitiveProjection(t *testing.T) {
	h, ctx := serverFixture(t)
	_, lib := seedServerCatalog(t, &h, ctx)
	r := mediausers.Repository{DB: h.DB}
	rating, streams := 18, 2
	expiry := time.Now().Add(time.Hour).UTC()
	u, e := r.Save(ctx, "", mediausers.Input{Username: "fixture_user", DisplayName: "Owned", Password: "Fixture-2026!", Enabled: true, MaxStreams: &streams, MaxParentalRating: &rating, ExpiresAt: &expiry, Grants: []mediausers.Grant{{LibraryID: lib, CanPlay: true}}})
	if e != nil {
		t.Fatal(e)
	}
	other, e := r.Save(ctx, "", mediausers.Input{Username: "other_user", DisplayName: "Other", Password: "Fixture-2026!", Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	var passwordBefore string
	if e = h.DB.QueryRow(ctx, `SELECT password_hash FROM media_users WHERE id=$1`, u.ID).Scan(&passwordBefore); e != nil {
		t.Fatal(e)
	}
	device := jobs.UUID()
	session := jobs.UUID()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO media_user_devices(id,user_id,device_key,name,client,app_version)VALUES($1,$2,'PRIVATE_DEVICE_KEY','https://cdn.115cdn.net/?token=SECRET_CANARY','clientIP=SECRET_CANARY','1')`, []any{device, u.ID}},
		{`INSERT INTO media_user_tokens(user_id,device_id,token_digest,credential_version,expires_at,last_seen_at)VALUES($1,$2,decode(repeat('ab',32),'hex'),$3,now()+interval '1 hour',now())`, []any{u.ID, device, u.Revision}},
		{`INSERT INTO native_item_ids(id,entity_type,internal_key)VALUES(100100,'movie','tmdb:42')`, nil},
		{`INSERT INTO media_user_item_data(user_id,item_id,position_ticks,play_count,last_played_at)VALUES($1,100100,42,1,now())`, []any{u.ID}},
		{`INSERT INTO native_playback_sessions(id,user_id,device_id,item_id,media_source_id,state,position_ticks,last_progress_at)VALUES($1,$2,$3,100100,'PRIVATE_SOURCE_ID','playing',42,now())`, []any{session, u.ID, device}},
		{`INSERT INTO player_access_events(id,observation_id,user_agent,client_ip,entry,method,allowed,reason,policy_revision,connection_ip,ip_source,ip_public)VALUES($1,repeat('a',64),'SECRET_CANARY','127.0.0.1','/Videos/PRIVATE','GET',true,'allowed',1,'127.0.0.1','direct',false)`, []any{jobs.UUID()}},
	} {
		if _, e = h.DB.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(q.sql, e)
		}
	}
	page := b.PageRequest{Page: 1, Size: 200}
	requireServer[b.Page[b.ServerUser]](t, h, "mediausers.list", page, nil)
	requireServer[b.ServerUser](t, h, "mediausers.get", b.ID{ID: u.ID}, nil)
	devices := requireServer[b.Page[b.ServerDevice]](t, h, "mediausers.devices", b.Selection{ID: u.ID, PageRequest: page}, nil)
	if len(devices.Items) != 1 {
		t.Fatal(devices)
	}
	requireServer[b.Page[b.WatchHistory]](t, h, "mediausers.history", b.Selection{ID: u.ID, PageRequest: page}, nil)
	for _, method := range []string{"playback.sessions", "mediaserver.sessions"} {
		v := requireServer[b.Page[b.PlaybackSession]](t, h, method, page, nil)
		if len(v.Items) != 1 || v.Items[0].ID != session {
			t.Fatal(v)
		}
	}
	counts := requireServer[b.PlaybackCounts](t, h, "playback.summary", b.Empty{}, nil)
	if counts.Playing != 1 {
		t.Fatal(counts)
	}
	access := requireServer[b.AccessCounts](t, h, "playeraccess.summary", b.Empty{}, nil)
	if access.Allowed != 1 {
		t.Fatal(access)
	}
	_, e = serverCall(t, h, "playback.sessions", page, []string{lib})
	mediaCode(t, e, contracts.Unsupported)
	// Device ownership is verified under lock; another user's ID is not authority.
	_, e = serverCall(t, h, "mediausers.revokeDevice", b.DeviceRevocation{ID: other.ID, DeviceID: device, Revision: other.Revision}, nil, other.Revision)
	if e == nil {
		t.Fatal("cross-user device revoke")
	}
	requireServer[b.Receipt](t, h, "mediausers.revokeDevice", b.DeviceRevocation{ID: u.ID, DeviceID: device, Revision: u.Revision}, nil, u.Revision)
	var live int
	if e = h.DB.QueryRow(ctx, `SELECT count(*)FROM media_user_tokens WHERE user_id=$1 AND revoked_at IS NULL`, u.ID).Scan(&live); e != nil || live != 0 {
		t.Fatal("device credential live", live, e)
	}
	var state string
	if e = h.DB.QueryRow(ctx, `SELECT state FROM native_playback_sessions WHERE id=$1`, session).Scan(&state); e != nil || state != "stopped" {
		t.Fatal(state, e)
	}
	name := "Updated"
	updated := requireServer[b.ServerUser](t, h, "mediausers.update", b.UserPatch{ID: u.ID, Revision: u.Revision, DisplayName: &name}, nil, u.Revision)
	if updated.ExpiresAt == nil || updated.MaxParentalRating == nil || updated.MaxStreams != 2 || len(updated.Grants) != 1 {
		t.Fatal("omitted patch cleared data", updated)
	}
	cleared := requireServer[b.ServerUser](t, h, "mediausers.update", b.UserPatch{ID: u.ID, Revision: updated.Revision, ClearExpiry: true, ClearParentalRating: true}, nil, updated.Revision)
	if cleared.ExpiresAt != nil || cleared.MaxParentalRating != nil {
		t.Fatal("clear semantics", cleared)
	}
	var passwordAfter string
	if e = h.DB.QueryRow(ctx, `SELECT password_hash FROM media_users WHERE id=$1`, u.ID).Scan(&passwordAfter); e != nil || passwordBefore != passwordAfter {
		t.Fatal("password changed", e)
	}
	// Fence a staged native user update; no credential/data changes may commit.
	current, e := r.Get(ctx, u.ID)
	if e != nil {
		t.Fatal(e)
	}
	denied := dbutil.WithCommitGuard(ctx, func(context.Context, pgx.Tx) error { return contracts.NewError(contracts.PermissionDenied) })
	_, e = r.Save(denied, u.ID, mediausers.Input{Username: current.Username, DisplayName: "DENIED", Enabled: false, Revision: current.Revision, Grants: current.Grants})
	if e == nil {
		t.Fatal("revoked user write committed")
	}
	actual, e := r.Get(ctx, u.ID)
	if e != nil || actual.Revision != current.Revision || actual.DisplayName != current.DisplayName {
		t.Fatal(actual, e)
	}
	for _, raw := range []string{`{"id":"` + u.ID + `","revision":1,"password":"forbidden"}`, `{"id":"` + u.ID + `","token":"forbidden"}`} {
		_, e = h.Call(pluginactor.With(ctx, "host", "mediaserver.users.write", nil), contracts.HostCall{Method: "mediausers.update", Args: []byte(raw)}, contracts.Permission{Scope: "mediaserver.users.write"}, nil)
		mediaCode(t, e, contracts.InvalidParams)
	}
}

func TestServerProcessingQueueOnlyGCAndNativeDetails(t *testing.T) {
	h, ctx := serverFixture(t)
	asset, _ := seedServerCatalog(t, &h, ctx)
	if _, e := h.DB.Exec(ctx, `UPDATE native_server_settings SET enabled=true,probe_enabled=true,stills_enabled=true`); e != nil {
		t.Fatal(e)
	}
	requireServer[b.ProcessingStatus](t, h, "mediaprobe.status", b.Empty{}, nil)
	requireServer[b.RetryCounts](t, h, "mediaprobe.retry", b.IDs{IDs: []string{asset}}, nil)
	for _, kind := range []string{"probe", "still"} {
		requireServer[b.Page[b.ProcessingTask]](t, h, "mediaprobe.list", b.ProcessingQuery{PageRequest: b.PageRequest{Page: 1, Size: 200}, Kind: kind}, nil)
	}
	// The episode has no exposable evidence, so native retry safely selects zero
	// or skips it; this cannot invoke a grabber or resolve a media link.
	if _, e := h.DB.Exec(ctx, `INSERT INTO native_item_ids(id,entity_type,internal_key)VALUES(100200,'episode','tmdb:42:s1e1')`); e != nil {
		t.Fatal(e)
	}
	requireServer[b.RetryCounts](t, h, "mediaprobe.stills.retry", b.IDs{IDs: []string{"100200"}}, nil)
	requireServer[b.GCCounts](t, h, "mediaprobe.stills.gc", b.Empty{}, nil)
	var jobsCount int
	if e := h.DB.QueryRow(ctx, `SELECT count(*)FROM jobs WHERE kind='native.probe'`).Scan(&jobsCount); e != nil || jobsCount == 0 {
		t.Fatal("retry did not enqueue", jobsCount, e)
	}
	target := b.NativeTarget{MediaType: "movie", TMDBID: 42, PageRequest: b.PageRequest{Page: 1, Size: 200}}
	requireServer[b.Receipt](t, h, "metadata.refreshDetails", target, nil)
	if _, e := h.DB.Exec(ctx, `UPDATE native_tmdb_details SET payload='{"name":"Owned","originalTitle":"Owned","posterPath":"https://cdn.115cdn.net/SECRET_CANARY","overview":"/115/SECRET_CANARY","genres":["Drama"],"episodes":[{"season":1,"number":1,"name":"https://secret.invalid","stillPath":"https://secret.invalid"}]}',state='ready',fetched_at=now()WHERE tmdb_id=42`); e != nil {
		t.Fatal(e)
	}
	v := requireServer[b.NativeMetadata](t, h, "metadata.nativeDetails", target, nil)
	if len(v.Episodes.Items) != 1 || v.Episodes.Items[0].Number != 1 {
		t.Fatal(v)
	}
	// Commit denial must prevent native retry from clearing backoff/terminal state.
	if _, e := h.DB.Exec(ctx, `UPDATE media_probe_results SET state='failed',last_error_class='permanent',last_error_reason='process_failed',next_attempt_at=now()+interval '1 day'`); e != nil {
		t.Fatal(e)
	}
	denied := dbutil.WithCommitGuard(pluginactor.With(ctx, "host", "mediaprobe.retry", nil), func(context.Context, pgx.Tx) error { return contracts.NewError(contracts.PermissionDenied) })
	raw, _ := json.Marshal(b.IDs{IDs: []string{asset}})
	_, e := h.Call(denied, contracts.HostCall{Method: "mediaprobe.retry", Args: raw}, contracts.Permission{Scope: "mediaprobe.retry"}, func() error { return nil })
	if e == nil {
		t.Fatal("revoked retry committed")
	}
	var state string
	if e = h.DB.QueryRow(ctx, `SELECT state FROM media_probe_results LIMIT 1`).Scan(&state); e != nil || state != "failed" {
		t.Fatal(state, e)
	}
	absent := h
	absent.Processing = nil
	_, e = serverCall(t, absent, "mediaprobe.status", b.Empty{}, nil)
	mediaCode(t, e, contracts.Unsupported)
}

func TestServerEmbyFacadeNoPlaybackClientAndScopedLookupFailsClosed(t *testing.T) {
	h, ctx := serverFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "SECRET_CANARY" {
			t.Error("host did not attach auth")
		}
		if strings.Contains(strings.ToLower(r.URL.Path), "playback") || strings.Contains(strings.ToLower(r.URL.Path), "image") || strings.Contains(strings.ToLower(r.URL.Path), "stream") {
			t.Error("forbidden upstream endpoint", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		body := `{"Items":[{"Id":"item-1","Name":"https://cdn.115cdn.net/?token=SECRET_CANARY","Type":"Movie","ProductionYear":2026,"Path":"/115/SECRET_CANARY","MediaSources":[{"Path":"https://cdn.115cdn.net/SECRET_CANARY"}]}],"TotalRecordCount":1}`
		if strings.HasSuffix(r.URL.Path, "item-1") {
			body = `{"Id":"item-1","Name":"Owned","Type":"Movie","Path":"/115/SECRET_CANARY","DirectStreamUrl":"https://cdn.115cdn.net/SECRET_CANARY"}`
		}
		w.Header().Set("Set-Cookie", "SECRET_CANARY")
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()
	_, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "emby", Name: "fake", Enabled: true, Settings: map[string]any{"endpoint": server.URL}, Secrets: map[string]string{"apiKey": "SECRET_CANARY"}})
	if e != nil {
		t.Fatal(e)
	}
	q := b.ServerQuery{Provider: "emby", PageRequest: b.PageRequest{Page: 1, Size: 100}}
	requireServer[b.Page[b.ServerItem]](t, h, "mediaserver.items", q, nil)
	q.ID = "item-1"
	requireServer[b.ServerItem](t, h, "catalog.item", q, nil)
	_, e = serverCall(t, h, "catalog.item", q, []string{"emby-lib"})
	mediaCode(t, e, contracts.Unsupported)
	q.ID = ""
	_, e = serverCall(t, h, "catalog.list", q, []string{"emby-lib"})
	mediaCode(t, e, contracts.Unsupported)
	_, e = serverCall(t, h, "mediaserver.libraries", q, nil)
	mediaCode(t, e, contracts.Unsupported)
}
