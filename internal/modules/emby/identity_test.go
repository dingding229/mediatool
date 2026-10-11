package emby

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
)

// Reproduces real Emby 4.10: Me fails, PlayAccess is absent, mpegts -> .ts.
func TestEmby410LoginIdentityAndStaticTransportStream(t *testing.T) {
	builder := strm.Builder{BaseURL: "https://media.example", APIKey: strings.Repeat("b", 64), AllowedRoots: []string{"/115"}}
	stable, _ := builder.Build(storage.FileRef{Path: "/115/movie.ts"})
	var meCalls atomic.Int32
	revoked, denyPlayback, deniedItem, playbackError, mismatchedSource := false, false, false, false, false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/Users/AuthenticateByName" {
			// Successful authentication is the authoritative token/user relationship.
			io.WriteString(w, `{"User":{"Id":"real-user"},"AccessToken":"actual-user-token"}`)
			return
		}
		if r.URL.Path == "/Users/Me" {
			meCalls.Add(1)
			w.WriteHeader(500)
			return
		}
		if callerToken(r) != "actual-user-token" || revoked {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/Users/real-user":
			json.NewEncoder(w).Encode(map[string]any{"Id": "real-user", "Policy": map[string]bool{"EnableMediaPlayback": !denyPlayback}})
		case "/Users/real-user/Items/9":
			if deniedItem {
				w.WriteHeader(403)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"Id": "9", "MediaSources": []map[string]string{{"Id": "mediasource_9", "Path": stable}}})
		case "/Items/9/PlaybackInfo":
			if r.URL.Query().Get("UserId") != "real-user" {
				t.Error("trusted client UserId")
			}
			sourcePath := stable
			if mismatchedSource {
				sourcePath = "https://foreign.example/file"
			}
			var code any
			if playbackError {
				code = "NotAllowed"
			}
			json.NewEncoder(w).Encode(map[string]any{"ErrorCode": code, "MediaSources": []map[string]string{{"Id": "mediasource_9", "Path": sourcePath}}})
		default:
			w.Header().Set("X-Original-Emby", "true")
			io.WriteString(w, `{}`)
		}
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	identities := NewMemoryIdentityStore()
	resolver := &gatewayResolver{}
	engine := playback.New(resolver, playback.Options{Policy: playback.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}}})
	opts := GatewayOptions{Upstream: target, Enable302: true, LegacyEnabled: true, Builder: builder, Playback: engine, Identities: identities}
	request := func(method, p, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, p, nil)
		if token != "" {
			r.Header.Set("X-Emby-Token", token)
		}
		w := httptest.NewRecorder()
		// Dynamic gateways share the store across requests.
		NewGateway(opts).ServeHTTP(w, r)
		return w
	}
	static := "/emby/videos/9/stream.mpegts?Static=true&MediaSourceId=mediasource_9&Container=mpegts&UserId=attacker"
	if w := request("GET", static, "actual-user-token"); w.Code == 302 {
		t.Fatal("unknown caller identity trusted")
	}
	if w := request("POST", "/emby/Users/AuthenticateByName", ""); w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("login failed", w.Code)
	}
	for _, method := range []string{"GET", "HEAD"} {
		w := request(method, static, "actual-user-token")
		if w.Code != 302 || w.Header().Get("X-Media-Gateway-Reason") != "authorized_static" {
			t.Fatal("static TS not redirected", w.Code, w.Body.String())
		}
	}
	if meCalls.Load() != 1 {
		t.Fatal("known identity still used unsupported Me", meCalls.Load())
	}
	for _, path := range []string{
		strings.Replace(static, "stream.mpegts", "stream.mp4", 1),
		strings.Replace(static, "Container=mpegts", "Container=mp4", 1),
		static + "&AudioCodec=aac",
		"/emby/videos/9/hls1/main/0.ts",
		"/emby/videos/9/master.m3u8",
	} {
		w := request("GET", path, "actual-user-token")
		if w.Code != 200 || w.Header().Get("X-Original-Emby") != "true" {
			t.Fatal("remux/transcoding not preserved", w.Code)
		}
	}
	denyPlayback = true
	if w := request("GET", static, "actual-user-token"); w.Code != 403 {
		t.Fatal("playback policy bypass", w.Code)
	}
	denyPlayback = false
	deniedItem = true
	if w := request("GET", static, "actual-user-token"); w.Code != 403 {
		t.Fatal("item permission bypass", w.Code)
	}
	deniedItem = false
	playbackError = true
	if w := request("GET", static, "actual-user-token"); w.Code != 403 {
		t.Fatal("PlaybackInfo error bypass", w.Code)
	}
	playbackError = false
	mismatchedSource = true
	if w := request("GET", static, "actual-user-token"); w.Code == 302 {
		t.Fatal("unconfirmed source redirected")
	}
	mismatchedSource = false
	revoked = true
	if w := request("GET", static, "actual-user-token"); w.Code != 401 {
		t.Fatal("revoked token used", w.Code)
	}
	if _, err := identities.Lookup(context.Background(), IdentityKey(target, "actual-user-token")); err == nil {
		t.Fatal("revoked binding retained")
	}
	revoked = false
	request("POST", "/emby/Users/AuthenticateByName", "")
	request("POST", "/emby/Sessions/Logout", "actual-user-token")
	if _, err := identities.Lookup(context.Background(), IdentityKey(target, "actual-user-token")); err == nil {
		t.Fatal("logout did not remove binding")
	}
	if resolver.calls.Load() != 2 {
		t.Fatal("unsafe resolution occurred", resolver.calls.Load())
	}
}

func TestIdentityDigestScopedAndNoRawTokens(t *testing.T) {
	a, _ := url.Parse("http://emby:8096/")
	b, _ := url.Parse("http://other:8096/")
	key := IdentityKey(a, "secret-token")
	if strings.Contains(key, "secret-token") || key == IdentityKey(b, "secret-token") {
		t.Fatal("credential storage is not isolated")
	}
	c, _ := url.Parse("http://EMBY:8096")
	if key != IdentityKey(c, "secret-token") {
		t.Fatal("equivalent endpoint not normalized")
	}
}

func TestFailedLoginCannotBindIdentity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, `{"User":{"Id":"attacker"},"AccessToken":"stolen"}`)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	store := NewMemoryIdentityStore()
	g := NewGateway(GatewayOptions{Upstream: target, Identities: store})
	g.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/emby/Users/AuthenticateByName", nil))
	if _, err := store.Lookup(context.Background(), IdentityKey(target, "stolen")); err == nil {
		t.Fatal("failed login trusted")
	}
}

func TestConflictingCredentialsRejected(t *testing.T) {
	r := httptest.NewRequest("GET", "http://gateway/stream?api_key=one", nil)
	r.Header.Set("X-Emby-Token", "two")
	if _, err := unambiguousToken(r); err == nil {
		t.Fatal("ambiguous credentials accepted")
	}
	r.Header.Set("X-Emby-Token", "one")
	r.Header.Set("Authorization", `Emby UserId="spoofed", Token="one"`)
	if token, err := unambiguousToken(r); err != nil || token != "one" {
		t.Fatal("same credential not accepted")
	}
}
