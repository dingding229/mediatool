package embyapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestErrorResponsesMatchGoldenWithoutInternalData(t *testing.T) {
	raw, err := os.ReadFile("testdata/errors.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]json.RawMessage
	if err = json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{400, 401, 403, 404, 405, 409, 500, 501, 503, 418, 0} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			w := httptest.NewRecorder()
			if err := WriteError(w, status); err != nil {
				t.Fatal(err)
			}
			expected := status
			if status == 418 || status == 0 {
				expected = 500
			}
			if w.Code != expected || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatal(w.Code, w.Header())
			}
			var a, b any
			if err = json.Unmarshal(w.Body.Bytes(), &a); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(golden[strconv.Itoa(expected)], &b); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("got=%s expected=%s", w.Body, golden[strconv.Itoa(expected)])
			}
			var strict struct {
				ResponseStatus struct{ ErrorCode, Message string }
			}
			decoder := json.NewDecoder(w.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&strict); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type brokenWriter struct {
	header http.Header
	status int
	err    error
}

func (w *brokenWriter) Header() http.Header       { return w.header }
func (w *brokenWriter) WriteHeader(status int)    { w.status = status }
func (w *brokenWriter) Write([]byte) (int, error) { return 0, w.err }
func TestResponseWriteErrorsAreReturnedAndNeverLoggedWithSecrets(t *testing.T) {
	secretError := errors.New("PRIVATE SQL /cloud/path?api_key=SECRET")
	w := &brokenWriter{header: http.Header{}, err: secretError}
	if err := WriteError(w, 503); !errors.Is(err, secretError) {
		t.Fatal("lost write error", err)
	}
	var log bytes.Buffer
	router, err := NewRouter(nil, Options{Logger: slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	if err != nil {
		t.Fatal(err)
	}
	router.ServeHTTP(w, httptest.NewRequest("GET", "/unknown?api_key=SECRET", nil))
	if w.status != 404 || !strings.Contains(log.String(), "error response interrupted") || strings.Contains(log.String(), "SECRET") || strings.Contains(log.String(), "SQL") {
		t.Fatal("unsafe interrupted response", log.String())
	}
}
func TestErrorGoldenDecoderRejectsUnexpectedFields(t *testing.T) {
	var target struct {
		ResponseStatus struct{ ErrorCode, Message string }
	}
	decoder := json.NewDecoder(strings.NewReader(`{"ResponseStatus":{"ErrorCode":"BadRequest","Message":"Bad Request","SQL":"private"}}`))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&target); err == nil || errors.Is(err, io.EOF) {
		t.Fatal("error contract accepted internal fields")
	}
}
