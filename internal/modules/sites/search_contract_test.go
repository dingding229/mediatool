package sites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func requestSearchClient(t *testing.T, searchRule Map, handler http.HandlerFunc) Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/profile" {
			fmt.Fprint(w, `{"data":{"username":"owned"}}`)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	searchRule = clone(searchRule)
	searchRule["path"] = "/search"
	searchRule["method"] = "POST"
	searchRule["parser"] = "JsonPath"
	searchRule["list"] = Map{"selector": "data.items"}
	searchRule["fields"] = Map{"id": Map{"selector": "id"}, "title": Map{"selector": "name"}}
	return Client{Config: Config{ID: "owned_contract", Schema: "mTorrent", Domain: server.URL, API: server.URL, Requests: map[string]Map{"profile": {"path": "/profile", "parser": "JsonPath", "fields": Map{"username": Map{"selector": "data.username"}}}, "search": searchRule}}, Account: Account{Endpoint: server.URL, UserID: "1", APIKey: "PRIVATE-FIXTURE-KEY"}}
}

func TestMTorrentExplicitSearchBodyIsNotOverwrittenByDefaultJSON(t *testing.T) {
	keyword := `literal{page}"quoted`
	received := false
	client := requestSearchClient(t, Map{"body": Map{"mode": "owned-custom-mode", "keyword": "{keyword}", "pageNumber": "{page}", "pageSize": 7, "categories": []any{401}, "promotion": Map{"free": true}}}, func(w http.ResponseWriter, r *http.Request) {
		var body Map
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		categories, _ := body["categories"].([]any)
		if body["mode"] != "owned-custom-mode" || body["keyword"] != keyword || body["pageNumber"] != "2" || body["pageSize"] != float64(7) || len(categories) != 1 || categories[0] != float64(401) || object(body["promotion"])["free"] != true {
			w.WriteHeader(400)
			return
		}
		received = true
		fmt.Fprint(w, `{"data":{"items":[{"id":"42","name":"Owned Movie"}]}}`)
	})
	resources, err := client.Search(context.Background(), SearchQuery{Keyword: keyword, Page: 2})
	if err != nil || !received || len(resources) != 1 {
		t.Fatal("explicit body/category/filters were ignored", err)
	}
}

func TestMTorrentExplicitFormSearchIsNotReplacedByJSON(t *testing.T) {
	received := false
	client := requestSearchClient(t, Map{"form_data": Map{"keyword": []any{"{keyword}"}, "page": []any{"{page}"}, "scope": []any{"owned"}}}, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			w.WriteHeader(400)
			return
		}
		if r.ParseForm() != nil || r.Form.Get("keyword") != "Owned" || r.Form.Get("page") != "3" || r.Form.Get("scope") != "owned" {
			w.WriteHeader(400)
			return
		}
		received = true
		fmt.Fprint(w, `{"data":{"items":[{"id":"42","name":"Owned"}]}}`)
	})
	_, err := client.Search(context.Background(), SearchQuery{Keyword: "Owned", Page: 3})
	if err != nil || !received {
		t.Fatal("explicit form transport replaced by hard-coded JSON", err)
	}
}

func TestExplicitEmptyFormNullBodyAndConflictingPayloadContracts(t *testing.T) {
	for _, entry := range []struct {
		rule              Map
		contentType, body string
	}{{Map{"body": nil}, "application/json", "null"}, {Map{"form_data": Map{}}, "application/x-www-form-urlencoded", ""}} {
		reader, contentType, err := requestPayload(entry.rule, nil, Map{"wrong": "default"})
		if err != nil || contentType != entry.contentType {
			t.Fatal("explicit empty payload ignored", err)
		}
		raw, err := io.ReadAll(reader)
		if err != nil || string(raw) != entry.body {
			t.Fatal("empty/null payload replaced by default", err)
		}
	}
	for _, rule := range []Map{{"body": Map{}, "form_data": Map{}}, {"form_data": nil}, {"form_data": "invalid"}, {"form_data": Map{"key": Map{"unsupported": "nested"}}}} {
		if _, _, err := requestPayload(rule, nil, Map{}); !errors.Is(err, ErrConfig) {
			t.Fatal("invalid/ambiguous payload accepted", err)
		}
	}
}

func TestSearchAllFollowsDeclaredPagesAndDeduplicatesWithoutGuessingShortLastPage(t *testing.T) {
	var pages []string
	client := requestSearchClient(t, Map{"body": Map{"keyword": "{keyword}", "page": "{page}"}}, func(w http.ResponseWriter, r *http.Request) {
		var body Map
		json.NewDecoder(r.Body).Decode(&body)
		page := text(body["page"])
		pages = append(pages, page)
		switch page {
		case "1":
			fmt.Fprint(w, `{"data":{"items":[{"id":"1","name":"Owned A"},{"id":"2","name":"Owned B"}]}}`)
		case "2":
			fmt.Fprint(w, `{"data":{"items":[{"id":"2","name":"duplicate"},{"id":"3","name":"Owned C"}]}}`)
		default:
			fmt.Fprint(w, `{"data":{"items":[]}}`)
		}
	})
	result, err := client.SearchAll(context.Background(), SearchQuery{Keyword: "Owned", Page: 1})
	if err != nil || !result.Complete || result.Pages != 3 || result.Reason != "empty_page" || len(result.Resources) != 3 || strings.Join(pages, ",") != "1,2,3" {
		t.Fatal("declared pages not collected correctly", result.Pages, len(result.Resources), err)
	}
	if result.Resources[1].Title != "Owned B" {
		t.Fatal("duplicate overwrote first-page evidence")
	}
}

func TestSearchAllBoundsRepeatedPageLimitsAndLaterFailure(t *testing.T) {
	for _, mode := range []string{"repeated", "limit", "failed"} {
		t.Run(mode, func(t *testing.T) {
			requests := 0
			client := requestSearchClient(t, Map{}, func(w http.ResponseWriter, r *http.Request) {
				requests++
				var body Map
				json.NewDecoder(r.Body).Decode(&body)
				id := requests
				if mode == "repeated" {
					id = 1
				}
				if mode == "failed" && requests == 2 {
					w.WriteHeader(503)
					return
				}
				fmt.Fprintf(w, `{"data":{"items":[{"id":"%d","name":"Owned"}]}}`, id)
			})
			result, err := client.SearchAll(context.Background(), SearchQuery{Keyword: "Owned", Page: 1})
			reason := map[string]string{"repeated": "repeated_page", "limit": "page_limit", "failed": "later_page_failed"}[mode]
			if err != nil || result.Complete || result.Reason != reason || len(result.Resources) == 0 || requests > SearchPageLimit {
				t.Fatal("bounded partial result incorrectly reported", mode, result.Reason, requests, err)
			}
			if mode == "limit" && (result.Pages != 5 || len(result.Resources) != 5) {
				t.Fatal("page limit not enforced")
			}
		})
	}
}

func TestSearchAllDoesNotInventPagingForDefinitionWithoutPageVariable(t *testing.T) {
	requests := 0
	client := requestSearchClient(t, Map{"body": Map{"keyword": "{keyword}", "offset": 0, "limit": 50}}, func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"data":{"items":[{"id":"42","name":"Owned"}]}}`)
	})
	result, err := client.SearchAll(context.Background(), SearchQuery{Keyword: "Owned"})
	if err != nil || requests != 1 || result.Pages != 1 || result.Reason != "single_page_contract" || result.Complete {
		t.Fatal("undefined offset/page contract was guessed", requests, err)
	}
}

func TestSearchJSONRequiresAnActualArrayAtDeclaredResultPath(t *testing.T) {
	for _, body := range []string{`{"message":"PRIVATE-FIXTURE-KEY"}`, `{"data":{"items":null}}`, `{"data":{"items":false}}`, `{"data":{"items":{"id":"42","name":"scalar"}}}`} {
		client := requestSearchClient(t, Map{}, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		_, err := client.Search(context.Background(), SearchQuery{Keyword: "Owned", Page: 1})
		if !errors.Is(err, ErrSearchResponse) || strings.Contains(err.Error(), "PRIVATE-FIXTURE-KEY") {
			t.Fatal("missing/non-array list treated as successful empty search", err)
		}
	}
}

func TestSearchAllResourceOverflowIsPartialNotSilentSuccess(t *testing.T) {
	client := requestSearchClient(t, Map{"body": Map{"keyword": "{keyword}"}}, func(w http.ResponseWriter, r *http.Request) {
		items := []Map{}
		for i := 0; i < 501; i++ {
			items = append(items, Map{"id": fmt.Sprint(i), "name": "Owned"})
		}
		json.NewEncoder(w).Encode(Map{"data": Map{"items": items}})
	})
	result, err := client.SearchAll(context.Background(), SearchQuery{Keyword: "Owned"})
	if err != nil || result.Complete || result.Reason != "resource_limit" || len(result.Resources) != 500 {
		t.Fatal("overflow silently discarded", len(result.Resources), err)
	}
}

func TestSearchDoesNotAcceptTrailingJSONOrLoseQueryParameters(t *testing.T) {
	requests := 0
	client := requestSearchClient(t, Map{"body": Map{"keyword": "{keyword}", "page": "{page}"}, "params": Map{"scope": []any{"owned-query"}}}, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("scope") != "owned-query" {
			w.WriteHeader(400)
			return
		}
		fmt.Fprint(w, `{"data":{"items":[]}} {"extra":"not-one-document"}`)
	})
	if _, err := client.Search(context.Background(), SearchQuery{Keyword: "Owned", Page: 1}); err == nil || requests != 1 {
		t.Fatal("trailing JSON ignored or query/body contract lost", err)
	}
}

func TestLaterAuthenticationFailureDiscardsUntrustedCollection(t *testing.T) {
	requests := 0
	client := requestSearchClient(t, Map{}, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 2 {
			w.WriteHeader(401)
			return
		}
		fmt.Fprint(w, `{"data":{"items":[{"id":"42","name":"Owned"}]}}`)
	})
	result, err := client.SearchAll(context.Background(), SearchQuery{Keyword: "Owned", Page: 1})
	if !errors.Is(err, ErrAuthentication) || len(result.Resources) != 0 {
		t.Fatal("revoked authentication advertised usable partial collection", err)
	}
}

func TestDeclaredUserAgentIsNotOverwrittenAndEmptyDeclarationIsExplicit(t *testing.T) {
	for _, agent := range []string{"Owned-Configured-UA/1", ""} {
		received := false
		client := requestSearchClient(t, Map{"headers": Map{"User-Agent": agent}, "body": Map{"keyword": "{keyword}"}}, func(w http.ResponseWriter, r *http.Request) {
			if r.UserAgent() != agent {
				w.WriteHeader(400)
				return
			}
			received = true
			fmt.Fprint(w, `{"data":{"items":[]}}`)
		})
		if _, err := client.Search(context.Background(), SearchQuery{Keyword: "Owned", Page: 1}); err != nil || !received {
			t.Fatal("declared UA replaced by program default", err)
		}
	}
}
