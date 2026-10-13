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

func TestDeclaredTTGZeroActivityProvesZeroVolumeOnlyForExactZero(t *testing.T) {
	for _, value := range []string{"0", "2", "not-a-number"} {
		basic := Map{"parser": "XPath", "path": "/profile", "seeding_zero_is_complete": true, "fields": Map{"is_login": Map{"selector": "//a[@href='logout']", "filters": []any{Map{"name": "not_blank"}}}, "id": Map{"selector": "//b[@id='uid']"}, "name": Map{"selector": "//b[@id='name']"}, "uploaded": Map{"selector": "//b[@id='uploaded']"}, "seeding": Map{"selector": "//img[@alt='做种中']/following-sibling::font[1]"}}}
		c := Client{Config: Config{Schema: "ttg", Domain: "https://owned.example", Requests: map[string]Map{"user_basic_info": basic}}, Account: Account{Endpoint: "https://owned.example"}, HTTP: &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Request: r, Body: io.NopCloser(strings.NewReader(`<a href="logout">Exit</a><b id="uid">42</b><b id="name">owned</b><b id="uploaded">100</b><img alt="做种中"><font><span>` + value + `</span></font>`))}, nil
		})}}
		m, e := c.Metrics(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		if value == "0" {
			if m.Seeding == nil || m.SeedingBytes == nil || *m.Seeding != 0 || *m.SeedingBytes != 0 || !m.SeedingComplete {
				t.Fatal(m)
			}
		} else if m.SeedingBytes != nil {
			t.Fatal("nonzero/unknown inferred volume", m)
		}
	}
}
