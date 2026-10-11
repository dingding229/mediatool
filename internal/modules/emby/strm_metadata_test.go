package emby

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDirectOnlySTRMPathIsExactForItemsAndPlaybackInfoWithoutPrebindingUA(t *testing.T) {
	// Fixed query order/escaping must survive JSON encoding and metadata rewriting.
	raw := "https://owned.test/api/v1/cloudStorage/strm302?path=%2F115%2F%E7%94%B5%E5%BD%B1%2Fa%20b.ts&assetId=owned&apiKey=FILE-GRANT&pickCode=pc"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "USER-TOKEN" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		source := map[string]any{"Id": "source", "Path": raw, "SupportsDirectPlay": true, "SupportsDirectStream": true, "SupportsTranscoding": true, "DirectStreamUrl": "/Videos/17/stream?Static=true&api_key=USER-TOKEN", "TranscodingUrl": "/Videos/17/master.m3u8?api_key=USER-TOKEN", "RequiredHttpHeaders": map[string]string{"Cookie": "PROVIDER-COOKIE"}}
		if r.URL.Path == "/Items" {
			json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{{"Id": "17", "MediaSources": []any{source}}}})
		} else {
			json.NewEncoder(w).Encode(map[string]any{"MediaSources": []any{source}})
		}
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	var providerCalls atomic.Int32
	proxy := newProxy(target, func(ctx context.Context, s string) bool { return s == raw }, nil, func(ctx context.Context, s string) map[string]string {
		providerCalls.Add(1)
		return map[string]string{"User-Agent": "metadata-UA"}
	})
	for _, route := range []struct {
		name, prefix string
		root         bool
	}{
		{"management_emby_mount", "/emby", false},
		{"dedicated_gateway_root", "", true},
	} {
		for _, request := range []struct{ method, endpoint string }{
			{"GET", "/Items"},
			{"GET", "/Items/17/PlaybackInfo"},
			{"POST", "/Items/17/PlaybackInfo"},
			{"GET", "/Users/user/Items/17"},
		} {
			for _, agent := range []string{"SenPlayer/6.2.1", "Mozilla/5.0"} {
				r := httptest.NewRequest(request.method, "http://gateway.test:25015"+route.prefix+request.endpoint, nil)
				if route.root {
					r = r.WithContext(WithRootOrigin(r.Context()))
				}
				r.Header.Set("X-Emby-Token", "USER-TOKEN")
				r.Header.Set("User-Agent", agent)
				r = r.WithContext(context.WithValue(r.Context(), directPlayOnlyKey{}, true))
				w := httptest.NewRecorder()
				proxy.ServeHTTP(w, r)
				if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "/Videos/17/stream") || strings.Contains(w.Body.String(), "USER-TOKEN") || strings.Contains(w.Body.String(), "PROVIDER-COOKIE") {
					t.Fatal("STRM metadata path/header contract", route.name, request.method, request.endpoint, w.Code)
				}
				var body map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if items, ok := body["Items"].([]any); ok {
					body = items[0].(map[string]any)
				}
				s := body["MediaSources"].([]any)[0].(map[string]any)
				if s["Path"] != raw || s["Protocol"] != "Http" || s["IsRemote"] != true || s["SupportsTranscoding"] != false || s["SupportsDirectStream"] != false || s["SupportsDirectPlay"] != true {
					t.Fatal("original URL/capabilities changed")
				}
				for _, field := range []string{"RequiredHttpHeaders", "DirectStreamUrl", "TranscodingUrl"} {
					if _, ok := s[field]; ok {
						t.Fatal("unexpected playback indirection or prebound headers", route.name, field)
					}
				}
			}
		}
	}
	if providerCalls.Load() != 0 {
		t.Fatal("metadata acquired UA-specific cloud link")
	}
	r := httptest.NewRequest("GET", "http://gateway/emby/Items/17/PlaybackInfo", nil)
	r = r.WithContext(context.WithValue(r.Context(), directPlayOnlyKey{}, true))
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, r)
	if w.Code != 401 || strings.Contains(w.Body.String(), "FILE-GRANT") {
		t.Fatal("anonymous metadata exposed file grant")
	}
	// Unknown/non-owned URL remains untouched, including arbitrary missing identity.
	var node any = map[string]any{"MediaSources": []any{map[string]any{"Path": "https://foreign.test/other", "SupportsTranscoding": true}}}
	rewriteJSON(context.Background(), node, target, publicOrigin{DirectPlayOnly: true}, "", func(context.Context, string) bool { return false }, nil)
	if node.(map[string]any)["MediaSources"].([]any)[0].(map[string]any)["SupportsTranscoding"] != true {
		t.Fatal("unowned source altered")
	}
}

func TestDirectOnlyPreservesSameUpstreamHostSTRMWithoutItemIdentity(t *testing.T) {
	var original string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"Items": []any{map[string]any{
				"Path": original,
				"MediaSources": []any{map[string]any{
					"Path": original, "SupportsDirectPlay": false,
					"DirectStreamUrl": "Videos/11/stream?Static=true",
					"TranscodingUrl":  "Videos/11/master.m3u8",
				}},
			}},
		})
	}))
	defer upstream.Close()
	original = upstream.URL + "/api/v1/cloudStorage/strm302?path=%2F115%2Fa%20b.mkv&apiKey=FIXTURE-GRANT&pickCode=p%2Bc"
	target, _ := url.Parse(upstream.URL)
	proxy := NewProxyWithManaged(target, func(_ context.Context, raw string) bool { return raw == original })
	r := httptest.NewRequest("GET", "http://different-gateway/emby/Items", nil)
	r = r.WithContext(context.WithValue(r.Context(), directPlayOnlyKey{}, true))
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("metadata failed", w.Code)
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	item := result["Items"].([]any)[0].(map[string]any)
	source := item["MediaSources"].([]any)[0].(map[string]any)
	if item["Path"] != original || source["Path"] != original || source["SupportsDirectPlay"] != false {
		t.Fatal("same-host STRM or real device capability changed")
	}
	for _, key := range []string{"DirectStreamUrl", "TranscodingUrl", "RequiredHttpHeaders"} {
		if _, ok := source[key]; ok {
			t.Fatal("surrogate/transcoding field survived", key)
		}
	}
}
