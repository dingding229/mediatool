package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/playeraccess"
	"media-tool/internal/modules/storage"
	"net"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type playerCheckFunc func(context.Context, playeraccess.Request) (playeraccess.Decision, error)

func (f playerCheckFunc) Check(ctx context.Context, r playeraccess.Request) (playeraccess.Decision, error) {
	return f(ctx, r)
}
func TestUAAdmissionBeforeResolverCacheAndAllPublicPlaybackEntries(t *testing.T) {
	var policy atomic.Bool
	var calls atomic.Int32
	var seen string
	checker := playerCheckFunc(func(_ context.Context, r playeraccess.Request) (playeraccess.Decision, error) {
		if r.UserAgent != "OwnedPlayer/1" || policy.Load() {
			return playeraccess.Decision{Allowed: false, Reason: "ua_not_whitelisted"}, nil
		}
		return playeraccess.Decision{Allowed: true, Reason: "whitelisted"}, nil
	})
	engine := playback.New(resolverFunc(func(_ context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
		calls.Add(1)
		seen = ref.UserAgent
		return storage.DownloadLink{URL: "https://cdn.example.test/owned", CacheUntil: time.Now().Add(time.Minute)}, nil
	}), playback.Options{Policy: playback.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}}})
	h := New(cfg(), Dependencies{Playback: engine, Storage: resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{}, nil
	}), PlayerAccess: checker})
	invokeUA := func(method, path, ua string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("User-Agent", ua)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, method := range []string{"GET", "HEAD"} {
		w := invokeUA(method, linkTarget(), "OwnedPlayer/1")
		if w.Code != 302 || seen != "OwnedPlayer/1" {
			t.Fatal(w.Code, w.Body.String(), seen)
		}
	}
	before := calls.Load()
	policy.Store(true)
	for _, c := range []struct{ method, path string }{{"GET", linkTarget()}, {"HEAD", linkTarget()}, {"GET", "/emby/Videos/11/original.mkv"}, {"POST", "/emby/Items/11/PlaybackInfo"}, {"GET", "/emby/Emby//videos/11/stream"}, {"POST", "/api/v1/member/libraries/movies/items/asset/playback"}} {
		w := invokeUA(c.method, c.path, "OwnedPlayer/1")
		if w.Code != 403 || w.Header().Get("Location") != "" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(c, w.Code, w.Body.String())
		}
	}
	if calls.Load() != before {
		t.Fatal("denied request reached resolver")
	}
	root := EmbyHandler(h)
	r := httptest.NewRequest("GET", "/Videos/11/original.mkv", nil)
	r.Header.Set("User-Agent", "unknown")
	w := httptest.NewRecorder()
	root.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("root listener bypass", w.Code)
	}
	w = invokeUA("GET", "/emby/Users/Me", "unknown")
	if w.Code == 403 && strings.Contains(w.Body.String(), "player_ua_denied") {
		t.Fatal("login/browse was blocked")
	}
	h = New(cfg(), Dependencies{PlayerAccess: playerCheckFunc(func(context.Context, playeraccess.Request) (playeraccess.Decision, error) {
		return playeraccess.Decision{}, playeraccess.ErrUnavailable
	})})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", linkTarget(), nil))
	if w.Code != 503 || w.Header().Get("Location") != "" {
		t.Fatal("failed policy opened access")
	}
	h = New(cfg(), Dependencies{PlayerAccess: playerCheckFunc(func(context.Context, playeraccess.Request) (playeraccess.Decision, error) {
		return playeraccess.Decision{}, errors.New("unexpected")
	})})
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", linkTarget(), nil)
	r.Header["User-Agent"] = []string{"one", "two"}
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("duplicate UA")
	}
}
