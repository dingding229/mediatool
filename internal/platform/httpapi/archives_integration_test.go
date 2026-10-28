package httpapi

import "testing"

func TestI12ArchiveManagementRoutesRemoved(t *testing.T) {
	h := New(cfg(), Dependencies{})
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/v1/library/archives"}, {"GET", "/api/v1/library/archive-policy"},
		{"POST", "/api/v1/library/archive-policy"}, {"POST", "/api/v1/library/archives/cleanup-preview"},
		{"POST", "/api/v1/library/archives/cleanup-confirm"}, {"GET", "/api/v1/library/archive-cleanups/11111111-1111-4111-8111-111111111111"},
		{"POST", "/api/v1/library/archive-cleanups/11111111-1111-4111-8111-111111111111/retry"}, {"GET", "/api/v1/library/artifact-repairs"},
	} {
		if w := invoke(h, route.method, route.path, "{}", ""); w.Code != 404 {
			t.Fatalf("retired route %s %s returned %d", route.method, route.path, w.Code)
		}
	}
}
