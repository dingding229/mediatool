package httpapi

import (
	"context"
	"encoding/json"
	v2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakePluginInstances struct{ calls int }

func (f *fakePluginInstances) List(context.Context, int, int) ([]v2.Instance, error) {
	return []v2.Instance{}, nil
}
func (f *fakePluginInstances) Apply(context.Context, string, v2.Request) (v2.Instance, error) {
	f.calls++
	return v2.Instance{}, nil
}
func TestPluginV2AdminCSRFMediaPortAndRetiredBoundary(t *testing.T) {
	f := &fakePluginInstances{}
	cfg := cfg()
	h := New(cfg, Dependencies{PluginInstances: f, AuthSessions: testSessionReader{}})
	for _, path := range []string{"/api/v1/plugins/resources", "/api/v1/plugins/11111111-0000-4000-8000-000000000062", "/api/v1/plugins/catalog", "/api/v1/plugins", "/api/v1/plugins/operations"} {
		method := "GET"
		if strings.HasSuffix(path, "operations") {
			method = "POST"
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(`{}`)))
		if w.Code != 401 {
			t.Fatal(path, w.Code)
		}
	}
	token, e := (auth.Auth{Key: cfg.SessionKey}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	missing := httptest.NewRequest("POST", "/api/v1/plugins/operations", strings.NewReader(`{}`))
	missing.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	wCSRF := httptest.NewRecorder()
	h.ServeHTTP(wCSRF, missing)
	if wCSRF.Code != 403 || f.calls != 0 {
		t.Fatal("CSRF did not reject operation", wCSRF.Code)
	}
	media, e := NativeMediaHandler(cfg, Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	media.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/plugins/catalog", nil))
	if w.Code != 404 {
		t.Fatal("media port exposed plugin API", w.Code)
	}
	a := &API{Config: cfg, Deps: Dependencies{PluginInstances: f}}
	w = httptest.NewRecorder()
	a.pluginV2Catalog(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var decoded any
	if json.Unmarshal(w.Body.Bytes(), &decoded) != nil || strings.Contains(w.Body.String(), "pickCode") {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/plugins/package", nil))
	if w.Code != 404 && w.Code != 405 {
		t.Fatal("upload route still registered", w.Code)
	}
	w = httptest.NewRecorder()
	a.pluginV2List(w, httptest.NewRequest("GET", "/?pageSize=201", nil))
	if w.Code != 400 {
		t.Fatal("unbounded list allowed", w.Code)
	}
}

func TestPluginV2OperationSharedContract(t *testing.T) {
	raw, e := os.ReadFile("testdata/contracts/plugin-v2-instances.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixture struct {
		Request  v2.Request  `json:"request"`
		Response v2.Instance `json:"response"`
	}
	if e = json.Unmarshal(raw, &fixture); e != nil {
		t.Fatal(e)
	}
	f := &fakePluginContract{t: t, request: fixture.Request, response: fixture.Response}
	a := &API{Config: cfg(), Auth: auth.Auth{Key: cfg().SessionKey}, Deps: Dependencies{PluginInstances: f, AuthSessions: testSessionReader{}}}
	body, e := json.Marshal(fixture.Request)
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(string(body)))
	token, e := (auth.Auth{Key: cfg().SessionKey}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	a.pluginV2Operation(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		Data v2.Instance `json:"data"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &result); e != nil || !reflect.DeepEqual(result.Data, fixture.Response) {
		t.Fatal(e, result)
	}
}

type fakePluginContract struct {
	t        *testing.T
	request  v2.Request
	response v2.Instance
}

func (f *fakePluginContract) List(context.Context, int, int) ([]v2.Instance, error) { return nil, nil }
func (f *fakePluginContract) Apply(_ context.Context, actor string, request v2.Request) (v2.Instance, error) {
	f.t.Helper()
	if actor != "unit-fixture" || !reflect.DeepEqual(request, f.request) {
		f.t.Fatal("contract/actor mismatch")
	}
	return f.response, nil
}
