package httpapi

import (
	"encoding/json"
	"media-tool/internal/modules/system"
	"net/http"
	"testing"
)

func TestOperationsAdminBoundaryAndDegradedSnapshot(t *testing.T) {
	h, _ := testHandler(nil)
	for _, path := range []string{"/api/v1/system/operations", "/api/v1/jobs/browse", "/api/v1/jobs/00000000-0000-4000-8000-000000000001"} {
		if w := invoke(h, http.MethodGet, path, "", ""); w.Code != 401 {
			t.Fatal(path, w.Code)
		}
	}
	w := invoke(h, http.MethodGet, "/api/v1/system/operations", "", cfg().SessionKey)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct{ Data system.Snapshot }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data.Schedules) == 0 || out.Data.Schedules[0].Name != "核心协调" || out.Data.Schedules[0].Cron != "*/15 * * * * *" {
		t.Fatal("core schedule name/frequency", out.Data.Schedules)
	}
}
