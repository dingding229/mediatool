package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNativeListenerNeverExposesManagementOrPWA(t *testing.T) {
	member, err := NativeMediaHandler(cfg(), Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"/", "/login", "/password", "/dashboard", "/assets/app.js", "/manifest.webmanifest", "/sw.js", "/offline.html", "/pwa/icon-192.png", "/pwa/icon-512.png", "/api/v1/media-server/users", "/api/v1/system/status"}
	for _, prefix := range []string{"", "/emby", "/mediabrowser"} {
		for _, method := range []string{"GET", "HEAD", "POST", "PUT", "DELETE", "OPTIONS"} {
			for _, path := range paths {
				w := httptest.NewRecorder()
				member.ServeHTTP(w, httptest.NewRequest(method, prefix+path, nil))
				if w.Code != 404 || (method == "HEAD" && w.Body.Len() != 0) || (method != "HEAD" && !json.Valid(w.Body.Bytes())) {
					t.Fatalf("%s %s: %d %q", method, prefix+path, w.Code, w.Body.String())
				}
				if w.Header().Get("Service-Worker-Allowed") != "" {
					t.Fatal("PWA scope leaked")
				}
			}
		}
	}
	w := httptest.NewRecorder()
	member.ServeHTTP(w, httptest.NewRequest("GET", "/System/Info/Public", nil))
	if w.Code != 503 {
		t.Fatal("settings failure should fail closed", w.Code)
	}
	management := New(cfg(), Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	for _, path := range []string{"/System/Info/Public", "/System/Ping", "/Users/Public", "/Items/100001/PlaybackInfo"} {
		w := httptest.NewRecorder()
		management.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if strings.Contains(w.Body.String(), `"ServerName"`) || strings.Contains(w.Body.String(), `"ResponseStatus"`) || strings.Contains(w.Body.String(), `"StartupWizardCompleted"`) {
			t.Fatal("native protocol exposed by management", path, w.Body.String())
		}
	}
}
