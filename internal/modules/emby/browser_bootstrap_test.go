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
	"strconv"
	"strings"
	"testing"
)

func TestBootstrapCacheKeyChangesNeitherVersionNorPermissions(t *testing.T) {
	for _, test := range []struct {
		policy browserAssetPolicy
		body   []byte
	}{
		{browserPolicies[1], []byte(`<html data-appversion="4.10.1.0">` + originalLoaderTag + `</html>`)},
		{browserPolicies[2], []byte(`/* owned loader fixture */` + originalLoaderCache + `;globalThis.dashboardVersion=docElem;`)},
	} {
		policy := test.policy
		digest := sha256.Sum256(test.body)
		policy.digest = hex.EncodeToString(digest[:])
		request := httptest.NewRequest("GET", "http://fixture"+policy.path+"?v=4.10.1.0", nil)
		content := "application/x-javascript"
		if policy.html {
			content = "text/html; charset=UTF-8"
		}
		response := &http.Response{Request: request, StatusCode: 200, Header: http.Header{"Content-Type": {content}, "Etag": {"original"}, "Last-Modified": {"original"}, "Digest": {"original"}, "Content-Digest": {"original"}, "Content-Md5": {"original"}}, Body: io.NopCloser(bytes.NewReader(test.body)), ContentLength: int64(len(test.body))}
		if err := rewriteBrowserAsset(response, policy); err != nil {
			t.Fatal(err)
		}
		updated, _ := io.ReadAll(response.Body)
		if !bytes.Contains(updated, []byte(policy.after)) || !bytes.Contains(updated, []byte("4.10.1.0")) && policy.html {
			t.Fatal("cache key not added, or version changed")
		}
		if bytes.Count(updated, []byte(browserPolicyCacheKey)) != 1 {
			t.Fatal("cache revision missing or duplicated")
		}
		if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Media-Browser-Policy") != policy.label || response.ContentLength != int64(len(updated)) || response.Header.Get("Content-Length") != strconv.Itoa(len(updated)) {
			t.Fatal("modified representation metadata wrong")
		}
		for _, header := range []string{"ETag", "Last-Modified", "Digest", "Content-Digest", "Content-MD5"} {
			if response.Header.Get(header) != "" {
				t.Fatal("old validator/integrity survives")
			}
		}
		for _, raw := range [][]byte{append(append([]byte{}, test.body...), '\n'), updated} {
			unchanged, changed := rewriteHashBoundAsset(raw, policy.digest, policy.before, policy.after)
			if changed || !bytes.Equal(raw, unchanged) {
				t.Fatal("unknown/already changed bootstrap modified")
			}
		}
	}
}

func TestBootstrapUnknownVersionEncodingMimeAndStatusStayUntouched(t *testing.T) {
	for _, test := range []struct {
		path, mime, encoding string
		status               int
	}{
		{browserIndexPath, "text/html", "", 200}, {browserLoaderPath, "application/javascript", "gzip", 200}, {browserIndexPath, "application/json", "", 200}, {browserLoaderPath, "application/javascript", "", 403},
	} {
		raw := []byte("owned unknown release; leave unchanged")
		response := &http.Response{Request: httptest.NewRequest("GET", "http://fixture"+test.path, nil), StatusCode: test.status, Header: http.Header{"Content-Type": {test.mime}, "Content-Encoding": {test.encoding}, "Etag": {"original"}}, Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw))}
		if err := rewriteBrowserStartup(response); err != nil {
			t.Fatal(err)
		}
		actual, _ := io.ReadAll(response.Body)
		if !bytes.Equal(raw, actual) || response.Header.Get("ETag") != "original" || response.Header.Get("X-Media-Browser-Policy") != "" {
			t.Fatal("unknown/ineligible representation changed")
		}
	}
}

func TestBootstrapValidatorsRemovedOnlyForManagedExactAssets(t *testing.T) {
	for _, managedEnabled := range []bool{false, true} {
		for _, path := range []string{browserIndexPath, browserLoaderPath, browserPlayerPath, "/web/app.js", "/System/Info/Public"} {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				cleared := managedEnabled && isBrowserStartupAsset(path)
				if (r.Header.Get("If-None-Match") == "") != cleared || (r.Header.Get("If-Modified-Since") == "") != cleared {
					t.Error("validator scope wrong", path)
				}
				w.Header().Set("Content-Type", "application/javascript")
				io.WriteString(w, "// unknown fixture remains unchanged")
			}))
			target, _ := url.Parse(upstream.URL)
			var managed ManagedPredicate
			if managedEnabled {
				managed = func(context.Context, string) bool { return true }
			}
			request := httptest.NewRequest("GET", "/emby"+path, nil)
			request.Header.Set("If-None-Match", "old")
			request.Header.Set("If-Modified-Since", "old")
			recorder := httptest.NewRecorder()
			NewProxyWithManaged(target, managed).ServeHTTP(recorder, request)
			if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "unknown fixture") {
				t.Fatal("asset forwarding changed")
			}
			upstream.Close()
		}
	}
}
