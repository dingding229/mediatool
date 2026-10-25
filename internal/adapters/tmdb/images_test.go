package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestImageCandidatesOnlyExplicitFixedEndpoint(t *testing.T) {
	for _, tc := range []struct{ kind, path string }{{"movie", "/movie/42/images"}, {"series", "/tv/42/images"}, {"season", "/tv/42/season/1/images"}, {"episode", "/tv/42/season/1/episode/2/images"}} {
		t.Run(tc.kind, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path || r.Header.Get("Authorization") != "Bearer test" || r.URL.Query().Get("include_image_language") != "zh,en,null" {
					t.Error(r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"posters":[{"file_path":"/ok.jpg"},{"file_path":"https://evil/a.jpg"}],"stills":[{"file_path":"/still.jpg"}],"logos":[{"file_path":"/logo.png"}]}`))
			}))
			defer s.Close()
			out, e := (Client{BaseURL: s.URL, Token: "test", HTTP: s.Client()}).ImageCandidates(context.Background(), tc.kind, 42, 1, 2)
			if e != nil || len(out["Primary"]) != 2 || len(out["Logo"]) != 1 {
				t.Fatal(out, e)
			}
		})
	}
}
