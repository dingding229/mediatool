package sites

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDeclaredRequiredHeadersAcceptExplicitValuesWithoutGuessingAuthentication(t *testing.T) {
	for _, header := range []string{"Authorization", "X-Csrf-Token", "X-Owned-Required"} {
		t.Run(header, func(t *testing.T) {
			var received atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get(header) != "owned PRIVATE-HEADER-VALUE" {
					w.WriteHeader(403)
					return
				}
				received.Add(1)
				fmt.Fprint(w, "owned")
			}))
			defer server.Close()
			client := Client{Config: Config{Domain: server.URL}, Account: Account{Endpoint: server.URL}}
			rule := Map{"path": "/", "parser": "None", "headers": Map{header: "owned {authValue}"}, "required_headers": []any{header}}
			_, _, err := client.call(context.Background(), rule, map[string]string{"authValue": "PRIVATE-HEADER-VALUE"}, nil)
			if err != nil || received.Load() != 1 {
				t.Fatal("explicit required header ignored", err)
			}
		})
	}
}

func TestAllowedOriginRedirectDoesNotForwardDeclaredCredentialHeaders(t *testing.T) {
	var received atomic.Int32
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		for _, name := range []string{"Cookie", "Authorization", "X-Csrf-Token", "X-Owned-Secret", "x-api-key"} {
			if r.Header.Get(name) != "" {
				leaked.Store(true)
			}
		}
		fmt.Fprint(w, "owned")
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer origin.Close()
	client := Client{Config: Config{Domain: origin.URL, AdditionalDomains: []string{other.URL}}, Account: Account{Endpoint: origin.URL, Cookie: "owned=PRIVATE-COOKIE", APIKey: "PRIVATE-API-KEY"}}
	rule := Map{"path": "/", "parser": "None", "headers": Map{"Authorization": "Bearer PRIVATE-AUTH", "X-Csrf-Token": "PRIVATE-CSRF", "X-Owned-Secret": "PRIVATE-EXTRA"}, "required_headers": []any{"x-api-key"}}
	_, _, err := client.call(context.Background(), rule, nil, nil)
	if err != nil || received.Load() != 1 || leaked.Load() {
		t.Fatal("cross-origin credential forwarding", received.Load(), leaked.Load(), err)
	}
}

func TestTLSRedirectCannotDowngradeEvenToDeclaredOrigin(t *testing.T) {
	var received atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1); fmt.Fprint(w, "owned") }))
	defer other.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer origin.Close()
	client := Client{Config: Config{Domain: origin.URL, AdditionalDomains: []string{other.URL}}, Account: Account{Endpoint: origin.URL, Cookie: "owned=PRIVATE-COOKIE"}, HTTP: origin.Client()}
	_, _, err := client.call(context.Background(), Map{"path": "/", "parser": "None"}, nil, nil)
	if !errors.Is(err, ErrRequest) || received.Load() != 0 {
		t.Fatal("HTTPS request followed plaintext downgrade", received.Load(), err)
	}
}

func TestRequestOriginIncludesSchemeAndEffectivePort(t *testing.T) {
	client := Client{Config: Config{Domain: "https://OWNED.example"}, Account: Account{Endpoint: "https://owned.example"}}
	for _, sample := range []struct {
		url     string
		allowed bool
	}{
		{"https://owned.example/path", true}, {"https://OWNED.example:443/path", true},
		{"http://owned.example/path", false}, {"https://owned.example:8443/path", false},
	} {
		u, _ := url.Parse(sample.url)
		if client.allowed(u) != sample.allowed {
			t.Fatal("origin scheme/port mismatch", sample.url)
		}
	}
}

func TestCrossOrigin307DoesNotResubmitRequestBody(t *testing.T) {
	var received atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1); fmt.Fprint(w, "owned") }))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	client := Client{Config: Config{Domain: origin.URL, AdditionalDomains: []string{other.URL}}, Account: Account{Endpoint: origin.URL}}
	_, _, err := client.call(context.Background(), Map{"path": "/", "parser": "None", "method": "POST", "body": Map{"auth": "{private}"}}, map[string]string{"private": "PRIVATE-BODY-VALUE"}, nil)
	if !errors.Is(err, ErrRequest) || received.Load() != 0 {
		t.Fatal("cross-origin redirect resubmitted original body", received.Load(), err)
	}
}

func TestSameOriginRedirectPreservesRequiredCredentials(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		if r.Header.Get("Cookie") != "owned=PRIVATE-COOKIE" || r.Header.Get("x-api-key") != "PRIVATE-API-KEY" || r.Header.Get("X-Owned-Secret") != "PRIVATE-EXTRA" {
			w.WriteHeader(403)
			return
		}
		received.Add(1)
		fmt.Fprint(w, "owned")
	}))
	defer server.Close()
	client := Client{Config: Config{Domain: server.URL}, Account: Account{Endpoint: server.URL, Cookie: "owned=PRIVATE-COOKIE", APIKey: "PRIVATE-API-KEY"}}
	_, _, err := client.call(context.Background(), Map{"path": "/", "parser": "None", "headers": Map{"X-Owned-Secret": "PRIVATE-EXTRA"}, "required_headers": []any{"x-api-key"}}, nil, nil)
	if err != nil || received.Load() != 1 {
		t.Fatal("same-origin request lost valid credentials", err)
	}
}

func TestRequiredHeadersMustHaveNonemptyTypedValuesBeforeHTTP(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1); fmt.Fprint(w, "owned") }))
	defer server.Close()
	client := Client{Config: Config{Domain: server.URL}, Account: Account{Endpoint: server.URL}}
	for _, headers := range []Map{{}, {"Authorization": ""}, {"Authorization": "{missing}"}, {"Authorization": 12}, {"Authorization": "bad\r\nInjected: secret"}} {
		_, _, err := client.call(context.Background(), Map{"path": "/", "parser": "None", "headers": headers, "required_headers": []any{"Authorization"}}, nil, nil)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("invalid or missing required value did not fail safely")
		}
	}
	if received.Load() != 0 {
		t.Fatal("invalid required header reached HTTP", received.Load())
	}
}

func TestRedirectChainCannotRestoreCredentialsWhenReturningToInitialOrigin(t *testing.T) {
	var checked atomic.Int32
	var leaked atomic.Bool
	var originURL string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, name := range []string{"Authorization", "X-Owned-Secret", "Cookie", "Referer"} {
			if r.Header.Get(name) != "" {
				leaked.Store(true)
			}
		}
		http.Redirect(w, r, originURL+"/returned", http.StatusFound)
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			if r.Header.Get("Authorization") != "Bearer PRIVATE-AUTH" || r.Header.Get("Cookie") != "owned=PRIVATE-COOKIE" {
				w.WriteHeader(403)
				return
			}
			http.Redirect(w, r, other.URL, http.StatusFound)
			return
		}
		for _, name := range []string{"Authorization", "X-Owned-Secret", "Cookie", "Referer"} {
			if r.Header.Get(name) != "" {
				leaked.Store(true)
			}
		}
		checked.Add(1)
		fmt.Fprint(w, "owned")
	}))
	defer origin.Close()
	originURL = origin.URL
	client := Client{Config: Config{Domain: origin.URL, AdditionalDomains: []string{other.URL}}, Account: Account{Endpoint: origin.URL, Cookie: "owned=PRIVATE-COOKIE"}}
	_, _, err := client.call(context.Background(), Map{"path": "/", "parser": "None", "headers": Map{"Authorization": "Bearer PRIVATE-AUTH", "X-Owned-Secret": "PRIVATE-EXTRA"}}, nil, nil)
	if err != nil || checked.Load() != 1 || leaked.Load() {
		t.Fatal("later redirect restored original headers", leaked.Load(), err)
	}
}

func TestSharedCookieJarCannotBroadenExplicitAccountCredentials(t *testing.T) {
	var received atomic.Int32
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		if r.Header.Get("Cookie") != "" {
			leaked.Store(true)
		}
		fmt.Fprint(w, "owned")
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer origin.Close()
	jar, _ := cookiejar.New(nil)
	otherURL, _ := url.Parse(other.URL)
	jar.SetCookies(otherURL, []*http.Cookie{{Name: "ambient", Value: "PRIVATE-JAR-COOKIE", Path: "/"}})
	client := Client{Config: Config{Domain: origin.URL, AdditionalDomains: []string{other.URL}}, Account: Account{Endpoint: origin.URL, Cookie: "owned=PRIVATE-COOKIE"}, HTTP: &http.Client{Jar: jar}}
	_, _, err := client.call(context.Background(), Map{"path": "/", "parser": "None"}, nil, nil)
	if err != nil || received.Load() != 1 || leaked.Load() {
		t.Fatal("ambient Jar supplied cross-origin account cookie", leaked.Load(), err)
	}
	if len(jar.Cookies(otherURL)) != 1 {
		t.Fatal("client mutated caller's cookie jar")
	}
}

func TestMalformedAndAmbiguousHeaderDefinitionsAreRejectedWithoutHTTP(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1); fmt.Fprint(w, "owned") }))
	defer server.Close()
	client := Client{Config: Config{Domain: server.URL}, Account: Account{Endpoint: server.URL}}
	for _, rule := range []Map{
		{"headers": "Authorization: secret"},
		{"headers": Map{"Host": "other.invalid"}},
		{"headers": Map{"X-Owned": "one", "x-owned": "two"}},
		{"headers": Map{"X-Owned": []any{"one", "two"}}},
		{"required_headers": "Authorization"},
		{"required_headers": []any{12}},
		{"required_headers": []any{"Authorization", "authorization"}},
		{"headers": Map{"Authorization": "Bearer {missing}"}, "required_headers": []any{"Authorization"}},
	} {
		rule["path"] = "/"
		rule["parser"] = "None"
		_, _, err := client.call(context.Background(), rule, nil, nil)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("unsafe header rule accepted or leaked a value")
		}
	}
	if received.Load() != 0 {
		t.Fatal("unsafe header definition reached HTTP", received.Load())
	}
}

func TestRequiredAPIHeaderStaysBoundToDeclaredRequestOrigin(t *testing.T) {
	var received atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1); fmt.Fprint(w, "owned") }))
	defer other.Close()
	client := Client{Config: Config{Domain: "https://owned.invalid", AdditionalDomains: []string{other.URL}}, Account: Account{Endpoint: "https://owned.invalid", APIKey: "PRIVATE-KEY"}}
	_, _, err := client.call(context.Background(), Map{"path": other.URL, "parser": "None", "required_headers": []any{"x-api-key"}}, nil, nil)
	if !errors.Is(err, ErrConfig) || received.Load() != 0 {
		t.Fatal("initial absolute URL received a different origin's API key", received.Load(), err)
	}
}

func TestHeaderCompatibilityReflectsDeclarationsWithoutInventingBootstrap(t *testing.T) {
	base := Config{ID: "owned_headers", Schema: "NexusPHP", Requests: map[string]Map{"search": {"parser": "CssSelector", "path": "/", "required_headers": []any{"Authorization"}}}}
	if Compatibility(base)["state"] != "partial" {
		t.Fatal("missing authorization producer was classified supported")
	}
	base.Requests["search"]["headers"] = Map{"Authorization": "Bearer owned-declaration"}
	if Compatibility(base)["state"] != "parser_supported" {
		t.Fatal("explicit required header declaration was classified unsupported")
	}
	base.Requests["search"]["headers"] = Map{"Authorization": "one", "authorization": "two"}
	if Compatibility(base)["state"] != "partial" {
		t.Fatal("ambiguous case duplicate was hidden by static audit")
	}
	base.Requests["search"]["headers"] = Map{"Authorization": "owned"}
	base.Requests["search"]["required_headers"] = "Authorization"
	if Compatibility(base)["state"] != "partial" {
		t.Fatal("invalid required-list shape was silently accepted")
	}
}

func TestInitialAdditionalOriginCannotReceiveBaseRequestHeadersOrBody(t *testing.T) {
	var received atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1); fmt.Fprint(w, "owned") }))
	defer other.Close()
	client := Client{Config: Config{Domain: "https://owned.invalid", AdditionalDomains: []string{other.URL}}, Account: Account{Endpoint: "https://owned.invalid"}}
	for _, rule := range []Map{
		{"headers": Map{"Authorization": "Bearer PRIVATE-AUTH"}},
		{"headers": Map{"X-Owned-Secret": "PRIVATE-EXTRA"}},
		{"method": "POST", "body": Map{"secret": "PRIVATE-BODY"}},
	} {
		rule["path"] = other.URL
		rule["parser"] = "None"
		_, _, err := client.call(context.Background(), rule, nil, nil)
		if !errors.Is(err, ErrConfig) {
			t.Fatal("initial additional-origin target accepted base credentials or write body", err)
		}
	}
	if received.Load() != 0 {
		t.Fatal("initial target received private base request material", received.Load())
	}
}
