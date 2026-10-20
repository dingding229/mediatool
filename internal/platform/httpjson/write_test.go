package httpjson

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type broken struct{ h http.Header }

func (b *broken) Header() http.Header       { return b.h }
func (b *broken) WriteHeader(int)           {}
func (b *broken) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func TestWritePreservesCallerProtocolAndReportsFailure(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Cache-Control", "no-store")
	if e := Write(w, 409, map[string]any{"ResponseStatus": map[string]string{"ErrorCode": "Conflict"}}); e != nil {
		t.Fatal(e)
	}
	if w.Code != 409 || w.Header().Get("Cache-Control") != "no-store" || w.Body.String() != "{\"ResponseStatus\":{\"ErrorCode\":\"Conflict\"}}\n" {
		t.Fatal(w.Body.String())
	}
	if e := Write(&broken{h: http.Header{}}, 200, map[string]bool{"ok": true}); e == nil {
		t.Fatal("write error swallowed")
	}
}
