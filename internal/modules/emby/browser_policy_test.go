package emby

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestFirstByteOptionPatchIsHashBoundAndPreservesLibraryRetryBudget(t *testing.T) {
	original := []byte("// owned test fixture\n" + browserOptions + ",hls=new Hls(hlsOptions);")
	digest := sha256.Sum256(original)
	hash := hex.EncodeToString(digest[:])
	updated, ok := rewriteHashBoundBrowser(original, hash)
	if !ok || !bytes.Contains(updated, []byte("maxTimeToFirstByteMs:6e4")) || !bytes.Contains(updated, []byte("Hls.DefaultConfig.fragLoadPolicy.default")) {
		t.Fatal("bounded cold budget not applied")
	}
	if bytes.Contains(updated, []byte("SupportsDirectPlay")) || bytes.Contains(updated, []byte("timeoutRetry")) || bytes.Contains(updated, []byte("maxLoadTimeMs")) {
		t.Fatal("capability/retry/total budget changed")
	}
	for _, unknown := range [][]byte{append(original, byte('\n')), []byte("unknown release"), updated} {
		result, changed := rewriteHashBoundBrowser(unknown, hash)
		if changed || !bytes.Equal(result, unknown) {
			t.Fatal("unknown/already-modified bundle changed")
		}
	}
	double := []byte(browserOptions + browserOptions)
	d := sha256.Sum256(double)
	if _, changed := rewriteHashBoundBrowser(double, hex.EncodeToString(d[:])); changed {
		t.Fatal("ambiguous bundle patched")
	}
}

func TestUnmatchedBrowserResponseIsByteAndHeaderPreserving(t *testing.T) {
	content := []byte("// other Emby version; leave exactly as-is")
	req := httptest.NewRequest("GET", "http://emby.fixture"+browserPlayerPath, nil)
	response := &http.Response{Request: req, StatusCode: 200, Header: http.Header{"Content-Type": {"application/javascript"}, "Etag": {"original"}}, Body: io.NopCloser(bytes.NewReader(content)), ContentLength: int64(len(content))}
	if err := rewriteBrowserStartup(response); err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if !bytes.Equal(body, content) || response.Header.Get("ETag") != "original" || response.ContentLength != int64(len(content)) {
		t.Fatal("unmatched version altered")
	}
}

func TestHLSFragmentHeaderBudgetDoesNotExtendAPIOrMutatingRequests(t *testing.T) {
	transport := defaultProxyTransport().(scopedProxyTransport)
	if transport.api.(*http.Transport).ResponseHeaderTimeout != 30*time.Second || transport.fragment.(*http.Transport).ResponseHeaderTimeout != 75*time.Second {
		t.Fatal("incorrect scoped budgets")
	}
	target, _ := url.Parse("http://emby.fixture")
	chosen := ""
	router := scopedProxyTransport{
		api:      failingTransport(func(*http.Request) (*http.Response, error) { chosen = "api"; return nil, nil }),
		fragment: failingTransport(func(*http.Request) (*http.Response, error) { chosen = "fragment"; return nil, nil }),
	}
	for _, test := range []struct{ method, path, want string }{{"GET", "/videos/11/hls1/main/170.ts", "fragment"}, {"HEAD", "/emby/videos/11/hls1/main/0.m4s", "fragment"}, {"POST", "/videos/11/hls1/main/0.ts", "api"}, {"GET", "/Items/11/PlaybackInfo", "api"}, {"GET", "/videos/11/master.m3u8", "api"}, {"GET", "/videos/11/original.mkv", "api"}} {
		req := httptest.NewRequest(test.method, target.String()+test.path, nil)
		router.RoundTrip(req)
		if chosen != test.want {
			t.Fatal("wrong budget scope", test)
		}
	}
}

func TestMatchedLegacyJavaScriptMimeUpdatesLengthAndInvalidatesOriginalCache(t *testing.T) {
	original := []byte("// owned fixture\n" + browserOptions)
	digest := sha256.Sum256(original)
	response := &http.Response{Request: httptest.NewRequest("GET", "http://emby.fixture"+browserPlayerPath, nil), StatusCode: 200, Header: http.Header{"Content-Type": {"application/x-javascript"}, "Etag": {"original"}, "Last-Modified": {"old"}}, Body: io.NopCloser(bytes.NewReader(original)), ContentLength: int64(len(original))}
	if err := rewriteBrowserStartupForHash(response, hex.EncodeToString(digest[:])); err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if response.ContentLength != int64(len(body)) || response.Header.Get("ETag") != "" || response.Header.Get("Last-Modified") != "" || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Media-Browser-Policy") != "hls-first-byte-60s" {
		t.Fatal("modified bundle cache/length unsafe")
	}
}

func TestOnlyManagedGatewayDiscardsPlayerConditionalValidators(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if (r.Header.Get("If-None-Match") == "") != enabled || (r.Header.Get("If-Modified-Since") == "") != enabled {
				t.Error("wrong validator handling", enabled)
			}
			w.Header().Set("Content-Type", "application/javascript")
			io.WriteString(w, "// unknown version unchanged")
		}))
		target, _ := url.Parse(upstream.URL)
		var managed ManagedPredicate
		if enabled {
			managed = func(context.Context, string) bool { return true }
		}
		proxy := NewProxyWithManaged(target, managed)
		request := httptest.NewRequest("GET", "/emby"+browserPlayerPath, nil)
		request.Header.Set("If-None-Match", "old")
		request.Header.Set("If-Modified-Since", "old")
		proxy.ServeHTTP(httptest.NewRecorder(), request)
		upstream.Close()
	}
}
