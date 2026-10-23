package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestStatisticsAndIPGeolocationRemainAdministratorOnly(t *testing.T) {
	h := New(cfg(), Dependencies{})
	native, e := NativeMediaHandler(cfg(), Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/api/v1/media-server/device-ips", "/api/v1/media-server/playback-events", "/api/v1/media-server/playback-statistics", "/api/v1/media-server/playback-ips", "/api/v1/ip-geolocation?ips=8.8.8.8", "/api/v1/ip-geolocation/settings"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("X-Emby-Token", "synthetic-media-token")
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(path, w.Code)
		}
		w = httptest.NewRecorder()
		native.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatal("media API exposed statistics", path, w.Code)
		}
	}
}
func TestStatisticsFiltersRejectInvalidIDsAndDays(t *testing.T) {
	for _, q := range []string{"?days=1", "?userId=not-id", "?deviceId=bad", "?startIndex=-1", "?days=abc"} {
		w := httptest.NewRecorder()
		if _, ok := statisticsFilter(w, httptest.NewRequest("GET", "/"+q, nil)); ok || w.Code != 400 {
			t.Fatal(q, w.Code)
		}
	}
}
