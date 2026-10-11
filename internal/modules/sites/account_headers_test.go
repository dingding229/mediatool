package sites

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAccountHeaderJSONIsStrictCanonicalAndLiteral(t *testing.T) {
	headers, err := ParseAccountHeaders(`{"authorization":"Bearer literal{passkey}","x-csrf-token":"PRIVATE-CSRF"}`)
	if err != nil || len(headers) != 2 || headers["Authorization"] != "Bearer literal{passkey}" || headers["X-Csrf-Token"] != "PRIVATE-CSRF" {
		t.Fatal("valid literal credentials not canonicalized", err)
	}
	for _, raw := range []string{"", "{}", " { } "} {
		values, err := ParseAccountHeaders(raw)
		if err != nil || len(values) != 0 {
			t.Fatal("explicit empty credential object rejected", err)
		}
	}
	for _, raw := range []string{`{"A":"one","A":"two"}`, `{"a":"one","A":"two"}`, `{"Authorization":"PRIVATE-AUTH"}[]`, `{"Authorization":["PRIVATE-AUTH"]}`, `{"Host":"PRIVATE-HOST"}`, `{"Authorization":"PRIVATE\u0000AUTH"}`, `{"Authorization":" "}`, strings.Repeat(" ", 16385)} {
		_, err := ParseAccountHeaders(raw)
		if !errors.Is(err, ErrAccountHeaders) || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("unsafe credential JSON accepted or exposed", err)
		}
	}
	fields := []string{}
	for i := 0; i < 17; i++ {
		fields = append(fields, fmt.Sprintf(`"X-Owned-%d":"owned"`, i))
	}
	if _, err := ParseAccountHeaders("{" + strings.Join(fields, ",") + "}"); !errors.Is(err, ErrAccountHeaders) {
		t.Fatal("credential header count unbounded", err)
	}
}

func TestAccountHeadersApplyOnlyToExactRequiredNamesAndAreNotTemplates(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer literal{passkey}" || r.Header.Get("X-Csrf-Token") != "PRIVATE-CSRF" || r.Header.Get("X-Owned-Unused") != "" {
			w.WriteHeader(403)
			return
		}
		received.Add(1)
		fmt.Fprint(w, "owned")
	}))
	defer server.Close()
	client := Client{Config: Config{Domain: server.URL}, Account: Account{Endpoint: server.URL, Passkey: "PRIVATE-PASSKEY", RequestHeaders: map[string]string{"authorization": "Bearer literal{passkey}", "X-Csrf-Token": "PRIVATE-CSRF", "X-Owned-Unused": "PRIVATE-EXTRA"}}}
	_, _, err := client.call(context.Background(), Map{"path": "/", "parser": "None", "required_headers": []any{"Authorization", "X-Csrf-Token"}, "headers": Map{"Authorization": "public-template"}}, client.vars("", 1), nil)
	if err != nil || received.Load() != 1 {
		t.Fatal("explicit required account headers not applied literally", err)
	}
	if client.Account.RequestHeaders["authorization"] != "Bearer literal{passkey}" {
		t.Fatal("request mutated supplied credential map")
	}
}

func TestAccountHeaderPrecedenceKeepsExistingAPIKeyAndSameOriginRedirect(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "PRIVATE-API-KEY" || r.Header.Get("Authorization") != "PRIVATE-AUTH" {
			w.WriteHeader(403)
			return
		}
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		received.Add(1)
		fmt.Fprint(w, "owned")
	}))
	defer server.Close()
	client := Client{Config: Config{Domain: server.URL}, Account: Account{Endpoint: server.URL, APIKey: "PRIVATE-API-KEY", RequestHeaders: map[string]string{"x-api-key": "PRIVATE-OTHER", "Authorization": "PRIVATE-AUTH"}}}
	_, _, err := client.call(context.Background(), Map{"path": "/", "parser": "None", "required_headers": []any{"x-api-key", "Authorization"}}, nil, nil)
	if err != nil || received.Load() != 1 {
		t.Fatal("account API key or same-origin header contract changed", err)
	}
}

func TestEncryptedAccountHeadersCannotEscapeAcrossRedirectOrUnusedRequest(t *testing.T) {
	var leaked atomic.Bool
	var hits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, name := range []string{"Authorization", "X-Csrf-Token", "X-Owned-Required"} {
			if r.Header.Get(name) != "" {
				leaked.Store(true)
			}
		}
		hits.Add(1)
		fmt.Fprint(w, "owned")
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer origin.Close()
	client := Client{Config: Config{Domain: origin.URL, AdditionalDomains: []string{other.URL}}, Account: Account{Endpoint: origin.URL, RequestHeaders: map[string]string{"Authorization": "PRIVATE-AUTH", "X-Csrf-Token": "PRIVATE-CSRF", "X-Owned-Required": "PRIVATE-EXTRA"}}}
	_, _, err := client.call(context.Background(), Map{"path": "/", "parser": "None", "required_headers": []any{"Authorization", "X-Csrf-Token", "X-Owned-Required"}}, nil, nil)
	if err != nil || hits.Load() != 1 || leaked.Load() {
		t.Fatal("account-required credentials escaped across origin", leaked.Load(), err)
	}
	_, _, err = client.call(context.Background(), Map{"path": other.URL, "parser": "None"}, nil, nil)
	if err != nil || leaked.Load() {
		t.Fatal("unused account credentials attached to allowed public download", err)
	}
	_, _, err = client.call(context.Background(), Map{"path": other.URL, "parser": "None", "required_headers": []any{"Authorization"}}, nil, nil)
	if !errors.Is(err, ErrConfig) {
		t.Fatal("account-required headers attached to an absolute additional origin", err)
	}
}
