package httpapi

import (
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
}
