package playback

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"media-tool/internal/modules/storage"
)

type resolverFunc func(context.Context, storage.FileRef) (storage.DownloadLink, error)

func (f resolverFunc) Resolve(ctx context.Context, r storage.FileRef) (storage.DownloadLink, error) {
	return f(ctx, r)
}
func publicLookup(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}
func testPolicy() Policy { return Policy{Lookup: publicLookup} }
func TestPolicyRejectsPrivateMixedDNSAndUnlistedHosts(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "192.168.1.1", "10.0.0.1", "169.254.169.254", "100.100.100.200", "198.18.0.1", "0.0.0.0"} {
		p := Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) { return []net.IPAddr{{IP: net.ParseIP(ip)}}, nil }}
		if p.Check(context.Background(), "https://cdn.example.com/video") == nil {
			t.Fatal("private DNS accepted", ip)
		}
	}
	p := Policy{Lookup: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}}
	if p.Check(context.Background(), "https://cdn.example.com/video") == nil {
		t.Fatal("mixed answer accepted")
	}
	p = testPolicy()
	p.AllowedHosts = []string{"*.115cdn.example.com"}
	if p.Check(context.Background(), "https://node.115cdn.example.com/video") != nil {
		t.Fatal("allowlist rejected")
	}
	if p.Check(context.Background(), "https://115cdn.example.com/video") == nil || p.Check(context.Background(), "https://node.115cdn.example.com.evil.test/video") == nil {
		t.Fatal("allowlist suffix confusion")
	}
}
func TestRedirectAndInspectionDoNotExposeTemporaryURLs(t *testing.T) {
	engine := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: "https://cdn.example.com/video?secret=token", ExpiresAt: time.Now().Add(time.Minute)}, nil
	}), Options{Policy: testPolicy()})
	inspection, err := engine.Inspect(context.Background(), storage.FileRef{}, "")
	if err != nil || inspection.Mode != "redirect" || inspection.MediaBytesChecked {
		t.Fatal(err, inspection)
	}
	for _, method := range []string{"GET", "HEAD"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, "https://app.example.com/media", nil)
		r.Header.Set("Range", "bytes=10-19")
		engine.Serve(w, r, storage.FileRef{})
		if w.Code != 302 || w.Header().Get("Location") == "" {
			t.Fatal(w.Code, w.Body.String())
		}
		if method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD body")
		}
	}
}
func proxyFixture(t *testing.T, handler http.HandlerFunc) (*http.Client, Policy) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	// Test-only transport routes the public test name to our local TLS fixture.
	// Production transports never use this bypass or InsecureSkipVerify.
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}}
	return client, testPolicy()
}
func TestControlledProxyRangeHEADHeadersAndCredentialIsolation(t *testing.T) {
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "provider-agent" || r.Header.Get("Referer") != "https://115.example.test/" || r.Header.Get("Cookie") != "provider-cookie" {
			t.Error("provider headers missing")
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Emby-Token") != "" {
			t.Error("client credentials leaked")
		}
		if r.Header.Get("Range") != "bytes=10-19" {
			t.Error("Range lost")
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 10-19/100")
		w.Header().Set("Content-Length", "10")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Set-Cookie", "secret-cookie")
		w.WriteHeader(206)
		if r.Method != "HEAD" {
			io.WriteString(w, "0123456789")
		}
	})
	engine := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: "https://cdn.example.com/video", UserAgent: "provider-agent", Headers: map[string]string{"Referer": "https://115.example.test/", "Cookie": "provider-cookie"}}, nil
	}), Options{Policy: p, ProxyFallback: true, Client: client})
	for _, method := range []string{"GET", "HEAD"} {
		r := httptest.NewRequest(method, "https://app.example.com/media", nil)
		r.Header.Set("User-Agent", "client-agent")
		r.Header.Set("Range", "bytes=10-19")
		r.Header.Set("Cookie", "media_tool_session=admin-secret")
		r.Header.Set("Authorization", "Bearer admin-secret")
		r.Header.Set("X-Emby-Token", "emby-secret")
		w := httptest.NewRecorder()
		engine.Serve(w, r, storage.FileRef{})
		if w.Code != 206 || w.Header().Get("Content-Range") != "bytes 10-19/100" || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Location") != "" {
			t.Fatal(w.Code, w.Header())
		}
		if method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD returned body")
		}
		if method == "GET" && w.Body.String() != "0123456789" {
			t.Fatal(w.Body.String())
		}
	}
}
func TestProxyRefreshes403OnceAndRejectsPrivateRedirect(t *testing.T) {
	var resolveCalls, mediaCalls atomic.Int32
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if mediaCalls.Add(1) == 1 {
			w.WriteHeader(403)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		io.WriteString(w, "new-media")
	})
	resolver := resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		resolveCalls.Add(1)
		return storage.DownloadLink{URL: "https://cdn.example.com/video", UserAgent: "provider-agent"}, nil
	})
	engine := New(resolver, Options{Policy: p, ProxyFallback: true, Client: client})
	w := httptest.NewRecorder()
	engine.Serve(w, httptest.NewRequest("GET", "https://app.example.com/media", nil), storage.FileRef{})
	if w.Code != 200 || w.Body.String() != "new-media" || resolveCalls.Load() != 2 {
		t.Fatal(w.Code, w.Body.String(), resolveCalls.Load())
	}
	client, p = proxyFixture(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "https://127.0.0.1/admin", 302) })
	engine = New(resolver, Options{Policy: p, ForceProxy: true, Client: client})
	w = httptest.NewRecorder()
	engine.Serve(w, httptest.NewRequest("GET", "https://app.example.com/media", nil), storage.FileRef{})
	if w.Code != 502 {
		t.Fatal("private redirect followed", w.Code)
	}
}
func TestExpiredUnsafeAndProviderHeadersFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		link storage.DownloadLink
		code int
	}{
		{"expired", storage.DownloadLink{URL: "https://cdn.example.com/video", ExpiresAt: time.Now().Add(-time.Second)}, 502},
		{"private", storage.DownloadLink{URL: "https://127.0.0.1/video"}, 502},
		{"header-without-fallback", storage.DownloadLink{URL: "https://cdn.example.com/video", UserAgent: "required-agent"}, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) { return tc.link, nil }), Options{Policy: testPolicy()})
			w := httptest.NewRecorder()
			engine.Serve(w, httptest.NewRequest("GET", "https://app.example.com/media", nil), storage.FileRef{})
			if w.Code != tc.code {
				t.Fatal(w.Code)
			}
		})
	}
	engine := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: "https://cdn.example.com/video", Headers: map[string]string{"Host": "evil"}}, nil
	}), Options{Policy: testPolicy(), ProxyFallback: true})
	w := httptest.NewRecorder()
	engine.Serve(w, httptest.NewRequest("GET", "https://app.example.com/media", nil), storage.FileRef{})
	if w.Code != 502 {
		t.Fatal("provider Host injection accepted")
	}
}
func TestCancellationStopsMediaRequest(t *testing.T) {
	done := make(chan struct{})
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done(); close(done) })
	engine := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: "https://cdn.example.com/video", UserAgent: "provider-agent"}, nil
	}), Options{Policy: p, ProxyFallback: true, Client: client})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	r := httptest.NewRequest("GET", "https://app.example.com/media", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	engine.Serve(w, r, storage.FileRef{})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("media request outlived cancellation")
	}
	if strings.Contains(w.Body.String(), "provider-agent") {
		t.Fatal("sensitive diagnostic")
	}
}

func TestProxyRejectsHTML(t *testing.T) {
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<script>SECRET</script>")
	})
	engine := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: "https://cdn.example.com/v", UserAgent: "provider-agent"}, nil
	}), Options{Policy: p, ProxyFallback: true, Client: client})
	w := httptest.NewRecorder()
	engine.Serve(w, httptest.NewRequest("GET", "https://app.example.com/media", nil), storage.FileRef{})
	if w.Code != 502 || strings.Contains(w.Body.String(), "<script>") {
		t.Fatal("proxied executable content")
	}
}

func TestPolicyRejectsConfiguredGatewayRecursion(t *testing.T) {
	p := testPolicy()
	p.BlockedHosts = []string{"media.example.com"}
	if p.Check(context.Background(), "https://media.example.com/api/v1/cloudStorage/strm302") == nil {
		t.Fatal("self-directed playback recursion accepted")
	}
}

func TestNativeClientHeadersNeverExposeProviderSecrets(t *testing.T) {
	for _, tc := range []struct {
		force   bool
		headers map[string]string
		url     string
		want    bool
	}{{false, nil, "https://cdn.example.com/video", true}, {false, map[string]string{"Cookie": "secret"}, "https://cdn.example.com/video", false}, {true, nil, "https://cdn.example.com/video", false}, {false, nil, "http://cdn.example.com/video", false}, {false, nil, "https://127.0.0.1/video", false}} {
		e := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
			return storage.DownloadLink{URL: tc.url, UserAgent: "provider-required-agent", Headers: tc.headers}, nil
		}), Options{Policy: testPolicy(), ForceProxy: tc.force})
		h := e.NativeClientHeaders(context.Background(), storage.FileRef{})
		if (len(h) > 0) != tc.want || len(h) > 1 || h["Cookie"] != "" {
			t.Fatal("provider headers exposed", h)
		}
	}
}

func TestInspectionProvidesNonSecretActionableUAReason(t *testing.T) {
	e := New(resolverFunc(func(_ context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
		if ref.UserAgent != "SenPlayer/6.2.1" {
			t.Error("inspection did not forward caller UA to resolver")
		}
		return storage.DownloadLink{URL: "https://cdn.example.com/video?FILE-SECRET=hidden", UserAgent: "CloudDrive2/1.1.2 (Linux-X64)"}, nil
	}), Options{Policy: testPolicy(), ProxyFallback: true})
	result, err := e.Inspect(context.Background(), storage.FileRef{}, "SenPlayer/6.2.1")
	if err != nil || result.Mode != "proxy" || result.DeliveryReason != "provider_user_agent_required" || result.RequiredUserAgent != "CloudDrive2/1.1.2 (Linux-X64)" || result.MediaBytesChecked {
		t.Fatal("unhelpful diagnostic", err, result)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "FILE-SECRET") || strings.Contains(string(raw), "https://") {
		t.Fatal("capability leaked")
	}
}

func TestOfficialDirectOnlyChecksCallerUAAndNeverSilentlyProxies(t *testing.T) {
	var calls atomic.Int32
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.UserAgent() != "SenPlayer/6.2.1" || r.Header.Get("Range") != "bytes=0-1023" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Emby-Token") != "" {
			t.Error("unsafe CDN verification request")
		}
		w.Header().Set("Content-Type", "video/x-matroska")
		w.Header().Set("Content-Range", "bytes 0-15/16")
		w.WriteHeader(206)
		io.WriteString(w, "0123456789abcdef")
	})
	source := resolverFunc(func(_ context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: "https://cdn.example.com/movie?SIGNATURE=private", UserAgent: ref.UserAgent, DirectOnly: true, CallerUserAgent: true}, nil
	})
	engine := New(source, Options{Policy: p, ProxyFallback: true, Client: client})
	r := httptest.NewRequest("GET", "http://gateway/media", nil)
	r.Header.Set("User-Agent", "SenPlayer/6.2.1")
	r.Header.Set("X-Emby-Token", "PRIVATE")
	w := httptest.NewRecorder()
	engine.Serve(w, r, storage.FileRef{})
	if w.Code != 302 || w.Header().Get("X-Media-Direct-Verified") != "true" || calls.Load() != 1 {
		t.Fatal("official flow did not redirect", w.Code)
	}
	forced := New(source, Options{Policy: p, ProxyFallback: true, ForceProxy: true, Client: client})
	w = httptest.NewRecorder()
	forced.Serve(w, r, storage.FileRef{})
	if w.Code != 409 || calls.Load() != 1 || w.Header().Get("X-Media-Delivery") == "proxy" {
		t.Fatal("official source silently proxied")
	}
}
func TestOfficialUA403ReturnsExplicitFailureNot302OrRelay(t *testing.T) {
	client, p := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		io.WriteString(w, "private provider error")
	})
	e := New(resolverFunc(func(context.Context, storage.FileRef) (storage.DownloadLink, error) {
		return storage.DownloadLink{URL: "https://cdn.example.com/movie", DirectOnly: true}, nil
	}), Options{Policy: p, Client: client, ProxyFallback: true})
	r := httptest.NewRequest("GET", "http://gateway/media", nil)
	r.Header.Set("User-Agent", "SenPlayer")
	w := httptest.NewRecorder()
	e.Serve(w, r, storage.FileRef{})
	if w.Code != 502 || w.Header().Get("Location") != "" || strings.Contains(w.Body.String(), "private provider error") || w.Header().Get("X-Media-Delivery") == "proxy" {
		t.Fatal("failed official link falsely delivered")
	}
}
