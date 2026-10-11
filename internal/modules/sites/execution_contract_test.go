package sites

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNonexecutableRequestsAreRejectedBeforeHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); fmt.Fprint(w, `<p>authenticated</p>`) }))
	defer server.Close()
	client := Client{Config: Config{Domain: server.URL}, Account: Account{Endpoint: server.URL, Cookie: "owned=PRIVATE-COOKIE"}}
	cases := []struct {
		name string
		rule Map
	}{
		{"browser", Map{"chrome": true}}, {"browser string", Map{"chrome": "true"}}, {"browser null", Map{"chrome": nil}},
		{"disabled", Map{"disabled": true}}, {"disabled string", Map{"disabled": "false"}}, {"disabled null", Map{"disabled": nil}},
		{"unknown parser", Map{"parser": "PRIVATE-UNSUPPORTED-PARSER"}}, {"parser number", Map{"parser": 42}},
		{"parser null", Map{"parser": nil}}, {"parser object", Map{"parser": Map{"secret": "PRIVATE-PARSER-VALUE"}}},
		{"whitespace parser", Map{"parser": " JsonPath "}}, {"missing rule", nil}, {"empty rule", Map{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := clone(tc.rule)
			if len(tc.rule) > 0 {
				rule["path"] = "/"
			}
			sentinel, _ := url.Parse("https://unchanged.invalid")
			target := sentinel
			raw, mode, err := client.callWithTarget(context.Background(), rule, nil, nil, &target)
			if !errors.Is(err, ErrConfig) || len(raw) != 0 || mode != "" || target != sentinel || requests.Load() != 0 {
				t.Fatal("unsupported declaration reached HTTP or changed result", requests.Load(), err)
			}
			if strings.Contains(err.Error(), "PRIVATE-") {
				t.Fatal("configuration content exposed")
			}
		})
	}
}

func TestUnknownDocumentParserDoesNotFallbackToHTMLOrCSS(t *testing.T) {
	for _, mode := range []string{"PRIVATE-UNKNOWN-PARSER", " JsonPath ", "json", "html", "None", "none"} {
		if _, err := parseDocument([]byte(`<p class="login">authenticated</p>`), mode); !errors.Is(err, ErrConfig) {
			t.Fatal("unknown parser fell back to HTML", err)
		}
		doc, err := parseDocument([]byte(`<p>owned</p>`), "CssSelector")
		if err != nil {
			t.Fatal(err)
		}
		doc.mode = mode
		if _, err = doc.values("p", nil); !errors.Is(err, ErrConfig) {
			t.Fatal("unknown selector mode fell back to CSS", err)
		}
	}
}

func TestSupportedParserDefaultsAndAliasesRemainExecutable(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); fmt.Fprint(w, "owned") }))
	defer server.Close()
	client := Client{Config: Config{Domain: server.URL}, Account: Account{Endpoint: server.URL}}
	for _, mode := range []string{"", "CssSelector", "cssselector", "cSsSeLeCtOr", "XPath", "xpath", "JsonPath", "jsonpath", "XML", "Xml", "xml", "None", "none"} {
		rule := Map{"path": "/", "parser": mode, "chrome": false, "disabled": false}
		raw, actual, err := client.call(context.Background(), rule, nil, nil)
		if err != nil || string(raw) != "owned" || actual != mode {
			t.Fatal("supported declaration changed", mode, err)
		}
	}
	if _, _, err := client.call(context.Background(), Map{"path": "/"}, nil, nil); err != nil {
		t.Fatal("omitted parser default changed", err)
	}
	if requests.Load() != 14 {
		t.Fatal("valid requests not executed")
	}
	for _, sample := range []struct{ mode, body, selector, want string }{
		{"", `<p>owned</p>`, "p", "owned"}, {"CssSelector", `<p>owned</p>`, "p", "owned"},
		{"XPath", `<p>owned</p>`, "//p", "owned"}, {"JsonPath", `{"name":"owned"}`, "name", "owned"},
		{"XML", `<root><name>owned</name></root>`, "//name", "owned"},
	} {
		doc, err := parseDocument([]byte(sample.body), sample.mode)
		if err != nil {
			t.Fatal(err)
		}
		value, err := doc.field("name", Map{"selector": sample.selector}, nil, nil)
		if err != nil || value != sample.want {
			t.Fatal("supported parsing changed", sample.mode, err)
		}
	}
}

func TestSearchPreflightsExecutionContractBeforeLogin(t *testing.T) {
	for _, override := range []Map{{"chrome": true}, {"disabled": true}, {"parser": "PRIVATE-UNKNOWN"}, {"parser": nil}, {"parser": "None"}, {"chrome": "false"}} {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			fmt.Fprint(w, `{"login":true,"items":[]}`)
		}))
		client := Client{Config: Config{Domain: server.URL, Schema: "NexusPHP", Requests: map[string]Map{
			"user_basic_info": {"path": "/profile", "parser": "JsonPath", "fields": Map{"is_login": Map{"selector": "login"}}},
			"search":          {"path": "/search", "parser": "JsonPath", "list": Map{"selector": "items"}},
		}}, Account: Account{Endpoint: server.URL, Cookie: "owned=PRIVATE-COOKIE"}}
		for key, value := range override {
			client.Config.Requests["search"][key] = value
		}
		result, err := client.SearchAll(context.Background(), SearchQuery{Keyword: "Owned"})
		server.Close()
		if !errors.Is(err, ErrConfig) || requests.Load() != 0 || result.Complete || len(result.Resources) != 0 {
			t.Fatal("invalid search triggered login or completion", requests.Load(), err)
		}
	}
}

func TestBrowserLoginCannotBeReportedAsVerified(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); fmt.Fprint(w, `{"login":true}`) }))
	defer server.Close()
	client := Client{Config: Config{Domain: server.URL, Schema: "NexusPHP", Requests: map[string]Map{
		"user_basic_info": {"path": "/profile", "parser": "JsonPath", "chrome": true, "fields": Map{"is_login": Map{"selector": "login"}}},
	}}, Account: Account{Endpoint: server.URL}}
	if err := client.Test(context.Background()); !errors.Is(err, ErrConfig) || requests.Load() != 0 {
		t.Fatal("ordinary HTTP satisfied browser login", requests.Load(), err)
	}
}

func TestMissingDisabledOrUnsupportedDownloadTokenDoesNotCallRoot(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprintf(w, `{"url":"http://%s/download"}`, r.Host)
	}))
	defer server.Close()
	for _, rule := range []Map{nil, {}, {"path": "/token", "disabled": true}, {"path": "/token", "chrome": true}, {"path": "/token", "parser": "PRIVATE-UNKNOWN"}, {"path": "/token", "parser": "None"}} {
		client := Client{Config: Config{Domain: server.URL, Schema: "mTorrent", Requests: map[string]Map{"gen_dl_token": rule}}, Account: Account{Endpoint: server.URL}}
		if _, err := client.Torrent(context.Background(), "42", server.URL+"/download"); !errors.Is(err, ErrConfig) || requests.Load() != 0 {
			t.Fatal("nonexecutable token request called root/download", requests.Load(), err)
		}
	}
}

func TestRSSCannotBypassDeclaredBrowserOrExecutionFlags(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `<rss version="2.0"><channel></channel></rss>`)
	}))
	defer server.Close()
	for _, rule := range []Map{{"chrome": true}, {"chrome": "false"}, {"parser": "PRIVATE-UNKNOWN"}, {"disabled": "false"}} {
		client := Client{Config: Config{Domain: server.URL, Requests: map[string]Map{"rss": rule}}, Account: Account{Endpoint: server.URL}}
		if _, err := client.RSS(context.Background(), server.URL+"/feed"); !errors.Is(err, ErrConfig) || requests.Load() != 0 {
			t.Fatal("raw feed request bypassed declaration", requests.Load(), err)
		}
	}
	// A manually configured feed does not require an invented rss request rule.
	client := Client{Config: Config{Domain: server.URL}, Account: Account{Endpoint: server.URL}}
	items, err := client.RSS(context.Background(), server.URL+"/feed")
	if err != nil || len(items) != 0 || requests.Load() != 1 {
		t.Fatal("manual feed contract changed", err)
	}
}

func TestCompatibilityIssuesAreTypedAndDoNotScanPayloadData(t *testing.T) {
	config := Config{Schema: "NexusPHP", Requests: map[string]Map{"search": {
		"parser": "JsonPath", "body": Map{"filters": []any{Map{"name": "PRIVATE-PAYLOAD-FILTER"}}, "fields": Map{"ignored": Map{"filters": []any{Map{"name": "timezone"}}}}},
		"fields": Map{"id": Map{"selector": "id"}},
	}}}
	if c := Compatibility(config); c["state"] != "parser_supported" {
		t.Fatal("request data interpreted as parser instructions")
	}
	config.Requests["search"]["parser"] = "PRIVATE-UNKNOWN-PARSER"
	config.Requests["search"]["chrome"] = true
	config.Requests["search"]["fields"] = Map{"date": Map{"filters": []any{Map{"name": "timezone", "args": []any{"+19"}}, Map{"name": "timezone", "args": []any{"+8"}}}}}
	compatibility := Compatibility(config)
	codes, ok := compatibility["issueCodes"].([]string)
	if !ok || !reflect.DeepEqual(codes, []string{"browser_execution_required", "timezone_semantics_unverified", "unsupported_parser"}) {
		t.Fatal("safe issue codes missing, unstable or repeated", codes)
	}
	if compatibility["accountVerified"] != false || compatibility["state"] != "partial" {
		t.Fatal("static evidence mislabeled")
	}
	for _, note := range compatibility["notes"].([]string) {
		if strings.Contains(note, "PRIVATE-") {
			t.Fatal("parser declaration leaked into report")
		}
	}
}

func TestCompatibilityInspectsOnlyExecutableFieldFilters(t *testing.T) {
	literal := Map{"filters": []any{Map{"name": "PRIVATE-LITERAL-FILTER"}}}
	config := Config{Schema: "NexusPHP", Requests: map[string]Map{"search": {
		"parser": "JsonPath", "params": Map{"filters": "ordinary-data"},
		"fields": Map{
			"literal": Map{"default": literal, "filters": []any{Map{"name": "constant", "args": literal}}},
			"lookup":  Map{"filters": []any{Map{"name": "case", "args": Map{"owned": literal}}}},
			"branch":  Map{"case": []any{Map{"selector": "*", "value": literal}}},
		},
	}}}
	if Compatibility(config)["state"] != "parser_supported" {
		t.Fatal("literal values interpreted as extraction rules")
	}
	config.Requests["search"]["fields"].(Map)["real"] = Map{"any": []any{Map{"filters": []any{Map{"name": "regex", "args": "(?<=unsupported)PCRE"}}}}}
	config.Requests["search"]["flat_map_fields"] = Map{"fields": Map{"date": Map{"filters": []any{Map{"name": "timezone", "args": []any{"+19"}}}}}}
	if got := Compatibility(config)["issueCodes"]; !reflect.DeepEqual(got, []string{"timezone_semantics_unverified", "unsupported_regexp"}) {
		t.Fatal("nested executable filters missed", got)
	}
}

func TestCompatibilityAndRuntimeRejectMalformedExecutionDeclarations(t *testing.T) {
	for _, sample := range []struct {
		rule Map
		code string
	}{
		{Map{"parser": nil}, "unsupported_parser"}, {Map{"parser": 42}, "unsupported_parser"},
		{Map{"parser": "None"}, "raw_parser_for_document"}, {Map{"chrome": "false"}, "invalid_execution_flags"},
		{Map{"disabled": "false"}, "invalid_execution_flags"}, {Map{"disabled": true}, "request_disabled"},
	} {
		rule := clone(sample.rule)
		rule["path"] = "/owned"
		config := Config{Schema: "NexusPHP", Requests: map[string]Map{"search": rule}}
		c := Compatibility(config)
		if !reflect.DeepEqual(c["issueCodes"], []string{sample.code}) || c["state"] != "partial" || validateDocumentRequest(rule) == nil {
			t.Fatal("static/runtime contract diverged", sample.code)
		}
	}
}
