package tmdb

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type autoTransport func(*http.Request) (*http.Response, error)

func (f autoTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestAutoMediaSearchSkipsPersonsKeepsActualTypes(t *testing.T) {
	client := Client{Token: "owned", HTTP: &http.Client{Transport: autoTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/3/search/multi" {
			t.Fatal(r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"results":[{"id":1,"media_type":"movie","title":"Owned Movie","release_date":"2024-01-01"},{"id":2,"media_type":"tv","name":"Owned Show","first_air_date":"2025-01-01"},{"id":3,"media_type":"person","name":"Not a movie"}]}`)), Header: http.Header{}}, nil
	})}}
	out, e := client.Search(context.Background(), "auto", "Owned")
	if e != nil || len(out) != 2 || out[0].MediaType != "movie" || out[1].MediaType != "series" {
		t.Fatal(out, e)
	}
}
