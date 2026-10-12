package sites

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSeedingDeclaredEmptyRowsPaginationAndFailure(t *testing.T) {
	for _, mode := range []string{"zero", "table", "pages", "repeat", "wrong_origin", "unknown", "challenge", "bad_size"} {
		t.Run(mode, func(t *testing.T) {
			basic := Map{"parser": "JsonPath", "path": "/profile", "fields": Map{"is_login": Map{"selector": "$.login"}, "id": Map{"selector": "$.id"}, "name": Map{"selector": "$.name"}, "uploaded": Map{"selector": "$.uploaded"}, "downloaded": Map{"selector": "$.downloaded"}, "ratio": Map{"selector": "$.ratio"}, "bonus": Map{"selector": "$.bonus"}, "level": Map{"selector": "$.level"}}}
			seed := Map{"parser": "XPath", "path": "/getusertorrentlistajax.php?userid={userId}&type=seeding", "list": Map{"selector": "//tr[position()>1]", "next_page": Map{"selector": "//a[@rel='next']/@href"}}, "fields": Map{"size": Map{"selector": "/td[3]", "filters": []any{Map{"name": "byte_size"}}}}}
			calls := 0
			c := Client{Config: Config{ID: "hdsky", Schema: "hdsky", Domain: "https://owned.example", Requests: map[string]Map{"user_basic_info": basic, "seeding_statistics": seed}}, Account: Account{Endpoint: "https://owned.example"}, HTTP: &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"login":true,"id":"1","name":"owned","uploaded":100,"downloaded":10,"ratio":10,"bonus":1,"level":"Owned"}`
				if r.URL.Path != "/profile" {
					calls++
					switch mode {
					case "zero":
						body = "没有记录"
					case "unknown":
						body = ""
					case "challenge":
						body = "<form>登录认证挑战</form>"
					default:
						size := "1 GiB"
						if mode == "bad_size" {
							size = "no size"
						}
						id := calls
						if mode == "repeat" {
							id = 1
						}
						body = fmt.Sprintf(`<table><tr><th>Name</th><th>Peers</th><th>Size</th></tr><tr><td>Owned %d</td><td>1</td><td>%s</td></tr></table>`, id, size)
						if mode == "pages" && calls == 1 || mode == "repeat" || mode == "wrong_origin" {
							target := "/getusertorrentlistajax.php?userid=1&amp;type=seeding&amp;page=1"
							if mode == "wrong_origin" {
								target = "https://evil.example/seed?page=1"
							}
							body += `<a rel="next" href="` + target + `">下一页</a>`
						}
					}
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}}
			metrics, e := c.Metrics(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			complete := mode == "zero" || mode == "table" || mode == "pages"
			if complete {
				if !metrics.SeedingComplete || metrics.Seeding == nil || metrics.SeedingBytes == nil {
					t.Fatal(mode, metrics)
				}
				expected := int64(1)
				if mode == "zero" {
					expected = 0
				}
				if mode == "pages" {
					expected = 2
				}
				if *metrics.Seeding != expected || *metrics.SeedingBytes != expected*(1<<30) {
					t.Fatal(mode, metrics)
				}
			} else if metrics.Seeding != nil || metrics.SeedingBytes != nil || !metrics.Partial {
				t.Fatal("unproved totals", mode, metrics)
			}
			if calls > 2 {
				t.Fatal("unbounded requests", calls)
			}
		})
	}
}
func TestPageLinksCannotChangeIdentityOrEscapeOrigin(t *testing.T) {
	u, _ := url.Parse("https://owned.example/torrents.php?search=Owned&cat=1")
	for _, bad := range []string{"https://evil.example/torrents.php?search=Owned&cat=1&page=1", "/download.php?search=Owned&cat=1&page=1", "?search=Changed&cat=1&page=1", "?cat=1&page=1", "?search=Owned&cat=1&page=1&token=x", "?search=Owned&cat=1&page=1&page=2", "?search=Owned&cat=1&page=x", "?search=Owned&cat=1"} {
		if _, e := validatedPageLink(u, bad); e == nil {
			t.Fatal(bad)
		}
	}
	if _, e := validatedPageLink(u, "?search=Owned&cat=1&page=1"); e != nil {
		t.Fatal(e)
	}
}
func TestNexusSearchFollowsActualLinksAndRejectsRepeatedChangedOrLimitedPages(t *testing.T) {
	for _, mode := range []string{"complete", "repeat", "changed", "limit", "challenge"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			basic := Map{"parser": "JsonPath", "path": "/profile", "fields": Map{"is_login": Map{"selector": "login"}}}
			rule := Map{"parser": "CssSelector", "path": "/torrents.php", "params": Map{"search": "{keyword}"}, "list": Map{"selector": "table.torrents tr:has(table.torrentname)"}, "fields": Map{"id": Map{"selector": "a.id", "attribute": "data-id"}, "title": Map{"selector": "a.id"}}}
			c := Client{Config: Config{ID: "hdsky", Schema: "hdsky", Domain: "https://owned.example", Requests: map[string]Map{"user_basic_info": basic, "search": rule}}, Account: Account{Endpoint: "https://owned.example"}, HTTP: &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"login":true}`
				if r.URL.Path == "/torrents.php" {
					calls++
					id := calls
					if mode == "repeat" {
						id = 1
					}
					body = fmt.Sprintf(`<table class="torrents"><tr><td><table class="torrentname"><tr><td><a class="id" data-id="%d">Owned 2026</a></td></tr></table></td></tr></table>`, id)
					if mode == "challenge" {
						body = "<form>challenge</form>"
					} else if calls == 1 || mode == "repeat" || mode == "limit" {
						query := "Owned"
						if mode == "changed" {
							query = "Changed"
						}
						body += fmt.Sprintf(`<a href="/torrents.php?search=%s&amp;page=%d">下一页</a>`, query, calls)
					}
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}}
			result, e := c.SearchAll(context.Background(), SearchQuery{Keyword: "Owned"})
			if mode == "challenge" {
				if e == nil {
					t.Fatal("challenge accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if mode == "complete" {
				if !result.Complete || len(result.Resources) != 2 {
					t.Fatal(result)
				}
			} else if result.Complete || result.Reason == "" {
				t.Fatal(result)
			}
			if calls > SearchPageLimit {
				t.Fatal(calls)
			}
		})
	}
}
