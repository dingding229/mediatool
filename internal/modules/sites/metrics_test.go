package sites

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type metricsTransport func(*http.Request) (*http.Response, error)

func (f metricsTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestUserMetricsUnknownAndExactValues(t *testing.T) {
	rule := Map{"parser": "JsonPath", "path": "/profile", "fields": Map{"is_login": Map{"selector": "$.login"}, "id": Map{"selector": "$.id"}, "name": Map{"selector": "$.name"}, "uploaded": Map{"selector": "$.uploaded"}, "downloaded": Map{"selector": "$.downloaded"}, "ratio": Map{"selector": "$.ratio"}, "bonus": Map{"selector": "$.bonus"}}}
	c := Client{Config: Config{Domain: "https://owned.example", Requests: map[string]Map{"user_basic_info": rule}}, Account: Account{Endpoint: "https://owned.example"}, HTTP: &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"login":true,"id":"1","name":"owned","uploaded":1099511627776,"downloaded":1073741824,"ratio":1024,"bonus":"1,234.5"}`)), Header: http.Header{}, Request: r}, nil
	})}}
	m, e := c.Metrics(context.Background())
	if e != nil || m.Uploaded == nil || *m.Uploaded != 1<<40 || m.Bonus == nil || *m.Bonus != 1234.5 || m.Seeding != nil || !m.Partial {
		t.Fatal(m, e)
	}
	if metricBytes("not a value") != nil || metricBytes("1.5") != nil {
		t.Fatal("invented values")
	}
}
