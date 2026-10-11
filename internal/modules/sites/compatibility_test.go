package sites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/text/encoding/simplifiedchinese"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCasesRetainDeclarationOrderAndFalsyValues(t *testing.T) {
	raw := []byte(`{"id":"owned","domain":"https://owned.invalid","reuse_schema":"NexusPHP","requests":{"search":{"parser":"CssSelector","fields":{"factor":{"case":{".z-free":0,".a-half":0.5,"*":1}},"login":{"case":{".z-free":false,"*":true}}}}}}`)
	config, err := Decode(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.Schema != "NexusPHP" {
		t.Fatal("reuse schema missing")
	}
	document, err := parseDocument([]byte(`<span class="z-free a-half">both</span>`), "CssSelector")
	if err != nil {
		t.Fatal(err)
	}
	fields, err := mapFields(document, object(config.Requests["search"]["fields"]), nil, nil)
	if err != nil || text(fields["factor"]) != "0" || fields["login"] != false {
		t.Fatal(fields, err)
	}
	canonical, err := canonicalDefinitionJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Marshal with sorted keys simulates jsonb reordering; only branch arrays keep priority.
	var stored Map
	if json.Unmarshal(canonical, &stored) != nil {
		t.Fatal("canonical JSON invalid")
	}
	canonical, _ = json.Marshal(stored)
	reloaded, err := DecodeStored(canonical, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	again, err := mapFields(document, object(reloaded.Requests["search"]["fields"]), nil, nil)
	if err != nil || text(again["factor"]) != "0" {
		t.Fatal(again, err)
	}
	if _, err = DecodeStored(raw, nil); !errors.Is(err, ErrCaseOrderUnavailable) {
		t.Fatal("legacy priority silently guessed", err)
	}
}

func TestCanonicalizationDoesNotTransformJSONPayloadsOrLookupFilters(t *testing.T) {
	raw := []byte(`{"id":"owned","domain":"https://owned.invalid","requests":{"search":{"body":{"fields":{"case":{"a":1,"b":2}}},"fields":{"labels":{"selector":"codes","array":true,"filters":[{"name":"case","args":{"a":"A","b":"B"}}]},"title":{"any":[{"case":{".first":"First","*":"Default"}}]}}}}}`)
	canonical, err := canonicalDefinitionJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	config, err := DecodeStored(canonical, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := object(config.Requests["search"]["body"])
	if len(object(object(body["fields"])["case"])) != 2 {
		t.Fatal("request data transformed into selector rules")
	}
	labels := object(object(config.Requests["search"]["fields"])["labels"])
	if len(object(object(slice(labels["filters"])[0])["args"])) != 2 {
		t.Fatal("lookup-filter object changed")
	}
	alternatives := slice(object(object(config.Requests["search"]["fields"])["title"])["any"])
	if _, ok := object(alternatives[0])["case"].([]any); !ok {
		t.Fatal("nested selector cases not ordered")
	}
}

func TestCaseOverrideIsExclusiveAndBadJSONRejected(t *testing.T) {
	common := []byte(`{"requests":{"search":{"fields":{"factor":{"selector":".normal","any":[{"selector":".other"}]}}}}}`)
	raw := []byte(`{"id":"owned","domain":"https://owned.invalid","requests":{"search":{"fields":{"factor":{"case":{".free":0,"*":1}}}}}}`)
	config, err := Decode(raw, common)
	if err != nil {
		t.Fatal(err)
	}
	rule := object(object(config.Requests["search"]["fields"])["factor"])
	if _, ok := rule["selector"]; ok {
		t.Fatal("case inherited conflicting selector")
	}
	if _, ok := rule["any"]; ok {
		t.Fatal("case inherited conflicting alternatives")
	}
	for _, bad := range []string{`{"id":"a","id":"b"}`, `{} {}`, `[]`, `{"requests":{"search":{"fields":{"title":{"case":true}}}}}`, `{"requests":{"search":{"fields":{"title":{"case":[{"selector":"a","value":1},{"selector":"a","value":2}]}}}}}`} {
		if _, err := decodeDefinition([]byte(bad), false); err == nil {
			t.Fatal("malformed/ambiguous configuration accepted")
		}
	}
}

func TestArrayFiltersDependenciesAndCycles(t *testing.T) {
	document, err := parseDocument([]byte(`{"codes":[" a ","b","other"],"title":"Album","suffix":" - Group"}`), "JsonPath")
	if err != nil {
		t.Fatal(err)
	}
	rules := Map{"labels": Map{"selector": "codes", "array": true, "filters": []any{Map{"name": "strip"}, Map{"name": "case", "args": Map{"a": "A", "b": "B"}}}}, "title": Map{"selector": "title", "filters": []any{Map{"name": "append_field_right", "args": "z_suffix"}}}, "z_suffix": Map{"selector": "suffix"}, "count": Map{"selector": "codes", "array": true, "filters": []any{Map{"name": "count"}}}}
	fields, err := mapFields(document, rules, nil, nil)
	if err != nil || fields["title"] != "Album - Group" || fields["count"] != 3 {
		t.Fatal(fields, err)
	}
	labels := slice(fields["labels"])
	if len(labels) != 3 || labels[0] != "A" || labels[1] != "B" || labels[2] != nil {
		t.Fatal("array transformed as a string", labels)
	}
	rules["z_suffix"] = Map{"selector": "suffix", "filters": []any{Map{"name": "append_field_right", "args": "title"}}}
	if _, err = mapFields(document, rules, nil, nil); !errors.Is(err, ErrConfig) {
		t.Fatal("dependency cycle accepted", err)
	}
	delete(rules, "z_suffix")
	if _, err = mapFields(document, rules, nil, nil); !errors.Is(err, ErrConfig) {
		t.Fatal("missing dependency silently ignored", err)
	}
}

func TestJSONBodyQueryAndFormAreSeparateAndSubstitutionIsSinglePass(t *testing.T) {
	keyword := `quoted "title" {passkey} & value`
	vars := map[string]string{"keyword": keyword, "page": "2", "passkey": "fixture-secret-not-expanded"}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("trace") != "fixed" || r.URL.Query().Get("q") != keyword {
			t.Error("query lost or recursively expanded")
		}
		if r.URL.Path == "/json" {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("wrong JSON content type")
			}
			var body Map
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["keyword"] != keyword || object(body["page"])["current"] != "2" || numeric(object(body["page"])["size"]) != 40 || body["enabled"] != true {
				t.Error("JSON types or placeholders corrupted")
			}
		} else {
			r.ParseForm()
			if r.PostForm.Get("title") != keyword || r.PostForm.Get("q") != "" {
				t.Error("query leaked into form body")
			}
		}
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	client := Client{Config: Config{Domain: server.URL}, Account: Account{Endpoint: server.URL}}
	for _, kind := range []string{"json", "form"} {
		rule := Map{"method": "POST", "parser": "JsonPath", "path": "/" + kind + "?trace=fixed", "params": Map{"q": "{keyword}"}}
		if kind == "json" {
			rule["body"] = Map{"keyword": "{keyword}", "page": Map{"current": "{page}", "size": 40}, "enabled": true}
		} else {
			rule["form_data"] = Map{"title": []any{"{keyword}"}}
		}
		if _, _, err := client.call(context.Background(), rule, vars, nil); err != nil {
			t.Fatal(err)
		}
	}
	if requests != 2 {
		t.Fatal("unexpected requests")
	}
	if substitute("{keyword}", vars) != keyword {
		t.Fatal("recursive credential expansion")
	}
	if _, _, err := client.call(context.Background(), Map{"path": "/json", "params": Map{"q": Map{"nested": "invalid"}}}, vars, nil); !errors.Is(err, ErrConfig) {
		t.Fatal("invalid query type accepted", err)
	}
}

func TestGroupedSearchKeepsParentFieldsAndChildIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/profile":
			fmt.Fprint(w, `{"login":true}`)
		case "/search":
			fmt.Fprint(w, `{"groups":[{"group":"Artist","name":"Record","details":"/group/7","codes":["lossless","unknown","lossless"],"torrents":[{"id":"11","size":1024,"seeds":5,"factor":"UNKNOWN","hr":"unknown"},{"id":"12","size":2048,"seeds":7,"factor":0,"hr":2}]}]}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	raw := []byte(fmt.Sprintf(`{"id":"owned_groups","domain":%q,"reuse_schema":"NexusPHP","requests":{"user_basic_info":{"path":"/profile","parser":"JsonPath","fields":{"is_login":{"selector":"login"}}},"search":{"path":"/search","parser":"JsonPath","list":{"selector":"groups"},"fields":{"title":{"selector":"group","filters":[{"name":"append_right","args":" - "},{"name":"append_field_right","args":"z_name"}]},"z_name":{"selector":"name"},"details":{"selector":"details"},"labels":{"selector":"codes","array":true,"filters":[{"name":"case","args":{"lossless":"Lossless"}}]},"ignored_date":{"selector":"date","filters":[{"name":"timezone","args":["+19"]}]}},"flat_map_fields":{"selector":"torrents","fields":{"id":{"selector":"id"},"size":{"selector":"size"},"seeders":{"selector":"seeds"},"downloadvolumefactor":{"selector":"factor"},"hr_days":{"selector":"hr"}}}}}}`, server.URL))
	config, err := Decode(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := Client{Config: config, Account: Account{Endpoint: server.URL}}
	resources, err := client.Search(context.Background(), SearchQuery{Keyword: "Record", Page: 1})
	if err != nil || len(resources) != 2 {
		t.Fatal(err, len(resources))
	}
	first, second := resources[0], resources[1]
	if first.TorrentID != "11" || second.TorrentID != "12" || first.Title != "Artist - Record" || second.Title != first.Title || first.Size != 1024 || second.Size != 2048 {
		t.Fatal("parent/child data lost", resources)
	}
	if first.DownloadFactor != nil || first.HR != nil {
		t.Fatal("unknown promotion or HR inferred as free/no-HR")
	}
	if second.DownloadFactor == nil || *second.DownloadFactor != 0 || second.HR == nil || !*second.HR {
		t.Fatal("known child values lost")
	}
	if len(first.Labels) != 1 || first.Labels[0] != "Lossless" || first.DetailsURL != server.URL+"/group/7" {
		t.Fatal("labels or parent URL lost")
	}
	compatibility := Compatibility(config)
	if compatibility["state"] != "partial" {
		t.Fatal("unsupported timezone mislabeled supported")
	}
}

func TestTimestampAlternativesAreHostIndependentAndSizesBounded(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("fixture", 8*3600)
	defer func() { time.Local = old }()
	value, err := filter("2026-10-1008:30:00", Map{"name": "timestamp", "args": []any{"Jan 02 2006", "2006-01-0215:04:05"}}, nil)
	expected := time.Date(2026, 10, 10, 8, 30, 0, 0, time.UTC).UnixMilli()
	if err != nil || value != expected {
		t.Fatal("alternative layout or host-independent time failed", value, err)
	}
	if _, err := filter(value, Map{"name": "timezone", "args": []any{"+8"}}, nil); !errors.Is(err, ErrConfig) {
		t.Fatal("timezone no-op accepted")
	}
	for _, input := range []string{"8 EiB", "9999999999999999999999999 GiB"} {
		if _, err := filter(input, Map{"name": "byte_size"}, nil); !errors.Is(err, ErrConfig) {
			t.Fatal("overflow accepted")
		}
	}
	if value, err := filter("1.5\u00a0GiB", Map{"name": "byte_size"}, nil); err != nil || value != int64(1610612736) {
		t.Fatal(value, err)
	}
	if value, err := filter("1024", Map{"name": "byte_size"}, nil); err != nil || value != int64(1024) {
		t.Fatal(value, err)
	}
	for _, input := range []any{nil, true, "UNKNOWN", "NaN", "Inf", math.NaN()} {
		if _, valid := checkedNumber(input); valid {
			t.Fatal("non-numeric value accepted")
		}
	}
	if value, valid := checkedInteger(json.Number("9223372036854775807"), 63); !valid || value != math.MaxInt64 {
		t.Fatal("integer precision lost")
	}
	if _, valid := checkedInteger("2147483648", 31); valid {
		t.Fatal("PostgreSQL integer overflow accepted")
	}
}

func TestHTTPFailuresRemainRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, "fixture-private-response")
	}))
	defer server.Close()
	client := Client{Config: Config{Domain: server.URL}, Account: Account{Endpoint: server.URL, Cookie: "fixture-cookie"}}
	_, _, err := client.call(context.Background(), Map{"path": "/search?passkey=fixture-passkey"}, nil, nil)
	if !errors.Is(err, ErrRequest) || strings.Contains(err.Error(), "fixture-") {
		t.Fatal("request failure contains private data")
	}
}

func TestExplicitLegacyEncodingDecodesHTMLTitles(t *testing.T) {
	encoded, err := simplifiedchinese.GBK.NewEncoder().String("自有测试电影")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<a class=title>"+encoded+"</a>")
	}))
	defer server.Close()
	client := Client{Config: Config{Domain: server.URL, Encoding: "GBK"}, Account: Account{Endpoint: server.URL}}
	raw, mode, err := client.call(context.Background(), Map{"path": "/search", "parser": "CssSelector"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	document, err := parseDocument(raw, mode)
	if err != nil {
		t.Fatal(err)
	}
	value, err := document.field("title", Map{"selector": "a.title"}, nil, nil)
	if err != nil || value != "自有测试电影" {
		t.Fatal("configured encoding ignored", err)
	}
}

func TestTrimmingPreservesTypedFilterOutputs(t *testing.T) {
	document, _ := parseDocument([]byte("<p>2026-10-10</p>"), "CssSelector")
	value, err := document.field("date", Map{"selector": "p", "trim_chars": true, "filters": []any{Map{"name": "timestamp"}}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := value.(int64); !ok {
		t.Fatal("timestamp was stringified by trimming")
	}
	array := slice(trimFieldStrings([]any{" a ", json.Number("0"), false}, strings.TrimSpace))
	if array[0] != "a" || array[1] != json.Number("0") || array[2] != false {
		t.Fatal("trim changed typed values")
	}
}
