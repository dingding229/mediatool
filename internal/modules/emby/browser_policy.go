package emby

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Exact bytes served by Emby 4.10.1.0. Unknown versions stay untouched. We
// apply bounded startup waiting and the HLS first-byte budget, never
// capabilities, license checks, authentication or the library retry policies.
const knownBrowserPlayerSHA256 = "8c61400307b6a13a668731bbc086b5673538f9aaee261e7b8d2bb49403a6e531"
const browserPlayerPath = "/web/modules/htmlvideoplayer/basehtmlplayer.js"
const browserOptions = `var hlsOptions={manifestLoadingTimeOut:2e4,debug:!1,testBandwidth:!1,emeEnabled:!1}`
const coldBrowserOptions = `var hlsOptions=Object.assign({manifestLoadingTimeOut:2e4,debug:!1,testBandwidth:!1,emeEnabled:!1},Hls.DefaultConfig&&Hls.DefaultConfig.fragLoadPolicy?{fragLoadPolicy:{default:Object.assign({},Hls.DefaultConfig.fragLoadPolicy.default,{maxTimeToFirstByteMs:6e4})}}:{})`
const maxBrowserScript = 2 << 20

func isBrowserPlayer(path string) bool { return path == browserPlayerPath }

const browserIndexPath = "/web/index.html"
const browserLoaderPath = "/web/apploader.js"
const knownBrowserIndexSHA256 = "fcbc75842ad94e990db8df8121548f7283a7392ec2bbc5d6f4491fc7bd0fcfa8"
const knownBrowserLoaderSHA256 = "2999230248d27eb187d4dc8db6dae45cdaaee2905c4effb3aceff7916d7ef447"
const browserPolicyCacheKey = "mt_hls_startup=2"
const originalLoaderTag = `<script src="apploader.js?v=4.10.1.0" defer></script>`
const revisedLoaderTag = `<script src="apploader.js?v=4.10.1.0&mt_hls_startup=2" defer></script>`
const originalLoaderCache = `docElem?globalThis.urlCacheParam="v="+docElem:appMode||(globalThis.urlCacheParam="v="+Date.now())`
const revisedLoaderCache = `docElem?globalThis.urlCacheParam="v="+docElem+"&mt_hls_startup=2":appMode||(globalThis.urlCacheParam="v="+Date.now())`

//go:embed browser_startup_guard.js
var boundedStartupGuard string

const browserStartupBinding = `function bindEventsToHlsPlayer(instance,hls,elem,signal,resolve,reject){hls.on`
const browserStartupErrorBranch = `else{if(console.log("HLS Error: Type: "+data.type`

type browserReplacement struct{ before, after string }
type browserAssetPolicy struct {
	path, digest, before, after, label string
	html                               bool
	additional                         []browserReplacement
}

var browserPolicies = []browserAssetPolicy{
	{path: browserPlayerPath, digest: knownBrowserPlayerSHA256, before: browserOptions, after: coldBrowserOptions, label: "hls-startup-retry-90s-v2", additional: []browserReplacement{
		{before: browserStartupBinding, after: `function bindEventsToHlsPlayer(instance,hls,elem,signal,resolve,reject){` + boundedStartupGuard + `hls.on`},
		{before: browserStartupErrorBranch, after: `else{if(mtStartupGuard.shouldWait(data))return;if(console.log("HLS Error: Type: "+data.type`},
	}},
	{path: browserIndexPath, digest: knownBrowserIndexSHA256, before: originalLoaderTag, after: revisedLoaderTag, label: "hls-bootstrap-cache-v2", html: true},
	{path: browserLoaderPath, digest: knownBrowserLoaderSHA256, before: originalLoaderCache, after: revisedLoaderCache, label: "hls-module-cache-v2"},
}

func isBrowserStartupAsset(path string) bool {
	for _, policy := range browserPolicies {
		if path == policy.path {
			return true
		}
	}
	return false
}
func rewriteBrowserStartup(response *http.Response) error {
	if response.Request == nil || response.Request.URL == nil {
		return nil
	}
	for _, policy := range browserPolicies {
		if response.Request.URL.Path == policy.path {
			return rewriteBrowserAsset(response, policy)
		}
	}
	return nil
}
func rewriteBrowserStartupForHash(response *http.Response, expected string) error {
	return rewriteBrowserAsset(response, browserAssetPolicy{path: browserPlayerPath, digest: expected, before: browserOptions, after: coldBrowserOptions, label: "hls-first-byte-60s"})
}
func rewriteBrowserAsset(response *http.Response, policy browserAssetPolicy) error {
	if response.StatusCode != http.StatusOK || response.Request == nil || response.Request.URL == nil || response.Request.URL.Path != policy.path {
		return nil
	}
	content := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if policy.html {
		if content != "text/html" {
			return nil
		}
	} else if content != "application/javascript" && content != "text/javascript" && content != "application/x-javascript" {
		return nil
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return nil
	}
	original := response.Body
	raw, err := io.ReadAll(io.LimitReader(original, maxBrowserScript+1))
	if err != nil {
		return err
	}
	if len(raw) > maxBrowserScript {
		response.Body = joinedReadCloser{Reader: io.MultiReader(bytes.NewReader(raw), original), Closer: original}
		return nil
	}
	deferutil.Close(original, "Read/descriptor teardown only; primary operations handle their own I/O result")
	updated, changed := rewriteBrowserPolicyBytes(raw, policy)
	response.Body = io.NopCloser(bytes.NewReader(updated))
	if !changed {
		return nil
	}
	response.ContentLength = int64(len(updated))
	response.Header.Set("Content-Length", strconv.Itoa(len(updated)))
	response.Header.Set("Cache-Control", "no-store")
	response.Header.Set("X-Media-Browser-Policy", policy.label)
	// These validators and integrity fields describe the original bytes, not the
	// representation served by this gateway. Unknown versions keep all headers.
	for _, header := range []string{"Content-Encoding", "ETag", "Last-Modified", "Content-MD5", "Content-Digest", "Digest"} {
		response.Header.Del(header)
	}
	return nil
}
func rewriteHashBoundBrowser(raw []byte, expected string) ([]byte, bool) {
	return rewriteHashBoundAsset(raw, expected, browserOptions, coldBrowserOptions)
}
func rewriteBrowserPolicyBytes(raw []byte, policy browserAssetPolicy) ([]byte, bool) {
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != policy.digest || bytes.Count(raw, []byte(policy.before)) != 1 {
		return raw, false
	}
	for _, edit := range policy.additional {
		if bytes.Count(raw, []byte(edit.before)) != 1 {
			return raw, false
		}
	}
	out := bytes.Replace(raw, []byte(policy.before), []byte(policy.after), 1)
	for _, edit := range policy.additional {
		out = bytes.Replace(out, []byte(edit.before), []byte(edit.after), 1)
	}
	return out, true
}

func rewriteHashBoundAsset(raw []byte, expected, before, after string) ([]byte, bool) {
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != expected || bytes.Count(raw, []byte(before)) != 1 {
		return raw, false
	}
	return bytes.Replace(raw, []byte(before), []byte(after), 1), true
}

// A valid HLS fragment may await the encoder's first bytes. Metadata/API
// requests retain 30s; only GET/HEAD HLS fragments use a finite 75s header budget
// so the known browser's 60s first-byte budget is not cut short by this proxy.
var hlsFragmentPath = regexp.MustCompile(`(?i)^/(?:emby/)?videos/[A-Za-z0-9_-]{1,128}/hls1/[A-Za-z0-9_-]+/[0-9]+\.(?:ts|m4s|m4a)$`)

type scopedProxyTransport struct{ api, fragment http.RoundTripper }

func (s scopedProxyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && hlsFragmentPath.MatchString(r.URL.Path) {
		return s.fragment.RoundTrip(r)
	}
	return s.api.RoundTrip(r)
}

// Share pools across the per-request Gateway factory; do not create a transport
// (and idle connection/goroutine pool) for every segment request.
var sharedProxyTransport = func() scopedProxyTransport {
	fragment := upstreamTransport.Clone()
	fragment.ResponseHeaderTimeout = 75 * time.Second
	return scopedProxyTransport{api: upstreamTransport, fragment: fragment}
}()

func defaultProxyTransport() http.RoundTripper { return sharedProxyTransport }
