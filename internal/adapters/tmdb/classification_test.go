package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClassificationOnlyRequestIgnoresUnrelatedMetadataAndChecksIdentity(t *testing.T) {
	body := `{"id":700,"original_language":"cn","origin_country":["HK"],"overview":"https://unsafe.invalid/unused","poster_path":"https://unsafe.invalid/not-fetched","genres":[{"id":18,"name":"Drama"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/tv/700" || r.URL.Query().Get("append_to_response") != "" {
			t.Error("repair requested unrelated resources")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	c := Client{HTTP: server.Client(), BaseURL: server.URL}
	f, e := c.ClassificationDetails(context.Background(), "series", 700)
	if e != nil || !f.Valid() || f.Countries[0] != "HK" || f.OriginalLanguage != "cn" {
		t.Fatal(f, e)
	}
	body = `{"id":701,"original_language":"cn","origin_country":["HK"]}`
	if _, e = c.ClassificationDetails(context.Background(), "series", 700); e == nil {
		t.Fatal("wrong provider identity accepted")
	}
	if _, e = c.ClassificationDetails(context.Background(), "season", 700); e == nil {
		t.Fatal("season repair accepted")
	}
}
