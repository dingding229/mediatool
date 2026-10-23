package httpapi

import (
	"encoding/json"
	"media-tool/internal/platform/buildinfo"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVersionAndUpdatesAreAdministratorOnly(t *testing.T) {
	h := New(cfg(), Dependencies{})
	native, e := NativeMediaHandler(cfg(), Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/system/version"}, {"GET", "/api/v1/system/updates"}, {"PUT", "/api/v1/system/updates/settings"}, {"POST", "/api/v1/system/updates/check"}, {"POST", "/api/v1/system/updates/notice"}} {
		r := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
		r.Header.Set("X-Emby-Token", "not-admin")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(route, w.Code)
		}
		w = httptest.NewRecorder()
		native.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatal("native exposed management update API", route, w.Code)
		}
	}
}
func TestVersionInfoContainsOnlyBuildProvenance(t *testing.T) {
	a := &API{}
	w := httptest.NewRecorder()
	a.versionInfo(w, httptest.NewRequest("GET", "/", nil))
	var v struct {
		Data buildinfo.Info `json:"data"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil || v.Data.Schema != buildinfo.Schema || v.Data.Version != buildinfo.Version {
		t.Fatal(e, w.Body.String())
	}
	for _, key := range []string{"version", "commit", "buildTime", "schema", "pluginPin"} {
		if !strings.Contains(w.Body.String(), `"`+key+`"`) {
			t.Fatal("missing provenance", key)
		}
	}
}
