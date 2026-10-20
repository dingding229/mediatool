package httpapi

import (
	"bytes"
	"encoding/json"
	v2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/platform/logging"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/dingding229/mediatool-plugin/sdk/contracts"
)

func TestPluginResponseWhitelist(t *testing.T) {
	for _, v := range []contracts.APIResponse{{Status: 302}, {Status: 200, Headers: map[string]string{"Set-Cookie": "session=x"}}, {Status: 200, Headers: map[string]string{"Location": "https://example.invalid"}}, {Status: 200, Headers: map[string]string{"Access-Control-Allow-Origin": "*"}}, {Status: 200, Headers: map[string]string{"Content-Type": "text/html"}}, {Status: 200, Headers: map[string]string{"Content-Type": "application/json\r\nSet-Cookie:x"}}, {Status: 204, Body: []byte("wrong")}, {Status: 200, Body: make([]byte, 65537)}} {
		if validatePluginResponse(v) == nil {
			t.Fatal("unsafe response accepted")
		}
	}
	if validatePluginResponse(contracts.APIResponse{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, Body: []byte(`{}`)}) != nil {
		t.Fatal("safe response rejected")
	}
}
func TestExtensionManagementAndGatewayAreAbsentFromMediaPort(t *testing.T) {
	h, e := NativeMediaHandler(cfg(), Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/api/v1/plugins/workflows", "/api/v1/plugins/auth/bindings", "/api/v1/plugins/auth/begin", "/api/v1/plugins/ui-surfaces", "/api/v1/plugins/11111111-0000-4000-8000-000000000062/ui/form", "/api/v1/plugin/11111111-0000-4000-8000-000000000062/public", "/api/v1/plugins/11111111-0000-4000-8000-000000000062/extensions", "/api/v1/plugins/11111111-0000-4000-8000-000000000062/api-keys"} {
		for _, method := range []string{"GET", "POST"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != 404 {
				t.Fatal(path, w.Code)
			}
		}
	}
}
func TestPluginExtensionSharedContract(t *testing.T) {
	raw, e := os.ReadFile("testdata/contracts/plugin-v2-extensions.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixture struct {
		Snapshot v2.Snapshot `json:"snapshot"`
		History  []v2.Run    `json:"history"`
	}
	if e = json.Unmarshal(raw, &fixture); e != nil {
		t.Fatal(e)
	}
	out, e := json.Marshal(fixture)
	if e != nil {
		t.Fatal(e)
	}
	var expected, actual map[string]any
	if e = json.Unmarshal(raw, &expected); e != nil {
		t.Fatal(e)
	}
	delete(expected, "request")
	if e = json.Unmarshal(out, &actual); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatal("Go/Vue extension contract drift")
	}
}

func TestPrivatePluginLogPreservesOwnerApprovedOriginalURL(t *testing.T) {
	var output bytes.Buffer
	logger, e := logging.NewRawControlled(&output, "info", nil)
	if e != nil {
		t.Fatal(e)
	}
	handler := New(cfg(), Dependencies{Logger: logger})
	w := httptest.NewRecorder()
	raw := "/api/v1/plugin/11111111-0000-4000-8000-000000000062/public?token=private-sentinel-token%2Boriginal&x=allowed&api_key=private-key&x=second"
	handler.ServeHTTP(w, httptest.NewRequest("GET", raw, nil))
	var entry map[string]any
	if e = json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry); e != nil {
		t.Fatal(e)
	}
	if entry["url"] != "http://example.com"+raw || strings.Contains(output.String(), "redacted") {
		t.Fatal("private request URL was altered", entry["url"])
	}
}
