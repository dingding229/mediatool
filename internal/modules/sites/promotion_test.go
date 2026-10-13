package sites

import (
	"net/url"
	"testing"
)

func TestDeclaredPromotionNormalAndUnknownFallback(t *testing.T) {
	template, _ := GoTemplate("ubits")
	rule := template.Definition["requests"].(Map)["search"].(Map)
	for _, tc := range []struct {
		html   string
		parsed *float64
		want   *float64
	}{
		{`<tr><td>Owned normal</td></tr>`, nil, ptr(1)},
		{`<tr><td><img class="pro_free" alt="Free"></td></tr>`, ptr(0), ptr(0)},
		{`<tr><td><img class="pro_free" alt="Free"></td></tr>`, nil, nil},
		{`<tr><td><img class="pro_newdiscount" alt="20%"></td></tr>`, nil, nil},
		{`<tr><td><img class="pro_free"><span class="seed-box-policy-download is-blocked">100%</span></td></tr>`, ptr(1), ptr(1)},
	} {
		d, _ := parseDocument([]byte(`<table>`+tc.html+`</table>`), "XPath")
		rows, _ := d.values("//tr", nil)
		actual := promotionFactor(rule, rows[0], tc.parsed)
		if (actual == nil) != (tc.want == nil) || actual != nil && *actual != *tc.want {
			t.Fatal(tc.html, actual, tc.want)
		}
	}
}
func ptr(n float64) *float64 { return &n }
func TestCompiledDefaultsKeepPaginationQueryBoundary(t *testing.T) {
	for _, id := range []string{"ptchdbits", "ubits"} {
		template, _ := GoTemplate(id)
		params := template.Definition["requests"].(Map)["search"].(Map)["params"].(Map)
		if params["search"] == nil || params["search_mode"] == nil || params["incldead"] == nil {
			t.Fatal("missing explicit search defaults", id)
		}
	}
}

func TestCompiledNextLinksAcceptOnlyDeclaredDefaultsAndExactKeyword(t *testing.T) {
	for _, id := range []string{"ptchdbits", "ubits"} {
		template, _ := GoTemplate(id)
		q := url.Values{}
		rule := template.Definition["requests"].(Map)["search"].(Map)
		for k, v := range rule["params"].(Map) {
			q.Set(k, text(slice(v)[0]))
		}
		q.Set("search", "Owned Film")
		current, _ := url.Parse(template.Domain + "/torrents.php?" + q.Encode())
		next := current.Query()
		next.Set("page", "1")
		if _, e := validatedPageLink(current, "?"+next.Encode()); e != nil {
			t.Fatal(id, "declared defaults rejected", e)
		}
		next.Set("search", "Other Film")
		if _, e := validatedPageLink(current, "?"+next.Encode()); e == nil {
			t.Fatal("changed query accepted")
		}
	}
}
