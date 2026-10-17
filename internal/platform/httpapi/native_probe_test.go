package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"media-tool/internal/modules/mediaprobe/probequeue"
)

type probeRetryFunc func(context.Context, string) (bool, error)

func (f probeRetryFunc) Retry(ctx context.Context, id string) (bool, error) { return f(ctx, id) }
func TestNativeProbeRetryContractAuthorizationAndPortIsolation(t *testing.T) {
	raw, err := os.ReadFile("testdata/contracts/native-probe-retry.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Request struct {
			AssetID string `json:"assetId"`
		} `json:"request"`
		Response struct {
			Data nativeProbeQueued `json:"data"`
		} `json:"response"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	api := &API{Deps: Dependencies{NativeProbeRetry: probeRetryFunc(func(_ context.Context, id string) (bool, error) {
		if id != fixture.Request.AssetID {
			t.Error("request contract")
		}
		return true, nil
	})}}
	path := "/api/v1/media-server/native/probe/" + fixture.Request.AssetID + "/retry"
	req := httptest.NewRequest("POST", path, nil)
	req.SetPathValue("assetId", fixture.Request.AssetID)
	w := httptest.NewRecorder()
	api.retryNativeProbe(w, req)
	var response struct {
		Data nativeProbeQueued `json:"data"`
	}
	dec = json.NewDecoder(w.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&response); err != nil || response != fixture.Response || w.Code != 202 {
		t.Fatal("response contract", err)
	}
	w = httptest.NewRecorder()
	New(cfg(), Dependencies{}).ServeHTTP(w, httptest.NewRequest("POST", path, nil))
	if w.Code != 401 {
		t.Fatal("retry lacks admin auth", w.Code)
	}
	media, err := NativeMediaHandler(cfg(), Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"POST", "GET", "HEAD"} {
		w = httptest.NewRecorder()
		media.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != 404 {
			t.Fatal("admin retry leaked to media port", w.Code)
		}
	}
}
func TestNativeProbeRetrySafeErrors(t *testing.T) {
	for _, tt := range []struct {
		id     string
		err    error
		status int
	}{{"bad", nil, 400}, {"12345678-1234-4234-8234-123456789abc", probequeue.ErrDisabled, 503}, {"12345678-1234-4234-8234-123456789abc", probequeue.ErrNotExposable, 404}, {"12345678-1234-4234-8234-123456789abc", errors.New("https://private.example/?TOKEN=NEVER_LOG"), 503}} {
		api := &API{Deps: Dependencies{NativeProbeRetry: probeRetryFunc(func(context.Context, string) (bool, error) { return false, tt.err })}}
		req := httptest.NewRequest("POST", "/", nil)
		req.SetPathValue("assetId", tt.id)
		w := httptest.NewRecorder()
		api.retryNativeProbe(w, req)
		if w.Code != tt.status || strings.Contains(w.Body.String(), "NEVER_LOG") || strings.Contains(w.Body.String(), "private.example") {
			t.Fatal("unsafe error", w.Code)
		}
	}
}
