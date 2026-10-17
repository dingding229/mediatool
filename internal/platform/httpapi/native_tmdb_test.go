package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"media-tool/internal/modules/nativetmdb"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

type nativeTMDBRefreshFunc func(context.Context, nativetmdb.Target) (string, error)

func (f nativeTMDBRefreshFunc) Refresh(ctx context.Context, t nativetmdb.Target) (string, error) {
	return f(ctx, t)
}
func TestNativeTMDBRefreshContractAndAuthorization(t *testing.T) {
	raw, err := os.ReadFile("testdata/contracts/native-tmdb-refresh.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Request  nativetmdb.Target `json:"request"`
		Response struct {
			Data nativeTMDBQueued `json:"data"`
		} `json:"response"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	var received nativetmdb.Target
	api := &API{Deps: Dependencies{NativeTMDBRefresh: nativeTMDBRefreshFunc(func(_ context.Context, in nativetmdb.Target) (string, error) {
		received = in
		return fixture.Response.Data.DetailID, nil
	})}}
	input, err := json.Marshal(fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	api.refreshNativeTMDB(w, httptest.NewRequest("POST", "/api/v1/media-server/native/tmdb/refresh", bytes.NewReader(input)))
	if w.Code != 202 || !reflect.DeepEqual(received, fixture.Request) {
		t.Fatal(w.Code, w.Body.String(), received)
	}
	var response struct {
		Data nativeTMDBQueued `json:"data"`
	}
	dec = json.NewDecoder(w.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Data != fixture.Response.Data {
		t.Fatal(response)
	}
	for _, body := range []string{`{"type":"movie","tmdbId":0}`, `{"type":"movie","tmdbId":1,"season":1}`, `{"type":"season","tmdbId":1,"season":-1}`, `{"type":"movie","tmdbId":1,"token":"secret"}`} {
		w = httptest.NewRecorder()
		api.refreshNativeTMDB(w, httptest.NewRequest("POST", "/api/v1/media-server/native/tmdb/refresh", bytes.NewBufferString(body)))
		if w.Code != 400 {
			t.Fatal(body, w.Code)
		}
	}
	w = httptest.NewRecorder()
	New(cfg(), Dependencies{}).ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/media-server/native/tmdb/refresh", bytes.NewReader(input)))
	if w.Code != 401 {
		t.Fatal("unauthed refresh", w.Code)
	}
	member, err := NativeMediaHandler(cfg(), Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	member.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/media-server/native/tmdb/refresh", bytes.NewReader(input)))
	if w.Code != 404 {
		t.Fatal("management endpoint leaked to media port", w.Code)
	}
}
