package open115

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOfficialAccountInfoExactSpaceAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		body  string
		space bool
		valid bool
	}{{`{"state":true,"data":{"user_id":42,"user_name":"Owned account","rt_space_info":{"all_total":{"size":"1000"},"all_use":{"size":300},"all_remain":{"size":"700"}},"vip_info":{"level_name":"VIP","expire":1798761600}}}`, true, true}, {`{"state":true,"data":{"user_id":"42"}}`, false, true}, {`{"state":true,"data":{"user_id":"42","rt_space_info":{"all_total":{"size":100},"all_use":{"size":300},"all_remain":{"size":700}}}}`, false, true}, {`{"state":true,"data":{"user_name":"No identity"}}`, false, false}} {
		client := Client{HTTP: &http.Client{Transport: accountTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != APIOrigin+"/open/user/info" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer owned-token" {
				t.Fatal("wrong official request", r.URL)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
		})}}
		out, err := client.AccountInfo(context.Background(), "owned-token")
		if (err == nil) != tc.valid {
			t.Fatal(out, err)
		}
		if tc.valid && (out.Space != nil) != tc.space {
			t.Fatal(out)
		}
		if tc.space && (out.Space.Free != 700 || out.UserID != "42" || out.Memberships[0].ExpiresAt == nil) {
			t.Fatal(out)
		}
	}
}

type accountTransport func(*http.Request) (*http.Response, error)

func (f accountTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
