package migrations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativeauth"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
)

func TestNativeManagementDevicesHistorySettingsRealHTTP(t *testing.T) {
	f := newBrowseFixture(t)
	// These assertions execute the current runtime, not the historical DTOs.
	if _, err := f.provider.UpTo(f.ctx, 79); err != nil {
		t.Fatal(err)
	}
	if _, err := f.provider.UpTo(f.ctx, 79); err != nil {
		t.Fatal(err)
	}
	repo := mediausers.Repository{DB: f.pool}
	id := f.text(t, `SELECT id::text FROM media_users WHERE id=$1`, f.user.ID)
	store := &nativeauth.Store{DB: f.pool, Secret: "synthetic-management-key"}
	login := func(key string) contracts.LoginResult {
		t.Helper()
		out, err := store.Login(f.ctx, "browse.member", "Browse123", contracts.DeviceInfo{DeviceID: key, Name: "Synthetic iPad", Client: "Infuse", Version: "8.0", UserAgent: "private-ua-marker", ClientIP: "93.184.216.34"})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	first, other := login("private-device-key"), login("other-device-key")
	keys := []nativelibrary.Key{{EntityType: nativelibrary.EntityMovie, InternalKey: "tmdb:990043"}, {EntityType: nativelibrary.EntityEpisode, InternalKey: "tmdb:990044:s1e1"}, {EntityType: nativelibrary.EntityEpisode, InternalKey: "tmdb:990044:s1e2"}}
	ids, err := nativelibrary.EnsureIDs(f.ctx, catalog.IDQueries{DB: f.pool}, keys)
	if err != nil {
		t.Fatal(err)
	}
	f.media(t, 990043, "Synthetic Film", "2026", "{}")
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',990044,'Synthetic Show','{}')`, jobs.UUID())
	for _, key := range keys {
		f.exec(t, `INSERT INTO media_user_item_data(user_id,item_id,last_played_at,position_ticks,play_count,played)VALUES($1,$2,'2026-10-17T00:00:00Z',1200000000,1,false)`, id, ids[key])
	}
	sessionID := jobs.UUID()
	f.exec(t, `INSERT INTO native_playback_sessions(id,user_id,device_id,item_id,media_source_id,state)VALUES($1,$2,$3,$4,'synthetic-source','playing')`, sessionID, id, first.Device.ID, ids[keys[0]])
	f.exec(t, `UPDATE native_server_settings SET enabled=true,server_name='Synthetic Native Server',server_id=repeat('a',32),revision=revision+1`)
	c := config.Config{Environment: "development", Address: "127.0.0.1:8080", MemberAddress: "127.0.0.1:8092", MemberPublicURL: "https://native.synthetic.invalid", PublicURL: "https://admin.synthetic.invalid", SessionKey: strings.Repeat("a", 64), STRMKey: "private-global-key-marker"}
	deps := httpapi.Dependencies{DB: f.pool, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	admin := httpapi.New(c, deps)
	deps.IPBlock = fixtureIPBlock(t)
	member, err := httpapi.NativeMediaHandler(c, deps)
	if err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	call := func(handler http.Handler, method, path string, body any, csrf bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&buf).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		r := httptest.NewRequest(method, path, &buf)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if csrf {
			r.Header.Set("X-Requested-With", "media-tool")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d %s", method, path, w.Code, want, w.Body.String())
		}
		for _, secret := range []string{"private-global-key-marker", "private-device-key", "other-device-key", "private-ua-marker", first.AccessToken, other.AccessToken, "token_digest", "last_user_agent", "/115/"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatalf("management response leaked %q", secret)
			}
		}
		return w
	}
	path := "/api/v1/media-server/users/" + id
	for _, p := range []string{path + "/devices", path + "/history", "/api/v1/media-server/native/settings", "/api/v1/media-server/native/library-mappings"} {
		call(admin, "GET", p, nil, false, 401)
		call(member, "GET", p, nil, false, 404)
	}
	w := call(admin, "POST", "/api/v1/auth/register", map[string]string{"username": "management_admin", "password": "Admin123", "confirmPassword": "Admin123"}, true, 201)
	cookie = w.Result().Cookies()[0]
	w = call(admin, "GET", path+"/devices?limit=1", nil, false, 200)
	var devices struct {
		Data mediausers.ManagementPage[mediausers.Device] `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &devices); err != nil || devices.Data.Total != 3 || len(devices.Data.Items) != 1 {
		t.Fatal("devices", err, w.Body.String())
	} // includes browse fixture's initial device
	w = call(admin, "GET", path+"/devices?startIndex=99&limit=1", nil, false, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &devices); err != nil || devices.Data.Total != 3 || len(devices.Data.Items) != 0 {
		t.Fatal("empty devices page", err, w.Body.String())
	}
	w = call(admin, "GET", path+"/history?limit=2", nil, false, 200)
	var history struct {
		Data mediausers.ManagementPage[mediausers.HistoryItem] `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &history); err != nil || history.Data.Total != 3 || len(history.Data.Items) != 2 {
		t.Fatal("history", err, w.Body.String())
	}
	firstID := history.Data.Items[0].ItemID
	w = call(admin, "GET", path+"/history?limit=2", nil, false, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &history); err != nil || history.Data.Items[0].ItemID != firstID {
		t.Fatal("unstable history", err)
	}
	w = call(admin, "GET", path+"/history?startIndex=2&limit=2", nil, false, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &history); err != nil || history.Data.Total != 3 || len(history.Data.Items) != 1 {
		t.Fatal("last page", err, w.Body.String())
	}
	w = call(admin, "GET", path+"/history?startIndex=99", nil, false, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &history); err != nil || history.Data.Total != 3 || len(history.Data.Items) != 0 {
		t.Fatal("empty history", err, w.Body.String())
	}
	for _, p := range []string{path + "/devices?limit=201", path + "/history?limit=201"} {
		call(admin, "GET", p, nil, false, 400)
	}
	call(admin, "GET", "/api/v1/media-server/users/aaaaaaaa-0000-4000-8000-000000000000/history", nil, false, 404)
	revoke := path + "/devices/" + first.Device.ID + "/revoke"
	call(admin, "POST", revoke, nil, false, 403)
	call(admin, "POST", path+"/devices/aaaaaaaa-0000-4000-8000-000000000000/revoke", nil, true, 404)
	call(admin, "POST", revoke, nil, true, 200)
	call(admin, "POST", revoke, nil, true, 200)
	if _, _, _, err = store.Authenticate(f.ctx, first.AccessToken, contracts.DeviceInfo{}); !errors.Is(err, contracts.ErrUnauthenticated) {
		t.Fatal("revoked token accepted", err)
	}
	if _, _, _, err = store.Authenticate(f.ctx, other.AccessToken, contracts.DeviceInfo{}); err != nil {
		t.Fatal("other device revoked", err)
	}
	if f.text(t, `SELECT state FROM native_playback_sessions WHERE id=$1`, sessionID) != "stopped" {
		t.Fatal("device slot not released")
	}
	if f.integer(t, `SELECT count(*) FROM media_user_item_data WHERE user_id=$1 AND play_count=1`, id) != 3 {
		t.Fatal("revoke changed history")
	}
	if f.integer(t, `SELECT count(*) FROM audit_logs WHERE action='media_user.device_revoke'`) != 2 {
		t.Fatal("revoke audit missing")
	}
	// Re-login after logout is allowed, but never resurrects an old credential.
	next := login("private-device-key")
	if next.AccessToken == first.AccessToken {
		t.Fatal("reused token")
	}
	w = call(admin, "GET", "/api/v1/media-server/native/settings", nil, false, 200)
	var prefs struct {
		Data struct {
			ServerName    string `json:"serverName"`
			Enabled       bool   `json:"enabled"`
			Revision      int64  `json:"revision"`
			PublicURL     string `json:"publicUrl"`
			ListenPort    string `json:"listenPort"`
			AddressSource string `json:"addressSource"`
		} `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &prefs); err != nil || prefs.Data.PublicURL != c.MemberPublicURL || prefs.Data.ListenPort != "8092" {
		t.Fatal("address", err, w.Body.String())
	}
	update := map[string]any{"serverName": "Synthetic Living Room", "enabled": true, "revision": prefs.Data.Revision}
	call(admin, "PUT", "/api/v1/media-server/native/settings", update, false, 403)
	call(admin, "PUT", "/api/v1/media-server/native/settings", update, true, 200)
	call(admin, "PUT", "/api/v1/media-server/native/settings", update, true, 409)
	if f.integer(t, `SELECT count(*) FROM audit_logs WHERE action='native.server.settings'`) != 1 {
		t.Fatal("settings audit missing")
	}
	for _, in := range []any{map[string]any{"serverName": "Synthetic", "revision": prefs.Data.Revision + 1}, map[string]any{"serverName": strings.Repeat("中", 101), "enabled": true, "revision": prefs.Data.Revision + 1}} {
		call(admin, "PUT", "/api/v1/media-server/native/settings", in, true, 400)
	}
	// Fresh handler simulates restart: server name and enable flag come from DB.
	deps.IPBlock = fixtureIPBlock(t)
	member, err = httpapi.NativeMediaHandler(c, deps)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/EmBy/SyStEm/Info", nil)
	r.Header.Set("X-Emby-Token", other.AccessToken)
	w = httptest.NewRecorder()
	member.ServeHTTP(w, r)
	var info dto.PublicSystemInfo
	if err = json.Unmarshal(w.Body.Bytes(), &info); err != nil || w.Code != 200 || info.ServerName != "Synthetic Living Room" || info.LocalAddress != c.MemberPublicURL {
		t.Fatal("persisted System/Info", err, w.Code, w.Body.String())
	}
	call(member, "GET", "/System/Info", nil, false, 401)
	update["revision"] = prefs.Data.Revision + 1
	update["enabled"] = false
	call(admin, "PUT", "/api/v1/media-server/native/settings", update, true, 200)
	time.Sleep(300 * time.Millisecond)
	call(member, "GET", "/System/Info", nil, false, 503)
	if _, err = repo.Devices(f.ctx, id, 0, 201); !errors.Is(err, mediausers.ErrInvalid) {
		t.Fatal("repository bound", err)
	}
}

func TestNativeManagementPageSQLBoundsAndCrossUserDeviceIsolation(t *testing.T) {
	f := newBrowseFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 79); err != nil {
		t.Fatal(err)
	}
	repo := mediausers.Repository{DB: f.pool}
	id := f.text(t, `SELECT id::text FROM media_users WHERE id=$1`, f.user.ID)
	f.exec(t, `INSERT INTO media_user_devices(id,user_id,device_key,name,last_seen_at) SELECT md5('p4-device-'||g::text)::uuid,$1,'device-'||g::text,'Synthetic Device', '2026-10-17T00:00:00Z' FROM generate_series(1,205)g`, id)
	f.trace.queries.Store(0)
	page, err := repo.Devices(f.ctx, id, 0, 200)
	if err != nil || page.Total != 206 || len(page.Items) != 200 || f.trace.queries.Load() != 2 {
		t.Fatal("bounded device query", err, page.Total, len(page.Items), f.trace.queries.Load())
	}
	for i := 1; i < len(page.Items); i++ {
		if page.Items[i].LastSeenAt.Equal(page.Items[i-1].LastSeenAt) && page.Items[i].ID < page.Items[i-1].ID {
			t.Fatal("device tiebreaker not stable")
		}
	}
	page, err = repo.Devices(f.ctx, id, 200, 200)
	if err != nil || len(page.Items) != 6 || page.Total != 206 {
		t.Fatal("device last page", err)
	}
	other, err := repo.Save(f.ctx, "", mediausers.Input{Username: "other.management", Password: "Synthetic123", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.RevokeDevice(f.ctx, other.ID, page.Items[0].ID); !errors.Is(err, mediausers.ErrNotFound) {
		t.Fatal("cross-user device", err)
	}
	f.exec(t, `UPDATE media_user_devices SET name='/115/private-marker',client='api_key=secret-marker' WHERE id=$1`, page.Items[0].ID)
	safe, err := repo.Devices(f.ctx, id, 200, 200)
	if err != nil || safe.Items[0].Name != "[名称已隐藏]" || safe.Items[0].Client != "[名称已隐藏]" {
		t.Fatal("device header reflected secrets", err)
	}
	// Load more than one page without creating/exposing any real asset or media path.
	f.exec(t, `INSERT INTO native_item_ids(entity_type,internal_key)SELECT 'movie','tmdb:'||(880000+g)::text FROM generate_series(1,205)g`)
	f.exec(t, `INSERT INTO media_user_item_data(user_id,item_id,last_played_at)SELECT $1,id,'2026-10-17T00:00:00Z' FROM native_item_ids WHERE internal_key~'^tmdb:880[0-9]+$'`, id)
	f.trace.queries.Store(0)
	history, err := repo.History(f.ctx, id, 0, 200)
	if err != nil || history.Total != 205 || len(history.Items) != 200 || f.trace.queries.Load() != 2 {
		t.Fatal("bounded history query", err, history.Total, len(history.Items), f.trace.queries.Load())
	}
	history, err = repo.History(f.ctx, id, 200, 200)
	if err != nil || len(history.Items) != 5 || history.Total != 205 {
		t.Fatal("history last page", err)
	}
	empty, err := repo.History(f.ctx, other.ID, 0, 200)
	if err != nil || empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatal("history user isolation", err)
	}
	cancelled, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err = repo.Devices(cancelled, id, 0, 50); !errors.Is(err, mediausers.ErrUnavailable) {
		t.Fatal("cancellation", err)
	}
	if _, err = repo.History(cancelled, id, 0, 50); !errors.Is(err, mediausers.ErrUnavailable) {
		t.Fatal("cancellation", err)
	}
}
