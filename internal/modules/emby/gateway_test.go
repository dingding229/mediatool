package emby

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
)

type gatewayResolver struct{ calls atomic.Int32 }

func (r *gatewayResolver) Resolve(context.Context, storage.FileRef) (storage.DownloadLink, error) {
	r.calls.Add(1)
	return storage.DownloadLink{URL: "https://cdn.example.com/file?temporary=token"}, nil
}
func TestStaticGatewayAuthorizesCallerAndRejectsLegacyMediaFallback(t *testing.T) {
	builder := strm.Builder{BaseURL: "https://media.example.com", APIKey: strings.Repeat("b", 64), AllowedRoots: []string{"/115/Media"}}
	stable, _ := builder.Build(storage.FileRef{Path: "/115/Media/movie.mkv"})
	var metadataCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Users/Me" || strings.HasPrefix(r.URL.Path, "/Users/") {
			metadataCalls.Add(1)
			if r.Header.Get("X-Emby-Token") != "user-token" && r.URL.Query().Get("api_key") != "user-token" {
				w.WriteHeader(401)
				return
			}
			if r.URL.Path == "/Users/Me" || r.URL.Path == "/Users/user-1" {
				json.NewEncoder(w).Encode(map[string]any{"Id": "user-1", "Policy": map[string]bool{"EnableMediaPlayback": true}})
				return
			}
			if r.URL.Path != "/Users/user-1/Items/item-1" {
				w.WriteHeader(403)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"Id": "item-1", "PlayAccess": "Full", "MediaSources": []map[string]string{{"Id": "source-1", "Path": stable}, {"Id": "source-2", "Path": "https://unknown.example.com/"}}})
			return
		}
		w.Header().Set("X-Original-Emby", "true")
		w.Write([]byte("original-stream"))
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	resolver := &gatewayResolver{}
	engine := playback.New(resolver, playback.Options{Policy: playback.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}}})
	gateway := NewGateway(GatewayOptions{LegacyEnabled: true, Upstream: target, Enable302: true, Builder: builder, Playback: engine})
	request := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if token != "" {
			r.Header.Set("X-Emby-Token", token)
		}
		w := httptest.NewRecorder()
		gateway.ServeHTTP(w, r)
		return w
	}
	if w := request("/emby/Videos/item-1/stream.mkv?Static=true&MediaSourceId=source-1", "user-token"); w.Code != 302 || w.Header().Get("X-Media-Gateway") != "emby-static" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("/emby/Videos/item-1/stream.mkv?Static=true&MediaSourceId=source-1", ""); w.Code != 401 {
		t.Fatal("unauthenticated redirect", w.Code)
	}
	if w := request("/emby/Videos/item-2/stream.mkv?Static=true&MediaSourceId=source-1", "user-token"); w.Code != 403 {
		t.Fatal("permission bypass", w.Code)
	}
	for _, path := range []string{
		"/emby/Videos/item-1/stream.mkv?Static=true",
		"/emby/Videos/item-1/stream.mkv?Static=true&MediaSourceId=source-2",
		"/emby/Videos/item-1/stream.mp4?Static=true&MediaSourceId=source-1",
		"/emby/Videos/item-1/stream.mkv?Static=true&MediaSourceId=source-1&VideoCodec=h264",
		"/emby/Videos/item-1/master.m3u8?MediaSourceId=source-1",
		"/emby/Videos/item-1/stream.mkv?MediaSourceId=source-1",
	} {
		if w := request(path, "user-token"); w.Code != 409 || w.Header().Get("X-Original-Emby") != "" {
			t.Fatal("legacy media fallback accepted", path, w.Code)
		}
	}
	if w := request("/emby/Items/item-1/PlaybackInfo", "user-token"); w.Code != 200 || w.Header().Get("X-Original-Emby") != "true" {
		t.Fatal("metadata was blocked", w.Code)
	}
	if w := request("/emby/Videos/item-1/stream.mkv?Static=true&static=false&MediaSourceId=source-1", "user-token"); w.Code != 400 {
		t.Fatal("case-conflicting params accepted")
	}
	if resolver.calls.Load() != 1 {
		t.Fatal("unexpected direct resolver calls", resolver.calls.Load())
	}
	if metadataCalls.Load() == 0 {
		t.Fatal("metadata permissions not consulted")
	}
}
func TestManagedSTRMReadDoesNotEscapeRoot(t *testing.T) {
	root := t.TempDir()
	builder := strm.Builder{BaseURL: "https://media.example.com", APIKey: strings.Repeat("b", 64), AllowedRoots: []string{"/115/Media"}}
	stable, _ := builder.Build(storage.FileRef{Path: "/115/Media/movie.mkv"})
	if err := strm.WriteAtomic(root, "movie.strm", stable); err != nil {
		t.Fatal(err)
	}
	gateway := &Gateway{options: GatewayOptions{LegacyEnabled: true, Builder: builder, STRMOutputDir: root, STRMPathPrefix: "/strm"}}
	if ref, err := gateway.reference(context.Background(), "/strm/movie.strm"); err != nil || ref.Path != "/115/Media/movie.mkv" {
		t.Fatal(err)
	}
	for _, raw := range []string{"/strm/../private/movie.strm", "https://evil.example.com" + strm.Endpoint + "?apiKey=" + builder.APIKey + "&path=%2F115%2FMedia%2Fa.mkv", strings.Replace(stable, builder.APIKey, "invalid-key", 1), stable + "\nsecond-line"} {
		if _, err := gateway.reference(context.Background(), raw); err == nil {
			t.Fatal("unsafe mapping accepted")
		}
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.strm"), []byte(stable), 0600)
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	if _, err := gateway.reference(context.Background(), "/strm/escape/secret.strm"); err == nil {
		t.Fatal("external STRM symlink followed")
	}
}

func TestLegacyToggleAlsoAppliesToGateway(t *testing.T) {
	builder := strm.Builder{BaseURL: "https://media.example", APIKey: strings.Repeat("b", 64), AllowedRoots: []string{"/115"}}
	raw, _ := builder.Build(storage.FileRef{Path: "/115/a.mkv"})
	g := NewGateway(GatewayOptions{Builder: builder})
	if _, err := g.reference(context.Background(), raw); err == nil {
		t.Fatal("disabled legacy grant bypassed by gateway")
	}
	if !g.MetadataLink(context.Background(), raw) {
		t.Fatal("legacy secret not redacted")
	}
}

func TestNativeOriginalEndpointIsAuthorizedWithoutStaticFlag(t *testing.T) {
	builder := strm.Builder{BaseURL: "https://media.example", APIKey: strings.Repeat("b", 64), AllowedRoots: []string{"/115"}}
	stable, _ := builder.Build(storage.FileRef{Path: "/115/video.mkv"})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if callerToken(r) != "user-token" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/Users/Me", "/Users/user":
			json.NewEncoder(w).Encode(map[string]any{"Id": "user", "Policy": map[string]bool{"EnableMediaPlayback": true}})
		case "/Users/user/Items/11":
			json.NewEncoder(w).Encode(map[string]any{"Id": "11", "PlayAccess": "Full", "MediaSources": []map[string]string{{"Id": "mediasource_11", "Path": stable}}})
		default:
			w.Header().Set("X-Original-Emby", "true")
			w.Write([]byte("original"))
		}
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	resolver := &gatewayResolver{}
	engine := playback.New(resolver, playback.Options{Policy: playback.Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}}})
	gateway := NewGateway(GatewayOptions{Upstream: target, Enable302: true, LegacyEnabled: true, Builder: builder, Playback: engine})
	for _, method := range []string{"GET", "HEAD"} {
		r := httptest.NewRequest(method, "http://gateway/emby/videos/11/original.mkv?MediaSourceId=mediasource_11", nil)
		r.Header.Set("X-Emby-Token", "user-token")
		w := httptest.NewRecorder()
		gateway.ServeHTTP(w, r)
		if w.Code != 302 || w.Header().Get("X-Media-Gateway") != "emby-static" {
			t.Fatal("native original not redirected", w.Code)
		}
	}
	r := httptest.NewRequest("GET", "http://gateway/emby/videos/11/original.mkv", nil)
	w := httptest.NewRecorder()
	gateway.ServeHTTP(w, r)
	if w.Code != 401 || w.Header().Get("Location") != "" {
		t.Fatal("unauthorized original redirected")
	}
	for _, q := range []string{"Static=false", "VideoCodec=h264", "Container=mp4"} {
		r := httptest.NewRequest("GET", "http://gateway/emby/videos/11/original.mkv?"+q, nil)
		r.Header.Set("X-Emby-Token", "user-token")
		w := httptest.NewRecorder()
		gateway.ServeHTTP(w, r)
		if w.Code != 409 || w.Header().Get("X-Original-Emby") != "" {
			t.Fatal("transformation bypassed", w.Code)
		}
	}
}
