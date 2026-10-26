package tooling

import (
	"io"
	"net/http"
	"time"
)

// Offline E2E provider replaces the retired external-Emby fixture. This is a
// build/test command, not an application listener or a real downloader.
func downloaderFixtureHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v2/auth/login" {
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if r.ParseForm() != nil || r.PostForm.Get("username") != "owned-fixture-user" || r.PostForm.Get("password") != "owned-fixture-password" {
				http.Error(w, "fixture authentication required", 403)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "owned-offline-fixture", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			_, _ = io.WriteString(w, "Ok.")
			return
		}
		if r.Method != "GET" || r.URL.Path != "/api/v2/app/version" {
			http.NotFound(w, r)
			return
		}
		cookie, e := r.Cookie("SID")
		if e != nil || cookie.Value != "owned-offline-fixture" {
			http.Error(w, "fixture authentication required", 403)
			return
		}
		_, _ = io.WriteString(w, "v4.6.5")
	})
}
func downloaderFixture() error {
	s := &http.Server{Addr: "127.0.0.1:18086", Handler: downloaderFixtureHandler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	return s.ListenAndServe()
}
