package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"media-tool/internal/modules/nativelibrary/catalog"
	"net/http/httptest"
	"os"
	"testing"
)

type nativeLibraryReaderFunc func(context.Context, int, int) (catalog.Page, error)

func (f nativeLibraryReaderFunc) List(ctx context.Context, start, limit int) (catalog.Page, error) {
	return f(ctx, start, limit)
}
func TestNativeLibrariesAdminReadContract(t *testing.T) {
	fixture, err := os.ReadFile("testdata/contracts/native-libraries.json")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data catalog.Page `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(fixture))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	expected := envelope.Data
	api := &API{Deps: Dependencies{NativeLibraries: nativeLibraryReaderFunc(func(context.Context, int, int) (catalog.Page, error) { return expected, nil })}}
	w := httptest.NewRecorder()
	api.listNativeLibraries(w, httptest.NewRequest("GET", "/api/v1/media-server/native/libraries?startIndex=0&limit=200", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		Data catalog.Page `json:"data"`
	}
	decoder = json.NewDecoder(w.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	actual := result.Data
	if !bytes.Equal(mustJSON(expected), mustJSON(actual)) {
		t.Fatalf("contract mismatch %s", w.Body.String())
	}
	for _, query := range []string{"?startIndex=-1", "?limit=201", "?limit=no", "?limit=1&limit=2"} {
		w = httptest.NewRecorder()
		api.listNativeLibraries(w, httptest.NewRequest("GET", "/api/v1/media-server/native/libraries"+query, nil))
		if w.Code != 400 {
			t.Fatal(query, w.Code)
		}
	}
}
func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
