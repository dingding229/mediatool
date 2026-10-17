package embyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/nativelibrary"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

type nativeSettingsFunc func(context.Context) (nativelibrary.ServerSettings, error)

func (f nativeSettingsFunc) Get(ctx context.Context) (nativelibrary.ServerSettings, error) {
	return f(ctx)
}
func settingsFixture() nativelibrary.ServerSettings {
	return nativelibrary.ServerSettings{ServerID: strings.Repeat("a", 32), ServerName: "Synthetic Native Server", Version: "4.8.11.1", Enabled: true, PublicURL: "https://native.fixture.invalid", LibraryGranularity: nativelibrary.GranularityRule, Revision: 1}
}
func concreteEndpoint(pattern string) string {
	path := strings.NewReplacer("{ext}", "mkv", "{format}", "vtt").Replace(pattern)
	path = strings.ReplaceAll(path, "{type}", "Primary")
	path = strings.ReplaceAll(path, "{index}", "0")
	path = regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(path, "100001")
	return strings.ReplaceAll(path, "*", "Countries")
}
func nativeServer(t *testing.T, read nativeSettingsFunc, origin string, log io.Writer) *Server {
	t.Helper()
	if log == nil {
		log = io.Discard
	}
	server, err := NewServer(ServerOptions{Settings: read, PublicURL: origin, Logger: slog.New(slog.NewTextHandler(log, nil))})
	if err != nil {
		t.Fatal(err)
	}
	return server
}
func TestNativeServerRegistersAllRoutesAndFailsClosed(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		value := settingsFixture()
		value.Enabled = enabled
		server := nativeServer(t, func(context.Context) (nativelibrary.ServerSettings, error) { return value, nil }, "", nil)
		for _, endpoint := range nativeEndpoints() {
			for _, prefix := range []string{"", "/emby", "/mediabrowser"} {
				methods := []string{endpoint.method}
				if endpoint.method == "GET" {
					methods = append(methods, "HEAD")
				}
				for _, method := range methods {
					path := prefix + concreteEndpoint(endpoint.pattern)
					r := httptest.NewRequest(method, path+"?api_key=forged-token", nil)
					r.Header.Set("User-Agent", "SyntheticPlayer/1")
					r.AddCookie(&http.Cookie{Name: "media_tool_session", Value: "forged-admin"})
					w := httptest.NewRecorder()
					server.ServeHTTP(w, r)
					want := 401
					if !enabled {
						want = 503
					}
					if endpoint.public && (endpoint.kind == publicInfoResponse || endpoint.kind == pingResponse) {
						want = 200
					} else if endpoint.public && enabled {
						want = 501
						if endpoint.kind == publicUsersResponse || endpoint.kind == endpointResponse || endpoint.kind == brandingResponse || endpoint.kind == brandingCSSResponse {
							want = 200
						}
						if endpoint.kind == noContentResponse {
							want = 204
						}
						if endpoint.kind == imageResponse {
							want = 404
						}
						if endpoint.kind == authenticateResponse {
							want = 400
						}
					}
					if w.Code != want {
						t.Fatalf("enabled=%v %s %s: want %d got %d %q", enabled, method, path, want, w.Code, w.Body.String())
					}
					if method == "HEAD" {
						if w.Body.Len() != 0 {
							t.Fatal("HEAD body", path)
						}
					} else if w.Code == 204 {
						if w.Body.Len() != 0 {
							t.Fatal("204 body")
						}
					} else if endpoint.kind != brandingCSSResponse && !json.Valid(w.Body.Bytes()) {
						t.Fatal("invalid JSON", path, w.Body.String())
					}
				}
			}
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, httptest.NewRequest("OPTIONS", "/Items/100001/PlaybackInfo", nil))
		want := 204
		if !enabled {
			want = 503
		}
		if w.Code != want {
			t.Fatal("OPTIONS bypassed settings gate", w.Code)
		}
	}
}
func TestNativeSystemInfoRealSettingsOriginPrecedenceAndPing(t *testing.T) {
	value := settingsFixture()
	var log bytes.Buffer
	read := nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return value, nil })
	server := nativeServer(t, read, "https://env.fixture.invalid", &log)
	get := func(server *Server, method, path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		server.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}
	w := get(server, "GET", "/EMBY//sYsTeM/Info/Public")
	var info dto.PublicSystemInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Id != value.ServerID || info.ServerName != value.ServerName || info.Version != value.Version || info.LocalAddress != value.PublicURL {
		t.Fatal("not actual settings", info)
	}
	value.PublicURL = ""
	w = get(server, "GET", "/System/Info/Public")
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil || info.LocalAddress != "https://env.fixture.invalid" {
		t.Fatal(info, err)
	}
	fallback := nativeServer(t, read, "", &log)
	for i := 0; i < 2; i++ {
		w = get(fallback, "GET", "http://host.fixture.invalid:25018/System/Info/Public")
	}
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil || info.LocalAddress != "http://host.fixture.invalid:25018" {
		t.Fatal(info, err)
	}
	if strings.Count(log.String(), "public address is not configured") != 1 {
		t.Fatal("missing/bursty warning", log.String())
	}
	for _, method := range []string{"GET", "POST"} {
		w = get(server, method, "/System/Ping")
		var name string
		if err := json.Unmarshal(w.Body.Bytes(), &name); err != nil || name != value.ServerName {
			t.Fatal(name, err)
		}
	}
	value.PublicURL = "https://private:secret@native.fixture.invalid"
	if w := get(server, "GET", "/System/Info/Public"); w.Code != 503 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestNativeServerErrorsEmptyShapesAndConstruction(t *testing.T) {
	server := nativeServer(t, func(context.Context) (nativelibrary.ServerSettings, error) {
		return nativelibrary.ServerSettings{}, errors.New("private database details")
	}, "", nil)
	for _, method := range []string{"GET", "HEAD"} {
		w := httptest.NewRecorder()
		server.ServeHTTP(w, httptest.NewRequest(method, "/System/Info/Public", nil))
		if w.Code != 503 || strings.Contains(w.Body.String(), "private") || (method == "HEAD" && w.Body.Len() != 0) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, tc := range []struct {
		kind   responseKind
		status int
	}{{listResponse, 200}, {singleResponse, 404}, {writeResponse, 501}} {
		w := httptest.NewRecorder()
		server.empty(w, tc.kind)
		if w.Code != tc.status || !json.Valid(w.Body.Bytes()) {
			t.Fatal(w.Code, w.Body.String())
		}
		if tc.kind == listResponse && strings.TrimSpace(w.Body.String()) != `{"Items":[],"TotalRecordCount":0}` {
			t.Fatal("invalid empty result", w.Body.String())
		}
	}
	if _, err := NewServer(ServerOptions{}); err == nil {
		t.Fatal("nil settings accepted")
	}
	if _, err := NewServer(ServerOptions{Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return settingsFixture(), nil }), PublicURL: "https://example.invalid/path"}); err == nil {
		t.Fatal("invalid configured origin")
	}
}
func TestNativeRouterStreamSuffixesWildcardsAndInvalidPatterns(t *testing.T) {
	router, err := NewRouter([]Route{{"GET", "/Videos/{id}/stream.{ext}", func(w http.ResponseWriter, r *http.Request) {
		mustWrite(t, w, r.PathValue("id")+":"+r.PathValue("ext"))
	}}, {"GET", "/Localization/*", func(w http.ResponseWriter, r *http.Request) { mustWrite(t, w, r.PathValue("wildcard")) }}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, body string }{{"/EMBY//ViDeOs/100001/STREAM.MKV", "100001:MKV"}, {"/mediabrowser/Localization/parts/countries", "parts/countries"}} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != 200 || w.Body.String() != tc.body {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, pattern := range []string{"/*/suffix", "/x{bad}", "/stream.{x}{y}", "/{wildcard}/*", "/suffix.*tail", "/Videos/{id}/stream.{id}"} {
		if _, err := NewRouter([]Route{{"GET", pattern, func(http.ResponseWriter, *http.Request) {}}}, Options{}); err == nil {
			t.Fatal("unsafe route pattern", pattern)
		}
	}
}

func TestNativeEndpointsMatchDocumentedMediaTable(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/NATIVE_EMBY_SERVER.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Split(strings.Split(string(raw), "### 媒体端口端点")[1], "### 管理端口端点")[0]
	rows := regexp.MustCompile("(?m)^\\| (GET|POST|PUT|DELETE) \\| `([^`]+)` \\|").FindAllStringSubmatch(text, -1)
	documented := map[string]bool{}
	for _, row := range rows {
		key := row[1] + " " + row[2]
		if documented[key] {
			t.Fatal("duplicate endpoint", key)
		}
		documented[key] = true
	}
	if len(documented) != len(nativeEndpoints()) {
		t.Fatalf("docs=%d registry=%d", len(documented), len(nativeEndpoints()))
	}
	for _, e := range nativeEndpoints() {
		if !documented[e.method+" "+e.pattern] {
			t.Fatal("undocumented endpoint", e.method, e.pattern)
		}
	}
}
func TestNativePublicInfoMatchesContract(t *testing.T) {
	fixture, err := os.ReadFile("testdata/contracts/native-public-info.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected dto.PublicSystemInfo
	decoder := json.NewDecoder(bytes.NewReader(fixture))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&expected); err != nil {
		t.Fatal(err)
	}
	s := nativeServer(t, func(context.Context) (nativelibrary.ServerSettings, error) { return settingsFixture(), nil }, "", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/System/Info/Public", nil))
	var actual dto.PublicSystemInfo
	if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("public info contract mismatch: %+v", actual)
	}
}
