package httpapi

import (
	"encoding/json"
	"media-tool/internal/platform/config"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeReadOnlyProcSnapshotAndUnavailableValues(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "loadavg"), []byte("1.25 2.50 3.75 1/12 1\n"), 0600)
	os.WriteFile(filepath.Join(dir, "meminfo"), []byte("MemTotal: 8192 kB\nMemAvailable: 2048 kB\n"), 0600)
	os.WriteFile(filepath.Join(dir, "uptime"), []byte("12345 12345"), 0600)
	a := API{Config: config.Config{HostProcDir: dir, STRMOutputDir: dir}}
	w := httptest.NewRecorder()
	a.runtimeSummary(w, httptest.NewRequest("GET", "/", nil))
	var out struct {
		Data struct {
			Server map[string]any `json:"server"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out.Data.Server["memoryUsed"] != float64(6144*1024) || out.Data.Server["diskAvailable"] == nil {
		t.Fatal(w.Body.String())
	}
	a.Config.HostProcDir = filepath.Join(dir, "missing")
	w = httptest.NewRecorder()
	a.runtimeSummary(w, httptest.NewRequest("GET", "/", nil))
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Data.Server["procAvailable"] != false {
		t.Fatal("missing proc fabricated values")
	}
}
