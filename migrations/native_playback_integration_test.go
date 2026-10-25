package migrations_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/redis/go-redis/v9"
	"io"
	"log/slog"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/nativelibrary/catalog"
	playbackruntime "media-tool/internal/modules/nativeplayback/runtime"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
	"media-tool/internal/platform/secrets"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type playbackTransport func(*http.Request) (*http.Response, error)

func (f playbackTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type playbackFixture struct {
	browseFixture
	a, b, other movieAsset
	item        string
	handler     http.Handler
	service     *playbackruntime.Service
	manager     *integrations.Manager
	logs        bytes.Buffer
	token       string
	mode        atomic.Int32
	downloads   atomic.Int32
	paths       atomic.Int32
	agents      sync.Map
	root        string
}
type syntheticTimeout struct{}

func (syntheticTimeout) Error() string   { return "synthetic transport timeout" }
func (syntheticTimeout) Timeout() bool   { return true }
func (syntheticTimeout) Temporary() bool { return true }
func newPlaybackFixture(t *testing.T, versions ...int64) *playbackFixture {
	target := int64(84)
	if len(versions) > 0 {
		target = versions[0]
	}
	t.Helper()
	f := &playbackFixture{browseFixture: newBrowseFixture(t, target), root: t.TempDir()}
	// Current runtime admission uses current schema, not the historical browse fixture.
	if _, e := f.provider.UpTo(f.ctx, target); e != nil {
		t.Fatal(e)
	}
	m := f.media(t, 3132001, "Synthetic Playback Film", "2026", `{"officialRating":"G"}`)
	f.a = f.asset(t, m, "g:test-a", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "mkv")
	f.b = f.asset(t, m, "g:test-a", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "mp4")
	f.other = f.asset(t, f.media(t, 3132002, "Other Synthetic Film", "2026", `{}`), "g:test-b", time.Now(), "mkv")
	for {
		n, err := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if err != nil {
			t.Fatal(err)
		}
		if n < 500 {
			break
		}
	}
	f.item = f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='movie' AND internal_key='tmdb:3132001'`)
	f.exec(t, `UPDATE native_server_settings SET enabled=true,revision=revision+1`)
	c := config.Config{SessionKey: "synthetic-native-playback-auth", STRMKey: "synthetic-global-strm-key-3132", PublicURL: "https://resolver.synthetic.invalid", MemberPublicURL: "https://native.synthetic.invalid", STRMOutputDir: f.root, STRMAllowedRoots: []string{"/115"}, LegacyPlaybackEnabled: true, PlaybackResolveTimeout: 2 * time.Second}
	vault, err := secrets.New(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	creds, err := vault.Seal(f.storage, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO integrations(id,kind,name,enabled,settings,credentials_ciphertext)VALUES($1,'clouddrive2','Synthetic scanner',true,'{"endpoint":"https://scanner.synthetic.invalid","cloudRoot":"/115"}',$2)`, f.storage, creds)
	f.manager = integrations.NewManager(integrations.Repository{DB: f.pool, Vault: vault}, c)
	t.Cleanup(f.manager.Close)
	_, err = f.manager.Repository.Save(f.ctx, "", integrations.Input{Kind: "open115", Name: "Synthetic official", Enabled: true, Settings: map[string]any{"appId": "3132", "cloudMountPrefix": "/115"}, Secrets: map[string]string{"accessToken": "synthetic-provider-access", "refreshToken": "synthetic-provider-refresh"}})
	if err != nil {
		t.Fatal(err)
	}
	f.manager.OutboundHTTP = &http.Client{Timeout: 50 * time.Millisecond, Transport: playbackTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "proapi.115.com" {
			t.Error("unexpected external host", r.URL.Host)
			return nil, errors.New("unexpected transport")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return nil, err
		}
		if r.MultipartForm != nil {
			defer func() {
				if err := r.MultipartForm.RemoveAll(); err != nil {
					t.Error(err)
				}
			}()
		}
		reply := func(raw string) *http.Response {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw)), Request: r}
		}
		if r.URL.Path == "/open/folder/get_info" {
			n := f.paths.Add(1)
			if f.mode.Load() == 3 && n == 1 {
				return nil, syntheticTimeout{}
			}
			raw, marshalErr := json.Marshal(map[string]any{"state": true, "code": 0, "data": map[string]any{"file_id": "1", "file_name": path.Base(r.FormValue("path")), "size_byte": 1, "pick_code": "abcdefgh3132", "file_category": "1"}})
			if marshalErr != nil {
				return nil, marshalErr
			}
			return reply(string(raw)), nil
		}
		if r.URL.Path != "/open/ufile/downurl" {
			t.Error("unexpected API", r.URL.Path)
			return nil, errors.New("wrong API")
		}
		f.downloads.Add(1)
		if f.mode.Load() == 4 {
			f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=$1`, f.a.file)
		}
		f.agents.Store(r.UserAgent(), true)
		switch f.mode.Load() {
		case 1:
			return reply(`{"state":false,"code":40140117}`), nil
		case 2:
			return reply(`{"state":false,"code":40140120}`), nil
		}
		// Names must match the per-path request, so retrieve the path through the
		// asset associated with the synthetic request's UA-independent file lookup.
		// FileByPath filename is the last UUID in our path; this test uses a per-UA
		// request header below to bind each simulated download to its source.
		name, _ := f.agents.Load("file-name")
		raw, marshalErr := json.Marshal(map[string]any{"state": true, "code": 0, "data": map[string]any{"1": map[string]any{"file_name": name, "file_size": 1, "pick_code": "abcdefgh3132", "url": map[string]string{"url": "https://cdn.synthetic.invalid/original?signature=SYNTHETIC_VISIBLE"}}}})
		if marshalErr != nil {
			return nil, marshalErr
		}
		return reply(string(raw)), nil
	})}
	// Wrap the fake to retain the latest path name. Tests that issue concurrent
	// requests use the same source; no real provider or CDN is contacted.
	original := f.manager.OutboundHTTP.Transport
	f.manager.OutboundHTTP.Transport = playbackTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/open/folder/get_info" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			if err = r.Body.Close(); err != nil {
				return nil, err
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			copy := r.Clone(r.Context())
			copy.Body = io.NopCloser(bytes.NewReader(body))
			if err = copy.ParseMultipartForm(1 << 20); err != nil {
				return nil, err
			}
			f.agents.Store("file-name", path.Base(copy.FormValue("path")))
			if copy.MultipartForm != nil {
				if err = copy.MultipartForm.RemoveAll(); err != nil {
					return nil, err
				}
			}
		}
		return original.RoundTrip(r)
	})
	for _, a := range []movieAsset{f.a, f.b, f.other} {
		relative := f.text(t, `SELECT output_path FROM strm_entries WHERE asset_id=$1`, a.asset)
		cloud := f.text(t, `SELECT full_path FROM cloud_files WHERE id=$1`, a.file)
		link, err := (strm.Builder{BaseURL: c.PublicURL, APIKey: c.STRMKey, AllowedRoots: c.STRMAllowedRoots}).Build(storage.FileRef{Path: cloud})
		if err != nil {
			t.Fatal(err)
		}
		if err = strm.WriteAtomic(f.root, relative, link); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(link + "\n"))
		f.exec(t, `INSERT INTO managed_media_artifacts(relative_path,asset_id,artifact_kind,owner_scope,content_digest)VALUES($1,$2::uuid,'strm','asset:'||($2::uuid)::text,$3)`, relative, a.asset, digest[:])
	}
	logger := slog.New(slog.NewJSONHandler(&f.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f.service = playbackruntime.New(f.pool, c, f.manager, nil, nil, logger)
	resolver := f.service.Links.(*playbackruntime.Resolver)
	resolver.Policy.Lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	f.handler, err = httpapi.NativeMediaHandler(c, httpapi.Dependencies{DB: f.pool, Integrations: f.manager, NativePlayback: f.service, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	f.token = f.login(t, "native-device")
	return f
}
func (f *playbackFixture) login(t *testing.T, device string) string {
	t.Helper()
	r := httptest.NewRequest("POST", "/Users/AuthenticateByName", strings.NewReader(`{"Username":"browse.member","Pw":"Browse123"}`))
	r.Header.Set("User-Agent", "Infuse/8")
	r.Header.Set("X-Emby-Authorization", `MediaBrowser Client="Infuse", DeviceId="`+device+`"`)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	var result dto.AuthenticationResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	return result.AccessToken
}
func (f *playbackFixture) call(method, path, token, ua string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("X-Emby-Token", token)
	r.Header.Set("User-Agent", ua)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}
func TestNativePlaybackFullSequenceAndSlots(t *testing.T) {
	f := newPlaybackFixture(t)
	f.probeSnapshot(t, f.a, 600000000, 1920)
	items := f.call("GET", "/Items?IncludeItemTypes=Movie&Recursive=true", f.token, "Infuse/8")
	if items.Code != 200 {
		t.Fatal(items.Code, items.Body.String())
	}
	info := f.call("GET", "/emby/Items/"+f.item+"/PlaybackInfo", f.token, "Infuse/8")
	var data dto.PlaybackInfoResponse
	if err := json.Unmarshal(info.Body.Bytes(), &data); err != nil || info.Code != 200 || len(data.MediaSources) != 2 {
		t.Fatal(info.Code, info.Body.String(), err)
	}
	if f.integer(t, `SELECT count(*) FROM native_playback_sessions WHERE state<>'stopped'`) != 0 {
		t.Fatal("PlaybackInfo consumed slot")
	}
	for _, m := range data.MediaSources {
		u, err := url.Parse(m.Path)
		if err != nil || u.Host != "native.synthetic.invalid" || u.RawQuery == "" || u.Query().Get("api_key") != "" || !strings.Contains(m.DirectStreamUrl, "api_key="+f.token) || m.SupportsTranscoding() {
			t.Fatal("unsafe media source")
		}
	}
	post := httptest.NewRequest("POST", "/emby/Items/"+f.item+"/PlaybackInfo", strings.NewReader(`{"MediaSourceId":"`+f.a.asset+`","DeviceProfile":{"Name":"Infuse Synthetic"},"UserId":null,"AudioStreamIndex":null,"SubtitleStreamIndex":null,"EnableTranscoding":true}`))
	post.Header.Set("X-Emby-Token", f.token)
	post.Header.Set("User-Agent", "Infuse/8")
	postResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(postResponse, post)
	var selected dto.PlaybackInfoResponse
	if err := json.Unmarshal(postResponse.Body.Bytes(), &selected); err != nil || postResponse.Code != 200 || len(selected.MediaSources) != 1 || selected.MediaSources[0].Id != f.a.asset || len(selected.MediaSources[0].MediaStreams) != 2 {
		t.Fatal("POST PlaybackInfo snapshot", postResponse.Code, err, "source count", len(selected.MediaSources), "streams", func() int {
			if len(selected.MediaSources) == 1 {
				return len(selected.MediaSources[0].MediaStreams)
			}
			return -1
		}())
	}
	stream := "/Videos/" + f.item + "/stream.mkv?Static=true&MediaSourceId=" + f.a.asset
	for _, method := range []string{"GET", "HEAD", "GET"} {
		w := f.call(method, stream, f.token, "Infuse/8")
		if w.Code != 302 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "0" || w.Header().Get("Location") != "https://cdn.synthetic.invalid/original?signature=SYNTHETIC_VISIBLE" {
			t.Fatal(method, w.Code, w.Body.String())
		}
	}
	if _, ok := f.agents.Load("Infuse/8"); !ok {
		t.Fatal("real UA not used")
	}
	if f.integer(t, `SELECT count(*) FROM native_playback_sessions WHERE state<>'stopped'`) != 1 {
		t.Fatal("seek multiplied slots")
	}
	if !strings.Contains(f.logs.String(), "SYNTHETIC_VISIBLE") || strings.Contains(f.logs.String(), f.token) || strings.Contains(f.logs.String(), "synthetic-global-strm-key-3132") {
		t.Fatal("logging URL/token policy")
	}
	f.exec(t, `UPDATE media_users SET max_streams=1 WHERE id=$1`, f.user.ID)
	other := f.login(t, "other-device")
	if w := f.call("GET", stream, other, "Infuse/8"); w.Code != 403 || !strings.Contains(w.Body.String(), "同时播放") {
		t.Fatal(w.Code, w.Body.String())
	}
	f.exec(t, `UPDATE native_playback_sessions SET started_at=now()-interval '10 minutes',last_progress_at=now()-interval '6 minutes' WHERE state<>'stopped'`)
	if w := f.call("GET", stream, other, "Infuse/8"); w.Code != 302 {
		t.Fatal("TTL did not release", w.Code, w.Body.String())
	}
	if f.integer(t, `SELECT count(*) FROM native_playback_sessions WHERE state<>'stopped'`) != 1 {
		t.Fatal("expired slot not converged")
	}
}
func TestNativePlaybackPermissionTokenUAAndSourceFailures(t *testing.T) {
	f := newPlaybackFixture(t)
	stream := "/Videos/" + f.item + "/stream.mkv?MediaSourceId=" + f.a.asset
	for _, token := range []string{"", "synthetic-global-strm-key-3132"} {
		if w := f.call("GET", stream, token, "Infuse/8"); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	if w := f.call("GET", stream+"&UserId=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", f.token, "Infuse/8"); w.Code != 403 {
		t.Fatal("cross user", w.Code)
	}
	if w := f.call("GET", "/Videos/"+f.item+"/stream?MediaSourceId="+f.other.asset, f.token, "Infuse/8"); w.Code != 404 {
		t.Fatal("foreign source", w.Code)
	}
	if w := f.call("GET", "/Items/"+f.item+"/Download?MediaSourceId="+f.a.asset, f.token, "Infuse/8"); w.Code != 403 {
		t.Fatal("download allowed", w.Code)
	}
	f.exec(t, `UPDATE media_user_library_grants SET can_play=false WHERE user_id=$1 AND library_id='g:test-a'`, f.user.ID)
	if w := f.call("GET", "/Items/"+f.item+"/PlaybackInfo", f.token, "Infuse/8"); w.Code != 403 {
		t.Fatal("browse-only play allowed", w.Code, w.Body.String())
	}
	f.exec(t, `UPDATE media_user_library_grants SET can_play=true WHERE user_id=$1 AND library_id='g:test-a'`, f.user.ID)
	f.exec(t, `UPDATE native_libraries SET hidden=true WHERE id='g:test-a'`)
	if w := f.call("GET", stream, f.token, "Infuse/8"); w.Code != 404 {
		t.Fatal("hidden source visible", w.Code)
	}
	f.exec(t, `UPDATE native_libraries SET hidden=false WHERE id='g:test-a'`)
	f.exec(t, `UPDATE player_ua_policy SET enabled=true,mode='whitelist' WHERE id='main'`)
	f.exec(t, `INSERT INTO player_ua_rules(id,label,match_type,user_agent,enabled)VALUES(gen_random_uuid(),'Synthetic','prefix','Infuse/',true)`)
	if w := f.call("GET", stream, f.token, "ForbiddenPlayer/1"); w.Code != 403 {
		t.Fatal("UA allowed", w.Code, w.Body.String())
	}
	f.exec(t, `UPDATE media_users SET expires_at=now()-interval '1 second' WHERE id=$1`, f.user.ID)
	if w := f.call("GET", stream, f.token, "Infuse/8"); w.Code != 401 {
		t.Fatal("expired user", w.Code)
	}
	f.exec(t, `UPDATE media_users SET expires_at=NULL,enabled=false WHERE id=$1`, f.user.ID)
	if w := f.call("GET", stream, f.token, "Infuse/8"); w.Code != 401 {
		t.Fatal("disabled user", w.Code)
	}
	f.exec(t, `UPDATE native_server_settings SET enabled=false,revision=revision+1`)
	time.Sleep(270 * time.Millisecond)
	if w := f.call("GET", stream, f.token, "Infuse/8"); w.Code != 503 {
		t.Fatal("disabled server", w.Code)
	}
}
func TestNativePlaybackLimitErrorsRetryAndExposureFence(t *testing.T) {
	for _, mode := range []int32{1, 2, 3, 4} {
		t.Run(strconv.Itoa(int(mode)), func(t *testing.T) {
			f := newPlaybackFixture(t)
			f.mode.Store(mode)
			w := f.call("GET", "/Videos/"+f.item+"/stream?MediaSourceId="+f.a.asset, f.token, "Infuse/8")
			want := 503
			if mode == 1 {
				want = 429
			}
			if mode == 3 {
				want = 302
			}
			if mode == 4 {
				want = 404
			}
			if w.Code != want {
				t.Fatal(w.Code, w.Body.String())
			}
			if mode == 1 && w.Header().Get("Retry-After") == "" {
				t.Fatal("rate limit lacks retry")
			}
			if mode == 3 && f.paths.Load() != 2 {
				t.Fatal("bounded timeout retry missing", f.paths.Load())
			}
			if mode != 3 && f.integer(t, `SELECT count(*) FROM native_playback_sessions WHERE state<>'stopped'`) != 0 {
				t.Fatal("failed lookup retained new slot")
			}
		})
	}
}

func TestNativePlaybackHeaderQueryVariantsDownloadAndPerVersionPermission(t *testing.T) {
	f := newPlaybackFixture(t)
	for _, variant := range []int{0, 1} {
		path := "/eMbY/ViDeOs/" + f.item + "/OrIgInAl.mp4?MediaSourceId=" + f.a.asset
		r := httptest.NewRequest("HEAD", path, nil)
		r.Header.Set("User-Agent", "SenPlayer/2")
		if variant == 0 {
			q := r.URL.Query()
			q.Set("API_KEY", f.token)
			r.URL.RawQuery = q.Encode()
		} else {
			r.Header.Set("Authorization", `MediaBrowser Token="`+f.token+`"`)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 302 || w.Body.Len() != 0 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if _, ok := f.agents.Load("SenPlayer/2"); !ok {
		t.Fatal("header UA lost")
	}
	f.exec(t, `UPDATE media_user_library_grants SET can_download=true WHERE user_id=$1 AND library_id='g:test-a'`, f.user.ID)
	if w := f.call("GET", "/Items/"+f.item+"/Download?MediaSourceId="+f.a.asset, f.token, "Infuse/8"); w.Code != 302 || w.Body.Len() != 0 {
		t.Fatal("download permission", w.Code, w.Body.String())
	}
	f.exec(t, `UPDATE native_asset_libraries SET library_id='g:test-b' WHERE asset_id=$1`, f.b.asset)
	f.exec(t, `UPDATE media_user_library_grants SET can_play=false WHERE user_id=$1 AND library_id='g:test-b'`, f.user.ID)
	w := f.call("GET", "/Items/"+f.item+"/PlaybackInfo", f.token, "Infuse/8")
	var info dto.PlaybackInfoResponse
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil || w.Code != 200 || len(info.MediaSources) != 1 || info.MediaSources[0].Id != f.a.asset {
		t.Fatal("nonplayable version advertised", w.Code, w.Body.String(), err)
	}
	if w := f.call("GET", "/Videos/"+f.item+"/stream?MediaSourceId="+f.b.asset, f.token, "Infuse/8"); w.Code != 403 {
		t.Fatal("version permission bypass", w.Code)
	}
	relative := f.text(t, `SELECT output_path FROM strm_entries WHERE asset_id=$1`, f.a.asset)
	if err := os.Remove(filepath.Join(f.root, relative)); err != nil {
		t.Fatal(err)
	}
	if w := f.call("GET", "/Videos/"+f.item+"/stream?MediaSourceId="+f.a.asset, f.token, "Infuse/8"); w.Code != 404 {
		t.Fatal("missing STRM exposed", w.Code, w.Body.String())
	}
}
func TestNativePlaybackConcurrentSeekAndAtomicUserSlots(t *testing.T) {
	f := newPlaybackFixture(t)
	f.exec(t, `UPDATE media_users SET max_streams=1 WHERE id=$1`, f.user.ID)
	stream := "/Videos/" + f.item + "/stream?MediaSourceId=" + f.a.asset
	var wg sync.WaitGroup
	codes := make(chan int, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- f.call("GET", stream, f.token, "Infuse/8").Code }()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 302 {
			t.Fatal("same key concurrency", code)
		}
	}
	if f.integer(t, `SELECT count(*) FROM native_playback_sessions WHERE state<>'stopped'`) != 1 {
		t.Fatal("same-key slot duplicates")
	}
	f.exec(t, `UPDATE native_playback_sessions SET state='stopped',ended_at=clock_timestamp() WHERE state<>'stopped'`)
	other := f.login(t, "race-device")
	codes = make(chan int, 2)
	for _, token := range []string{f.token, other} {
		wg.Add(1)
		go func(token string) { defer wg.Done(); codes <- f.call("GET", stream, token, "Infuse/8").Code }(token)
	}
	wg.Wait()
	close(codes)
	success, limited := 0, 0
	for code := range codes {
		if code == 302 {
			success++
		} else if code == 403 {
			limited++
		} else {
			t.Fatal("unexpected slot result", code)
		}
	}
	if success != 1 || limited != 1 {
		t.Fatal("non-atomic limit", success, limited)
	}
}

func TestNativePlaybackExistingUACacheAndCooldown(t *testing.T) {
	f := newPlaybackFixture(t)
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("isolated Redis DB15 required")
	}
	opts, err := redis.ParseURL(raw)
	if err != nil || opts.DB != 15 {
		t.Fatal("only Redis DB15 is allowed", err)
	}
	cache := redis.NewClient(opts)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Error(err)
		}
	})
	if err = cache.Ping(f.ctx).Err(); err != nil {
		t.Fatal(err)
	}
	c := f.service.Links.(*playbackruntime.Resolver).Config
	f.service = playbackruntime.New(f.pool, c, f.manager, cache, nil, slog.New(slog.NewJSONHandler(&f.logs, nil)))
	f.service.Links.(*playbackruntime.Resolver).Policy.Lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	f.handler, err = httpapi.NativeMediaHandler(c, httpapi.Dependencies{DB: f.pool, Integrations: f.manager, NativePlayback: f.service})
	if err != nil {
		t.Fatal(err)
	}
	stream := "/Videos/" + f.item + "/stream?MediaSourceId=" + f.a.asset
	for i := 0; i < 2; i++ {
		if w := f.call("GET", stream, f.token, "Infuse/8"); w.Code != 302 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if f.downloads.Load() != 1 {
		t.Fatal("seek missed existing cache", f.downloads.Load())
	}
	if w := f.call("HEAD", stream, f.token, "SenPlayer/2"); w.Code != 302 {
		t.Fatal(w.Code)
	}
	if f.downloads.Load() != 2 {
		t.Fatal("UA cache not partitioned", f.downloads.Load())
	}
	request := httptest.NewRequest("GET", stream, nil)
	request.RemoteAddr = "9.9.9.9:123"
	request.Header.Set("X-Emby-Token", f.token)
	request.Header.Set("User-Agent", "Infuse/8")
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, request)
	if w.Code != 302 || f.downloads.Load() != 3 {
		t.Fatal("IP cache not partitioned", w.Code, f.downloads.Load())
	}
	f.mode.Store(1)
	if w := f.call("GET", stream, f.token, "NewPlayer/3"); w.Code != 429 {
		t.Fatal(w.Code)
	}
	before := f.downloads.Load()
	if w := f.call("GET", stream, f.token, "AnotherPlayer/4"); w.Code != 429 || f.downloads.Load() != before {
		t.Fatal("cooldown missed", w.Code, f.downloads.Load())
	}
	// Valid UA-bound cached links do not depend on an unavailable/rate-limited API.
	if w := f.call("HEAD", stream, f.token, "Infuse/8"); w.Code != 302 || f.downloads.Load() != before {
		t.Fatal("cooldown blocked cache hit", w.Code)
	}
}
