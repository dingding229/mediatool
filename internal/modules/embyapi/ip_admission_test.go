package embyapi

import (
	"context"
	"media-tool/internal/modules/nativelibrary"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPGateCoversEveryNativeEndpointAndAliasBeforeAuthOrProvider(t *testing.T) {
	s, e := NewServer(ServerOptions{Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) {
		t.Fatal("settings/provider ran after IP rejection")
		return settingsFixture(), nil
	}), IPAdmission: func(*http.Request) (bool, error) { return false, nil }})
	if e != nil {
		t.Fatal(e)
	}
	for _, ep := range nativeEndpoints() {
		for _, prefix := range []string{"", "/emby", "/mediabrowser"} {
			methods := []string{ep.method}
			if ep.method == "GET" {
				methods = append(methods, "HEAD")
			}
			for _, method := range methods {
				t.Run(method+prefix+ep.pattern, func(t *testing.T) {
					r := httptest.NewRequest(method, prefix+concreteEndpoint(ep.pattern), nil)
					w := httptest.NewRecorder()
					s.ServeHTTP(w, r)
					if w.Code != 403 {
						t.Fatal(w.Code, w.Body.String())
					}
				})
			}
		}
	}
	for _, path := range []string{"/api/v1/health", "/missing"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatal("unknown management path leaked", w.Code)
		}
	}
}

func TestIPAdmissionPreservesNativeOverloadAndBodyGuards(t *testing.T) {
	calls := 0
	s, e := NewServer(ServerOptions{Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return settingsFixture(), nil }), IPAdmission: func(*http.Request) (bool, error) { calls++; return false, nil }})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < cap(s.requests); i++ {
		s.requests <- struct{}{}
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/System/Info/Public", nil))
	if w.Code != 503 || calls != 0 {
		t.Fatal("IP check bypassed overload guard", w.Code, calls)
	}
	for len(s.requests) > 0 {
		<-s.requests
	}
	r := httptest.NewRequest("POST", "/Users/AuthenticateByName", nil)
	r.ContentLength = 2 << 20
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 413 || calls != 0 {
		t.Fatal("IP check bypassed body guard", w.Code, calls)
	}
}
