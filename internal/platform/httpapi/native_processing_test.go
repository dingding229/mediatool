package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/mediaprobe/admin"
	"media-tool/internal/modules/mediaprobe/stillstore"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

type contractResponse[T any] struct {
	Data T `json:"data"`
}
type processingContract struct {
	Summary struct {
		Response contractResponse[admin.Summary] `json:"response"`
	} `json:"summary"`
	Settings struct {
		Request  admin.Settings                   `json:"request"`
		Response contractResponse[admin.Settings] `json:"response"`
	} `json:"settings"`
	Tasks struct {
		Request struct {
			Kind     string `json:"kind"`
			State    string `json:"state"`
			Page     int    `json:"page"`
			PageSize int    `json:"pageSize"`
		} `json:"request"`
		Response contractResponse[admin.Page] `json:"response"`
	} `json:"tasks"`
	Retry struct {
		Request  admin.Retry                     `json:"request"`
		Response contractResponse[admin.Retried] `json:"response"`
	} `json:"retry"`
	Batch struct {
		Request  admin.Retry                     `json:"request"`
		Response contractResponse[admin.Retried] `json:"response"`
	} `json:"batch"`
	GC struct {
		Response contractResponse[nativeGC] `json:"response"`
	} `json:"gc"`
}

func readProcessingContract(t *testing.T) processingContract {
	t.Helper()
	raw, err := os.ReadFile("testdata/contracts/native-processing.json")
	if err != nil {
		t.Fatal(err)
	}
	var f processingContract
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&f); err != nil {
		t.Fatal(err)
	}
	return f
}

type fakeProcessing struct {
	f      processingContract
	t      *testing.T
	err    error
	audits int
}

func (f *fakeProcessing) Summary(context.Context) (admin.Summary, error) {
	return f.f.Summary.Response.Data, f.err
}
func (f *fakeProcessing) Settings(_ context.Context, in admin.Settings) (admin.Settings, error) {
	if in != f.f.Settings.Request {
		f.t.Error("settings request")
	}
	return f.f.Settings.Response.Data, f.err
}
func (f *fakeProcessing) List(_ context.Context, in admin.Filter) (admin.Page, error) {
	if in.Kind != "probe" || in.State != "failed" || in.Page != 1 || in.PageSize != 50 {
		f.t.Error(in)
	}
	return f.f.Tasks.Response.Data, f.err
}
func (f *fakeProcessing) Retry(_ context.Context, in admin.Retry) (admin.Retried, error) {
	if in.Bulk {
		if in != f.f.Batch.Request {
			f.t.Error(in)
		}
		return f.f.Batch.Response.Data, f.err
	}
	if in != f.f.Retry.Request {
		f.t.Error(in)
	}
	return f.f.Retry.Response.Data, f.err
}
func (f *fakeProcessing) Audit(context.Context, string, any) error { f.audits++; return f.err }

type fakeGC struct{}

func (fakeGC) ReadStill(context.Context, string) ([]byte, string, error) { return nil, "", nil }
func (fakeGC) Collect(context.Context) (stillstore.GCReport, error) {
	return stillstore.GCReport{Visited: 8, Reclaimed: 2, ReclaimedBytes: 1024}, nil
}
func TestNativeProcessingSharedContract(t *testing.T) {
	f := readProcessingContract(t)
	service := &fakeProcessing{f: f, t: t}
	a := &API{Deps: Dependencies{NativeProcessing: service, NativeStills: fakeGC{}}}
	for _, tt := range []struct {
		name, method, path string
		in, expected       any
		status             int
		handler            http.HandlerFunc
	}{
		{"summary", "GET", "/", nil, f.Summary.Response, 200, a.processingSummary},
		{"settings", "PUT", "/", f.Settings.Request, f.Settings.Response, 200, a.processingSettings},
		{"tasks", "GET", "/?kind=probe&state=failed&page=1&pageSize=50", nil, f.Tasks.Response, 200, a.processingTasks},
		{"retry", "POST", "/", f.Retry.Request, f.Retry.Response, 202, a.processingRetry},
		{"batch", "POST", "/", f.Batch.Request, f.Batch.Response, 202, a.processingRetry},
		{"gc", "POST", "/", nil, f.GC.Response, 200, a.processingGC},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var body bytes.Buffer
			if tt.in != nil {
				if err := json.NewEncoder(&body).Encode(tt.in); err != nil {
					t.Fatal(err)
				}
			}
			w := httptest.NewRecorder()
			tt.handler(w, httptest.NewRequest(tt.method, tt.path, &body))
			if w.Code != tt.status {
				t.Fatal(w.Code, w.Body.String())
			}
			got := reflect.New(reflect.TypeOf(tt.expected))
			dec := json.NewDecoder(w.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(got.Interface()); err != nil || !reflect.DeepEqual(got.Elem().Interface(), tt.expected) {
				t.Fatal("response", err, got.Elem().Interface())
			}
		})
	}
	if service.audits != 2 {
		t.Fatal("GC request/result audits", service.audits)
	}
}
func TestNativeProcessingValidationAuthIsolationAndSafeErrors(t *testing.T) {
	f := readProcessingContract(t)
	service := &fakeProcessing{f: f, t: t, err: errors.New("https://private.invalid/?token=SECRET")}
	a := &API{Deps: Dependencies{NativeProcessing: service}}
	for _, path := range []string{"/?kind=still&state=probing", "/?kind=probe&pageSize=201", "/?kind=probe&page=0", "/?kind=probe&page=1&page=2", "/?kind=other"} {
		w := httptest.NewRecorder()
		a.processingTasks(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	for _, body := range []string{`{"kind":"probe","id":"bad"}`, `{"kind":"probe","bulk":true,"confirmed":false}`, `{"kind":"still","id":"-1"}`, `{"kind":"still","bulk":true,"confirmed":true,"id":"100001"}`} {
		w := httptest.NewRecorder()
		a.processingRetry(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatal(body, w.Code)
		}
	}
	for _, body := range []string{`{}`, `{"enabled":null,"probeEnabled":true,"stillsEnabled":true,"revision":1}`, `{"enabled":false,"probeEnabled":true,"stillsEnabled":true,"revision":1,"url":"bad"}`} {
		w := httptest.NewRecorder()
		a.processingSettings(w, httptest.NewRequest("PUT", "/", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatal(body, w.Code)
		}
	}
	w := httptest.NewRecorder()
	a.processingSummary(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 503 || strings.Contains(w.Body.String(), "SECRET") {
		t.Fatal(w.Code, w.Body.String())
	}
	media, err := NativeMediaHandler(cfg(), Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ method, suffix string }{{"GET", ""}, {"PUT", "/settings"}, {"GET", "/tasks"}, {"POST", "/retry"}, {"POST", "/gc"}} {
		path := "/api/v1/media-server/native/processing" + entry.suffix
		w := httptest.NewRecorder()
		New(cfg(), Dependencies{}).ServeHTTP(w, httptest.NewRequest(entry.method, path, nil))
		if w.Code != 401 {
			t.Fatal("missing admin boundary", path, w.Code)
		}
		w = httptest.NewRecorder()
		media.ServeHTTP(w, httptest.NewRequest(entry.method, path, nil))
		if w.Code != 404 {
			t.Fatal("media port exposed admin", path, w.Code)
		}
	}
}

type partialGC struct{ fakeGC }

func (partialGC) Collect(context.Context) (stillstore.GCReport, error) {
	return stillstore.GCReport{Visited: 8, Failures: 1}, nil
}
func TestNativeProcessingGCReportsCleanupFailureAsPartial(t *testing.T) {
	service := &fakeProcessing{t: t}
	a := &API{Deps: Dependencies{NativeProcessing: service, NativeStills: partialGC{}}}
	w := httptest.NewRecorder()
	a.processingGC(w, httptest.NewRequest("POST", "/", nil))
	var out contractResponse[nativeGC]
	dec := json.NewDecoder(w.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil || w.Code != 200 || !out.Data.Partial || out.Data.Scanned != 8 || out.Data.Reclaimed != 0 {
		t.Fatal(w.Code, w.Body.String(), err)
	}
}

func TestNativeProcessingMissingDatabaseIsUnavailableNotBadInput(t *testing.T) {
	w := httptest.NewRecorder()
	(&API{}).processingSummary(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
}
