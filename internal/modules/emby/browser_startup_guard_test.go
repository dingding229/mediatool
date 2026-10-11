package emby

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStartupGuardRewritesAllAnchorsAtomicallyForTheVerifiedVersion(t *testing.T) {
	raw := []byte("// owned fixture\n" + browserOptions + ";" + browserStartupBinding + "(events,x);}" + browserStartupErrorBranch + "+x)){}}")
	policy := browserPolicies[0]
	digest := sha256.Sum256(raw)
	policy.digest = hex.EncodeToString(digest[:])
	updated, changed := rewriteBrowserPolicyBytes(raw, policy)
	if !changed || !bytes.Contains(updated, []byte("mtStartupGuard.shouldWait(data)")) || !bytes.Contains(updated, []byte("90000")) || !bytes.Contains(updated, []byte("maxTimeToFirstByteMs:6e4")) {
		t.Fatal("finite startup policy missing")
	}
	for _, forbidden := range []string{"hls.startLoad(", "loadSource(", "SupportsDirectPlay", "MaxNumRetry:"} {
		if bytes.Contains([]byte(boundedStartupGuard), []byte(forbidden)) {
			t.Fatal("guard replaces retry/source/capabilities")
		}
	}
	for _, broken := range [][]byte{bytes.Replace(raw, []byte(browserStartupBinding), []byte("unknown binding"), 1), append(raw, []byte(browserStartupErrorBranch)...), updated} {
		candidate := policy
		hash := sha256.Sum256(broken)
		candidate.digest = hex.EncodeToString(hash[:])
		result, changed := rewriteBrowserPolicyBytes(broken, candidate)
		if changed || !bytes.Equal(result, broken) {
			t.Fatal("partial/ambiguous patch applied")
		}
	}
	request := httptest.NewRequest("GET", "http://fixture"+browserPlayerPath, nil)
	response := &http.Response{Request: request, StatusCode: 200, Header: http.Header{"Content-Type": {"application/x-javascript"}, "Etag": {"original"}}, Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw))}
	if err := rewriteBrowserAsset(response, policy); err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if !bytes.Equal(body, updated) || response.ContentLength != int64(len(updated)) || response.Header.Get("X-Media-Browser-Policy") != "hls-startup-retry-90s-v2" || response.Header.Get("ETag") != "" {
		t.Fatal("response metadata does not match whole guard")
	}
}
