package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestT16RemovedExternalRoutesAre404(t *testing.T) {
	h := New(cfg(), Dependencies{})
	for _, p := range []string{"/api/v1/emby/test", "/api/v1/emby/check", "/api/v1/library/outputs/fixture/emby", "/api/v1/media/emby/items/fixture", "/api/v1/media/library/emby/home", "/api/v1/media/library/emby/fixture/image", "/api/v1/player-access/users", "/emby/Items/fixture/PlaybackInfo"} {
		for _, method := range []string{"GET", "POST"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(method, p, nil))
			if w.Code != 404 {
				t.Fatalf("%s %s=%d", method, p, w.Code)
			}
		}
	}
}
