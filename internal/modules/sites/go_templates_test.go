package sites

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGoTemplatesUseEnglishNamesAndCompiledSnapshots(t *testing.T) {
	want := map[string]string{"hdsky": "HDSKY", "ourbits": "OURBITS", "ptchdbits": "CHDBITS", "ssd": "SSD", "ttg": "TTG", "ubits": "UBITS"}
	for _, template := range GoTemplates() {
		if template.Name != want[template.ID] || len(template.Fields) != 2 {
			t.Fatal(template)
		}
		raw, e := template.Snapshot()
		if e != nil {
			t.Fatal(e)
		}
		c, e := DecodeStored(raw, nil)
		if e != nil || c.Name != template.Name || len(c.Requests) != 4 {
			t.Fatal(c, e)
		}
	}
}
func TestRSSDiscoveryUsesActualAuthenticatedFeedAndUserAgent(t *testing.T) {
	raw := SnapshotForTest("hdsky")
	cfg, e := DecodeStored(raw, nil)
	if e != nil {
		t.Fatal(e)
	}
	cfg.Domain = "https://owned.example"
	c := Client{Config: cfg, Account: Account{Endpoint: cfg.Domain, Cookie: "owned-cookie", UserAgent: "Owned-UA"}, HTTP: &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("User-Agent") != "Owned-UA" || r.Header.Get("Cookie") != "owned-cookie" || r.URL.Host != "owned.example" {
			t.Fatal("identity/UA", r.Header.Get("User-Agent"))
		}
		body := `<a href="logout.php">退出</a><a href="userdetails.php?id=42"><b>owned</b></a><a href="/torrentrss.php?passkey=owned">RSS</a>`
		if r.URL.Path == "/torrentrss.php" {
			body = `<rss version="2.0"><channel><title>Owned</title></channel></rss>`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}}
	rss, uid, e := c.DiscoverRSS(context.Background())
	if e != nil || rss != "https://owned.example/torrentrss.php?passkey=owned" || uid != "42" {
		t.Fatal(rss, uid, e)
	}
}

func TestRSSDiscoveryDoesNotPersistUnverifiedOrCrossOriginLinks(t *testing.T) {
	cfg, e := DecodeStored(SnapshotForTest("hdsky"), nil)
	if e != nil {
		t.Fatal(e)
	}
	cfg.Domain = "https://owned.example"
	for _, kind := range []string{"foreign", "html"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			c := Client{Config: cfg, Account: Account{Endpoint: cfg.Domain, Cookie: "owned", UserAgent: "Owned-UA"}, HTTP: &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "owned.example" {
					t.Fatal("foreign RSS was fetched")
				}
				body := `<a href="logout.php">退出</a><a href="userdetails.php?id=42">owned</a><a href="/rss.php">RSS</a>`
				if kind == "foreign" {
					body = `<a href="logout.php">退出</a><a href="userdetails.php?id=42">owned</a><a href="https://other.example/rss.php?passkey=owned">RSS</a>`
				}
				if r.URL.Path == "/rss.php" {
					body = `<form>login HTML, not RSS</form>`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}}
			rss, _, e := c.DiscoverRSS(context.Background())
			if e == nil || rss != "" || calls > 12 {
				t.Fatal(rss, e, calls)
			}
		})
	}
}

func TestCompiledSSDSubtitleAndSummaryEmptyEvidence(t *testing.T) {
	cfg, e := DecodeStored(SnapshotForTest("ssd"), nil)
	if e != nil {
		t.Fatal(e)
	}
	d, e := parseDocument([]byte(`<table class="torrents"><tr><td><table class="torrentname"><tr><td class="embedded"><div class="torrent-smalldescr"><span class="torrent-tag">tag</span><span>Owned subtitle</span></div></td></tr></table></td></tr></table>`), "CssSelector")
	if e != nil {
		t.Fatal(e)
	}
	rows, _ := d.values("table.torrents > tbody > tr", nil)
	if len(rows) == 0 {
		rows, _ = d.values("table.torrents tr:has(table.torrentname)", nil)
	}
	value, e := mapFields(d, Map{"description": object(cfg.Requests["search"]["fields"])["description"]}, rows[0], nil)
	if e != nil || value["description"] != "Owned subtitle" {
		t.Fatal(value, e)
	}
	c := Client{Config: cfg, Account: Account{Endpoint: cfg.Domain, UserID: "42"}, HTTP: &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Request: r, Body: io.NopCloser(strings.NewReader("没有记录"))}, nil
	})}}
	count, size, _, complete, note := c.collectSeedingSummary(context.Background(), cfg.Requests["seeding_statistics"], map[string]string{"userId": "42", "domain": cfg.Domain})
	if !complete || count != 0 || size != 0 || note != "" {
		t.Fatal(count, size, complete, note)
	}
}

func TestUbitsBlockedSeedboxOverridesFreeEvenWithEmptyMarkerText(t *testing.T) {
	cfg, e := DecodeStored(SnapshotForTest("ubits"), nil)
	if e != nil {
		t.Fatal(e)
	}
	d, e := parseDocument([]byte(`<table><tr><td><img class="pro_free" alt="Free"><span class="seed-box-policy-download is-blocked"></span></td></tr></table>`), "CssSelector")
	if e != nil {
		t.Fatal(e)
	}
	rows, _ := d.values("tr", nil)
	v, e := mapFields(d, Map{"factor": object(cfg.Requests["search"]["fields"])["downloadvolumefactor"]}, rows[0], nil)
	factor, ok := checkedNumber(v["factor"])
	if e != nil || !ok || factor != 1 {
		t.Fatal(v, e)
	}
}
