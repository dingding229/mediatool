package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"media-tool/internal/modules/nativeplayback"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

type sessionContract struct {
	Request struct {
		StartIndex int `json:"startIndex"`
		Limit      int `json:"limit"`
	} `json:"request"`
	Response contractResponse[nativeplayback.ActiveSessions] `json:"response"`
}
type fakeNativeSessions struct {
	t    *testing.T
	page nativeplayback.ActiveSessions
}

func (f fakeNativeSessions) List(_ context.Context, start, limit int) (nativeplayback.ActiveSessions, error) {
	if start != 0 || limit != 100 {
		f.t.Fatal(start, limit)
	}
	return f.page, nil
}
func TestNativeSessionsContractAndPortBoundary(t *testing.T) {
	raw, e := os.ReadFile("testdata/contracts/native-sessions.json")
	if e != nil {
		t.Fatal(e)
	}
	var c sessionContract
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		t.Fatal(e)
	}
	a := &API{Deps: Dependencies{NativeSessions: fakeNativeSessions{t, c.Response.Data}}}
	w := httptest.NewRecorder()
	a.nativeSessions(w, httptest.NewRequest("GET", "/?startIndex=0&limit=100", nil))
	var got contractResponse[nativeplayback.ActiveSessions]
	if e = json.Unmarshal(w.Body.Bytes(), &got); e != nil || w.Code != 200 || !reflect.DeepEqual(got, c.Response) {
		t.Fatal(w.Code, e, w.Body.String())
	}
	for _, path := range []string{"/?limit=abc", "/?limit=2&limit=3", "/?limit=201", "/?startIndex=-1", "/?limit=0"} {
		w = httptest.NewRecorder()
		a.nativeSessions(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	handler := New(cfg(), Dependencies{})
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/media-server/native/sessions", nil))
	if w.Code != 401 {
		t.Fatal("admin auth", w.Code)
	}
	member, e := NativeMediaHandler(cfg(), Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	w = httptest.NewRecorder()
	member.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/media-server/native/sessions", nil))
	if w.Code != 404 {
		t.Fatal("member port", w.Code)
	}
}
