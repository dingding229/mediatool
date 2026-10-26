package tooling

import (
	"context"
	"media-tool/internal/adapters/qbittorrent"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestT16OfflineDownloaderFixtureAuthenticatesWithoutExternalNetwork(t *testing.T) {
	s := httptest.NewServer(downloaderFixtureHandler())
	defer s.Close()
	a := qbittorrent.Adapter{Endpoint: s.URL, Username: "owned-fixture-user", Password: "owned-fixture-password"}
	if v, e := a.Probe(context.Background()); e != nil || v != "v4.6.5" {
		t.Fatal(v, e)
	}
	r, e := http.Get(s.URL + "/api/v2/app/version")
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatal("unauthenticated fixture opened", r.StatusCode)
	}
}
