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

func TestDirectOnlyNoTranscodeOrMediaFallbackEverReachesUpstream(t *testing.T) {
	builder := strm.Builder{BaseURL: "https://owned.test", APIKey: strings.Repeat("b", 64), AllowedRoots: []string{"/115"}}
	fixed, _ := builder.Build(storage.FileRef{Path: "/115/owned.ts"})
	var media, cdn atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/Users/") {
			if r.Header.Get("X-Emby-Token") != "caller" {
				w.WriteHeader(401)
				return
			}
			if r.URL.Path == "/Users/Me" || r.URL.Path == "/Users/user" {
				json.NewEncoder(w).Encode(map[string]any{"Id": "user", "Policy": map[string]bool{"EnableMediaPlayback": true}})
				return
			}
			if r.URL.Path != "/Users/user/Items/17" {
				w.WriteHeader(403)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"Id": "17", "PlayAccess": "Full", "MediaSources": []map[string]string{{"Id": "mediasource_17", "Path": fixed}}})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/Videos/") || strings.HasPrefix(r.URL.Path, "/Audio/") {
			media.Add(1)
		}
		io.WriteString(w, "upstream metadata")
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	resolver := &gatewayResolver{}
	e := playback.New(resolver, playback.Options{Policy: playback.Policy{Lookup: publicLookupForDirectOnly}})
	g := NewGateway(GatewayOptions{Upstream: target, Enable302: true, DirectPlayOnly: true, LegacyEnabled: true, Builder: builder, Playback: e})
	request := func(path, token, method string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://gateway"+path, nil)
		if token != "" {
			r.Header.Set("X-Emby-Token", token)
		}
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/Videos/17/stream?Static=true&MediaSourceId=mediasource_17", "/emby/Videos/17/original.ts?MediaSourceId=mediasource_17"} {
		for _, method := range []string{"GET", "HEAD"} {
			w := request(path, "caller", method)
			if w.Code != 302 || w.Header().Get("X-Media-Playback-Policy") != "direct-only" {
				t.Fatal(path, method, w.Code, w.Body.String())
			}
		}
	}
	for _, path := range []string{
		"/Items/17/Download",
		"/emby/Items/17/File",
	} {
		w := request(path, "caller", "GET")
		if w.Code != 302 || w.Header().Get("Location") == "" {
			t.Fatal("managed item file did not use direct redirect", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{
		"/Videos/17/master.m3u8?MediaSourceId=mediasource_17",
		"/Videos/17/main.m3u8", "/Videos/17/future-media-output", "/Audio/17/universal", "/Videos/17/hls1/main/0.ts", "/Videos/17/stream.m3u8?Static=true",
		"/Videos/17/live.mpd", "/emby/Emby/Videos/17/master.m3u8", "/emby//Videos/17/master.m3u8/", "/Videos/17/stream.mp4?Static=true&MediaSourceId=mediasource_17",
		"/Videos/17/stream?MediaSourceId=mediasource_17", "/Videos/17/original.mkv?MediaSourceId=mediasource_17",
		"/Videos/17/stream?Static=true&MediaSourceId=mediasource_17&VideoCodec=h264",
		"/Videos/17/stream?Static=true&MediaSourceId=unknown",
		"/Sync/17/File",
		"/emby//Sync/17/Download",
		"/LiveTv/LiveStream/id/stream",
		"/emby/livetv/recording/id/file",
	} {
		w := request(path, "caller", "GET")
		if w.Code != 409 || w.Header().Get("Location") != "" || !strings.Contains(w.Body.String(), "direct_play_required") {
			t.Fatal("not explicitly rejected", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/Videos/17/master.m3u8", "/Videos/17/stream?MediaSourceId=mediasource_17"} {
		if w := request(path, "", "GET"); w.Code != 401 {
			t.Fatal("missing authentication", w.Code)
		}
	}
	if media.Load() != 0 || cdn.Load() != 0 || resolver.calls.Load() != 6 {
		t.Fatal("upstream media fallback or extra resolve", media.Load(), resolver.calls.Load())
	}
	g.options.Enable302 = false
	if w := request("/Videos/17/stream?Static=true&MediaSourceId=mediasource_17", "caller", "GET"); w.Code != 409 || media.Load() != 0 {
		t.Fatal("disabled interception bypassed direct-only")
	}
}

func TestStrictManagedMetadataPreservesSTRMGrantAndDoesNotInventFormatSupport(t *testing.T) {
	fixed := "https://owned/api/v1/cloudStorage/strm302?apiKey=PRIVATE"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"MediaSources": []map[string]any{{"Id": "s", "Path": fixed, "SupportsDirectPlay": false, "SupportsDirectStream": true, "SupportsTranscoding": true, "DirectStreamUrl": "/Videos/17/master.m3u8?AudioCodec=aac", "TranscodingUrl": "/Videos/17/master.m3u8?VideoCodec=h264", "TranscodingContainer": "ts", "RequiredHttpHeaders": map[string]string{"Cookie": "SECRET"}}, {"Id": "foreign", "Path": "https://foreign.test/other", "SupportsTranscoding": true}}})
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	predicate := func(ctx context.Context, raw string) bool { return raw == fixed }
	proxy := newProxy(target, predicate, nil, nil)
	r := httptest.NewRequest("POST", "http://gateway/emby/Items/17/PlaybackInfo", nil)
	r = r.WithContext(context.WithValue(r.Context(), directPlayOnlyKey{}, true))
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "PRIVATE") || strings.Contains(w.Body.String(), "SECRET") {
		t.Fatal("metadata redaction failed", w.Code)
	}
	var data struct{ MediaSources []map[string]any }
	if json.Unmarshal(w.Body.Bytes(), &data) != nil {
		t.Fatal("bad metadata")
	}
	s := data.MediaSources[0]
	if s["SupportsDirectPlay"] != false || s["SupportsDirectStream"] != false || s["SupportsTranscoding"] != false {
		t.Fatal("capabilities fabricated", s)
	}
	for _, field := range []string{"TranscodingUrl", "DirectStreamUrl", "TranscodingContainer", "RequiredHttpHeaders"} {
		if _, ok := s[field]; ok {
			t.Fatal("unsafe output survived", field)
		}
	}
	if s["Path"] != fixed || s["IsRemote"] != true || s["Protocol"] != "Http" || data.MediaSources[1]["SupportsTranscoding"] != true {
		t.Fatal("foreign source changed")
	}
}

func publicLookupForDirectOnly(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

func TestDirectOnlyOpaqueMediaRoutesNeverReachReverseProxy(t *testing.T) {
	var forwarded atomic.Int32
	g := &Gateway{options: GatewayOptions{DirectPlayOnly: true, Enable302: true, Playback: playback.New(nil, playback.Options{})}, proxy: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1); w.WriteHeader(200) })}
	for _, route := range []string{"/Sync/17/File", "/Sync/17/Download", "/Sync/JobItems/17/File", "/LiveTv/LiveStream/17/stream.ts", "/LiveTv/LiveStreamFiles/17/stream.ts", "/LiveTv/Recordings/17/stream"} {
		for _, variant := range []string{route, strings.ToLower(route), "/emby" + route, "/emby//EmBy" + strings.ReplaceAll(route, "/", "//")} {
			for _, method := range []string{"GET", "HEAD"} {
				w := httptest.NewRecorder()
				g.ServeHTTP(w, httptest.NewRequest(method, "http://owned"+variant, nil))
				if w.Code != 409 || w.Header().Get("Location") != "" {
					t.Fatal("opaque media path was not denied", method, variant, w.Code)
				}
			}
		}
	}
	if forwarded.Load() != 0 {
		t.Fatal("strict policy forwarded media", forwarded.Load())
	}
}
