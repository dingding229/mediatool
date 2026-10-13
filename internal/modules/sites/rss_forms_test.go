package sites

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOURBITSObservedSerializedFormAndCredentialedCandidatePriority(t *testing.T) {
	cfg, e := DecodeStored(SnapshotForTest("ourbits"), nil)
	if e != nil {
		t.Fatal(e)
	}
	cfg.Domain = "https://owned.example"
	calls := 0
	c := Client{Config: cfg, Account: Account{Endpoint: cfg.Domain, Cookie: "owned"}, HTTP: &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `<a href="logout.php">退出</a><a href="userdetails.php?id=42">Owned</a>`
		if r.URL.Path == "/getrss.php" {
			body += `<form id="getrss_form" action="getrss.php" method="post"><input name="userId" value="42"/><input name="passkey" value="ownedkey"/><input name="ititle" checked type="checkbox"/><input name="https" value="1" checked type="checkbox"/></form><a href="#" id="dl_link"></a>`
		}
		if r.URL.Path == "/torrentrss.php" {
			if r.URL.Query().Get("passkey") != "ownedkey" || r.URL.Query().Get("linktype") != "dl" {
				t.Fatal("form values not preserved", r.URL.Query())
			}
			body = `<rss version="2.0"><channel><title>Owned</title></channel></rss>`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}}
	rss, uid, e := c.DiscoverRSS(context.Background())
	if e != nil || rss == "" || uid != "42" || calls > 8 {
		t.Fatal(rss, uid, e, calls)
	}
	doc, _ := parseDocument([]byte(`<a href="/torrentrss.php?rows=10">invalid first</a><a href="/torrentrss.php?passkey=owned&amp;linktype=dl">valid</a>`), "XPath")
	candidates := feedCandidates(doc.html)
	if !strings.Contains(candidates[0], "passkey=owned") {
		t.Fatal("credentialless feed fetched before real feed", candidates)
	}
}
