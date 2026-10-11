package emby

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestPlaybackMetadataHidesSTRMGrantAndPreservesTranscoding(t *testing.T) {
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"MediaSources":[{"Id":"source-1","Path":"https://media.fixture/api/v1/cloudStorage/strm302?apiKey=FILE-SECRET&assetId=asset-id&path=private","SupportsDirectPlay":true,"SupportsTranscoding":true,"TranscodingUrl":%q}]}`, upstream.URL+"/Videos/item-1/master.m3u8?api_key=user-token")
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	r := httptest.NewRequest("POST", "http://gateway.example/emby/Items/item-1/PlaybackInfo", strings.NewReader(`{}`))
	r.Header.Set("X-Emby-Token", "user-token")
	w := httptest.NewRecorder()
	NewProxyWithManaged(target, func(_ context.Context, raw string) bool { return strings.HasPrefix(raw, "https://media.fixture/") }).ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), "FILE-SECRET") {
		t.Fatal(w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	sources := body["MediaSources"].([]any)
	source := sources[0].(map[string]any)
	if source["SupportsTranscoding"] != true || source["SupportsDirectPlay"] != true {
		t.Fatal("changed Emby capability decision")
	}
	if !strings.Contains(source["Path"].(string), "/emby/Videos/item-1/stream") || source["TranscodingUrl"] != "Videos/item-1/master.m3u8?api_key=user-token" {
		t.Fatal(source)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("user metadata cached globally")
	}
}
func TestLargeJSONResponseIsNotTruncated(t *testing.T) {
	body := `{"Items":[],"padding":"` + strings.Repeat("x", (4<<20)+1) + `"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	r := httptest.NewRequest("GET", "http://gateway.example/emby/Items", nil)
	w := httptest.NewRecorder()
	NewProxy(target).ServeHTTP(w, r)
	if w.Body.String() != body {
		t.Fatal("large response truncated")
	}
}

func TestForeignResolverIsNotRewritten(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"MediaSources":[{"Id":"s","Path":"https://foreign.example/api/v1/cloudStorage/strm302?apiKey=FOREIGN"}]}`)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	g := NewGateway(GatewayOptions{Upstream: target})
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("GET", "http://gw/emby/Items/i/PlaybackInfo", nil))
	if !strings.Contains(w.Body.String(), "FOREIGN") || strings.Contains(w.Body.String(), "DirectStreamUrl") {
		t.Fatal(w.Body.String())
	}
}
func TestConfiguredOriginIgnoresSpoofedForwarding(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"MediaSources":[{"Id":"s","Path":"https://owned/api/v1/cloudStorage/strm302?apiKey=secret","RequiredHttpHeaders":{"X-Api-Key":"hidden"}}]}`)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	public, _ := url.Parse("https://gateway.owned.example")
	r := httptest.NewRequest("GET", "http://attacker/emby/Items/item/PlaybackInfo", nil)
	r.Header.Set("X-Forwarded-Host", "attacker.example")
	r.Header.Set("X-Forwarded-Proto", "http")
	w := httptest.NewRecorder()
	NewProxyWithManaged(target, func(context.Context, string) bool { return true }, public).ServeHTTP(w, r)
	if strings.Contains(w.Body.String(), "attacker") || strings.Contains(w.Body.String(), "hidden") || !strings.Contains(w.Body.String(), "https://gateway.owned.example") {
		t.Fatal(w.Body.String())
	}
}
func TestOversizedMetadataFailsClosed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"padding":"`+strings.Repeat("x", 17<<20)+`","Path":"SECRET"}`)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	w := httptest.NewRecorder()
	NewProxy(target).ServeHTTP(w, httptest.NewRequest("GET", "http://gw/emby/Items", nil))
	if w.Code != 502 || strings.Contains(w.Body.String(), "SECRET") {
		t.Fatal("oversized metadata leaked")
	}
}
func TestConcurrentOwnedMetadataDoesNotExposeFileGrant(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"MediaSources":[{"Id":"s","Path":"https://owned/api/v1/cloudStorage/strm302?apiKey=PRIVATE-GRANT","SupportsTranscoding":true}]}`)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	proxy := NewProxyWithManaged(target, func(context.Context, string) bool { return true })
	var wg sync.WaitGroup
	for n := 0; n < 16; n++ {
		wg.Go(func() {
			for i := 0; i < 30; i++ {
				r := httptest.NewRequest("POST", "http://gw/emby/Items/item/PlaybackInfo", strings.NewReader("{}"))
				r.Header.Set("X-Emby-Token", "fixture-user")
				w := httptest.NewRecorder()
				proxy.ServeHTTP(w, r)
				if w.Code != 200 || strings.Contains(w.Body.String(), "PRIVATE-GRANT") || !strings.Contains(w.Body.String(), `"SupportsTranscoding":true`) {
					t.Errorf("metadata load check failed")
				}
			}
		})
	}
	wg.Wait()
}
func TestIPv6GatewayUpstream(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 unavailable on this host")
	}
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"Items":[]}`)
	}))
	upstream.Listener.Close()
	upstream.Listener = listener
	upstream.Start()
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	w := httptest.NewRecorder()
	NewProxy(target).ServeHTTP(w, httptest.NewRequest("GET", "http://gw/emby/Items", nil))
	if w.Code != 200 {
		t.Fatal("IPv6 upstream failed", w.Code)
	}
}

func TestManagedMetadataPreservesRelativeHLSDirectStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"MediaSources":[{"Id":"s","Path":"https://owned/api/v1/cloudStorage/strm302?apiKey=FILE-SECRET","SupportsDirectPlay":false,"SupportsDirectStream":true,"DirectStreamUrl":"/videos/item/master.m3u8?VideoCodec=copy&AudioCodec=aac&api_key=user-token","TranscodingUrl":"videos/item/master.m3u8?VideoCodec=h264&api_key=user-token"}]}`)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	w := httptest.NewRecorder()
	NewProxyWithManaged(target, func(_ context.Context, raw string) bool { return strings.HasPrefix(raw, "https://owned/") }).ServeHTTP(w, httptest.NewRequest("POST", "http://gateway/emby/Items/item/PlaybackInfo", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "FILE-SECRET") {
		t.Fatal("metadata not redacted")
	}
	var info struct {
		Sources []struct {
			DirectStreamURL    string `json:"DirectStreamUrl"`
			TranscodingURL     string `json:"TranscodingUrl"`
			SupportsDirectPlay bool   `json:"SupportsDirectPlay"`
		} `json:"MediaSources"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	source := info.Sources[0]
	if source.DirectStreamURL != "videos/item/master.m3u8?VideoCodec=copy&AudioCodec=aac&api_key=user-token" || source.TranscodingURL != "videos/item/master.m3u8?VideoCodec=h264&api_key=user-token" || source.SupportsDirectPlay {
		t.Fatal("relative HLS contract changed", source)
	}
}

// Real Emby Web joins its server address with DirectStream/Transcoding fields.
// Absolute fields produced /http:/host/... 404, even though manual URL GET worked.
func TestMediaURLContractForEmbyWebGetURL(t *testing.T) {
	target, _ := url.Parse("http://upstream:8096/emby")
	origin := publicOrigin{Scheme: "https", Host: "gateway.example", Base: "/emby"}
	for _, raw := range []string{"videos/11/master.m3u8?AudioCodec=aac", "/videos/11/master.m3u8?AudioCodec=aac", "http://upstream:8096/emby/videos/11/master.m3u8?AudioCodec=aac"} {
		rewritten := rewriteMediaURL(raw, target, origin)
		if rewritten != "videos/11/master.m3u8?AudioCodec=aac" {
			t.Fatal("field is not server-relative", rewritten)
		}
		joined := url.URL{Scheme: "https", Host: "gateway.example", Path: "/emby/" + strings.Split(rewritten, "?")[0]}
		if strings.Contains(joined.Path, "http:") {
			t.Fatal("browser reproduces malformed HLS URL")
		}
	}
	foreign := "https://foreign.example/media"
	if rewriteMediaURL(foreign, target, origin) != foreign {
		t.Fatal("foreign URL modified")
	}
}

func TestNativePlaybackHeadersOnlyOnPlaybackInfo(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"MediaSources":[{"Id":"s","Path":"https://owned/api/v1/cloudStorage/strm302?apiKey=SECRET","RequiredHttpHeaders":{"Cookie":"PROVIDER-SECRET"}}]}`)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	var calls int
	handler := newProxy(target, func(context.Context, string) bool { return true }, nil, func(context.Context, string) map[string]string {
		calls++
		return map[string]string{"User-Agent": "provider-required-agent"}
	})
	for _, tc := range []struct {
		ua, path string
		want     bool
	}{{"SenPlayer/6.2.1", "/emby/Items/11/PlaybackInfo", true}, {"Mozilla/5.0", "/emby/Items/11/PlaybackInfo", false}, {"SenPlayer/6.2.1", "/emby/Items", false}} {
		r := httptest.NewRequest("GET", "http://gateway"+tc.path, nil)
		r.Header.Set("User-Agent", tc.ua)
		r.Header.Set("X-Emby-Token", "fixture-user-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		hasUA := strings.Contains(w.Body.String(), `"User-Agent":"provider-required-agent"`)
		if tc.want && (!strings.Contains(w.Body.String(), `"IsRemote":true`) || !strings.Contains(w.Body.String(), `"Protocol":"Http"`)) {
			t.Fatal("native cloud source lost remote HTTP semantics")
		}
		if !tc.want && strings.Contains(w.Body.String(), `"IsRemote":true`) {
			t.Fatal("browser/list media semantics changed")
		}
		if hasUA != tc.want || strings.Contains(w.Body.String(), "PROVIDER-SECRET") || strings.Contains(w.Body.String(), "apiKey=SECRET") {
			t.Fatal("unsafe/incompatible media headers", w.Body.String())
		}
	}
	if calls != 1 {
		t.Fatal("native listing triggered provider requests", calls)
	}
}
