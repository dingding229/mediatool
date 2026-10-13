package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStandaloneDefinitionExtractionAndLegacyInheritanceRejected(t *testing.T) {
	raw := []byte(`{"templateVersion":1,"id":"fixture","name":"Fixture","domain":"https://site.example.com","schema":"NexusPHP","requests":{"search":{"parser":"CssSelector","path":"/torrents.php","list":{"selector":"tr.resource"},"fields":{"id":{"selector":"a","attribute":"data-id"},"title":{"selector":"a.title","attribute":"title"},"size":{"selector":"td.size","filters":[{"name":"byte_size"}]}}}}}`)
	config, err := Decode(raw, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	d, err := parseDocument([]byte(`<table><tr class="resource"><td><a class="title" data-id="123" title="Movie 2026">text</a></td><td class="size">1.5 GiB</td></tr></table>`), "CssSelector")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := d.values("tr.resource", nil)
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	fields, err := mapFields(d, object(config.Requests["search"]["fields"]), rows[0], nil)
	if err != nil || text(fields["title"]) != "Movie 2026" || int64(numeric(fields["size"])) != 1610612736 {
		t.Fatal(fields, err)
	}
	if _, err = Decode(raw, []byte(`{"requests":{}}`)); err == nil {
		t.Fatal("common inheritance accepted")
	}
	legacy := []byte(`{"templateVersion":1,"id":"api","domain":"https://api.example.com","requests":{"search":{"disabled_extends":{"params":true}}}}`)
	if _, err = Decode(legacy, []byte(`{}`)); err == nil {
		t.Fatal("legacy request accepted")
	}
}
func TestAPIAuthenticationSearchAndDownload(t *testing.T) {
	torrent := "d4:infod4:name4:test6:pieces20:" + strings.Repeat("a", 20) + "ee"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/profile":
			if r.Header.Get("x-api-key") != "api-secret" {
				w.WriteHeader(403)
				return
			}
			fmt.Fprint(w, `{"data":{"username":"test-user"}}`)
		case "/search":
			var input map[string]any
			json.NewDecoder(r.Body).Decode(&input)
			if input["keyword"] != "Movie" {
				t.Error("keyword body missing")
			}
			fmt.Fprint(w, `{"data":{"data":[{"id":"1","name":"Movie","size":"1024","status":{"seeders":12,"discount":"FREE"}}]}}`)
		case "/token":
			fmt.Fprintf(w, `{"data":"http://%s/download?id=1&passkey=secret"}`, r.Host)
		case "/download":
			w.Header().Set("Content-Type", "application/x-bittorrent")
			fmt.Fprint(w, torrent)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	config := Config{ID: "mteam", Domain: server.URL, API: server.URL, Schema: "mTorrent", Requests: map[string]Map{
		"profile":      {"parser": "JsonPath", "method": "POST", "path": "/profile", "use_api": true, "required_headers": []any{"x-api-key"}, "fields": Map{"username": Map{"selector": "data.username"}}},
		"search":       {"parser": "JsonPath", "method": "POST", "path": "/search", "use_api": true, "required_headers": []any{"x-api-key"}, "list": Map{"selector": "data.data"}, "fields": Map{"id": Map{"selector": "id"}, "title": Map{"selector": "name"}, "size": Map{"selector": "size"}, "seeders": Map{"selector": "status.seeders"}, "downloadvolumefactor": Map{"selector": "status.discount", "filters": []any{Map{"name": "case", "args": Map{"FREE": "0"}}}}}},
		"gen_dl_token": {"parser": "JsonPath", "method": "POST", "path": "/token", "use_api": true, "required_headers": []any{"x-api-key"}, "form_data": Map{"id": []any{"{id}"}}, "fields": Map{"url": Map{"selector": "data"}}},
	}}
	client := Client{Config: config, Account: Account{Endpoint: server.URL, APIKey: "api-secret", UserID: "1"}}
	resources, err := client.Search(context.Background(), SearchQuery{Keyword: "Movie", Page: 1})
	if err != nil || len(resources) != 1 || resources[0].DownloadFactor == nil || *resources[0].DownloadFactor != 0 {
		t.Fatal(err, resources)
	}
	raw, err := client.Torrent(context.Background(), "1", "")
	if err != nil || string(raw) != torrent {
		t.Fatal("torrent bytes modified", err)
	}
}
