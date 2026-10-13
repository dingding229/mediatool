package sites

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestHDSKYConfirmedEmptySearchVersusBrokenAndLoggedOutDocuments(t *testing.T) {
	cfg, e := DecodeStored(SnapshotForTest("hdsky"), nil)
	if e != nil {
		t.Fatal(e)
	}
	cfg.Domain = "https://owned.example"
	for _, tc := range []struct {
		name, html string
		want       bool
	}{{"empty", `<a href="logout.php">退出</a><table><tr><td class="text">没有种子。请用准确的关键字重试。</td></tr></table>`, true}, {"loggedout", `<table><tr><td class="text">没有种子。请用准确的关键字重试。</td></tr></table>`, false}, {"broken", `<a href="logout.php">退出</a><p>broken response</p>`, false}, {"generic", `<a href="logout.php">退出</a><p>没有种子。请用准确的关键字重试。</p>`, false}} {
		t.Run(tc.name, func(t *testing.T) {
			c := Client{Config: cfg, Account: Account{Endpoint: cfg.Domain, Cookie: "owned"}, HTTP: &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
				body := `<a href="logout.php">退出</a>`
				if r.URL.Path == "/torrents.php" {
					body = tc.html
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
			})}}
			out, e := c.SearchAll(context.Background(), SearchQuery{Keyword: "Owned Zero"})
			if tc.want && (e != nil || !out.Complete || len(out.Resources) != 0) {
				t.Fatal(out, e)
			}
			if !tc.want && e == nil {
				t.Fatal("unproved empty accepted", out)
			}
		})
	}
}
func TestRSSGeneratorFormRetainsCSRFAndOnlySubmitsDeclaredSameOriginForm(t *testing.T) {
	doc, _ := parseDocument([]byte(`<form action="profile.php" method="post"><input name="password" value="never"/></form><form action="getrss.php" method="post"><input name="_token" type="hidden" value="owned-csrf"/><input name="inclbookmarked" type="radio" checked value="0"/><input name="inclbookmarked" type="radio" value="1"/><input name="unchecked" type="checkbox" value="unsafe"/><select name="showrows"><option value="10">10</option><option value="50">50</option></select></form>`), "XPath")
	page, _ := url.Parse("https://owned.example/getrss.php")
	r, ok := rssFormRule(doc.html, page, "/getrss.php", "")
	if !ok {
		t.Fatal("form missed")
	}
	fields := object(r["form_data"])
	if fields["password"] != nil || fields["unchecked"] != nil || fields["_token"].([]any)[0] != "owned-csrf" || fields["showrows"].([]any)[0] != "10" {
		t.Fatal(fields)
	}
}
