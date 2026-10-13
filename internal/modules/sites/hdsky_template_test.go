package sites

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func minimalHDSKY(t *testing.T, mode string) Client {
	t.Helper()
	raw, e := os.ReadFile("testdata/repository/sites/hdsky.json")
	if e != nil {
		t.Fatal(e)
	}
	config, e := Decode(raw, []byte(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	if len(config.Requests) != 4 || len(object(config.Requests["search"]["fields"])) != 10 || config.HasHR != nil {
		t.Fatal("unexpected feature/HR declaration", config)
	}
	if Compatibility(config)["state"] != "parser_supported" {
		t.Fatal(Compatibility(config))
	}
	config.Domain = "https://owned.example"
	return Client{Config: config, Account: Account{Endpoint: "https://owned.example", Cookie: "owned-fixture-only"}, HTTP: &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "owned.example" {
			t.Fatal("escaped site", r.URL)
		}
		if r.Header.Get("Cookie") != "owned-fixture-only" {
			t.Fatal("missing own cookie")
		}
		body := `<html><body><a href="logout.php">退出</a><a href="userdetails.php?id=42"><b>Owned</b></a><div>上传量：1 TiB 下载量：1 GiB 分享率：1024</div><a href="mybonus.php">1,234.5</a></body></html>`
		switch r.URL.Path {
		case "/userdetails.php":
			body = `<table><tr><td>等级</td><td><img title="Power User"></td></tr></table>`
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "42" || r.URL.Query().Get("type") != "seeding" {
				t.Fatal("wrong stats identity", r.URL)
			}
			if r.Header.Get("Referer") != "https://owned.example/userdetails.php?id=42" {
				t.Fatal("wrong referer")
			}
			body = "没有记录"
			if mode == "seeding" {
				body = `<table><tr><th>Name</th><th>Peer</th><th>Size</th></tr><tr><td>Owned</td><td>3</td><td>2 GiB</td></tr></table>`
			}
			if mode == "challenge" {
				body = "<form>认证挑战</form>"
			}
		case "/torrents.php":
			if r.URL.Query().Get("search") != "Owned" {
				t.Fatal("lost search identity", r.URL)
			}
			id := "11"
			free := `<img class="pro_free">`
			next := `<a rel="next" href="torrents.php?search=Owned&amp;page=1">下一页</a>`
			if r.URL.Query().Get("page") == "1" {
				id = "12"
				free = ""
				next = ""
			}
			body = fmt.Sprintf(`<table class="torrents"><tr><th>category</th></tr><tr><td>movie</td><td><table class="torrentname"><tr><td class="embedded"><a href="details.php?id=%s" title="Owned.Movie.2026.1080p.WEB-DL.x265-TEAM">Owned</a><span>自有说明</span><span class="optiontag">国语</span>%s</td></tr></table><form action="/download.php?id=%s"></form></td><td>3</td><td>4</td><td>2 GiB</td><td>9</td><td>1</td></tr></table>%s`, id, free, id, next)
		case "/download.php":
			body = "owned synthetic torrent bytes"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}}
}
func TestHDSKYMinimalTemplateSearchAndTorrent(t *testing.T) {
	c := minimalHDSKY(t, "zero")
	result, e := c.SearchAll(context.Background(), SearchQuery{Keyword: "Owned"})
	if e != nil || !result.Complete || result.Pages != 2 || len(result.Resources) != 2 {
		t.Fatal(result, e)
	}
	first := result.Resources[0]
	if first.Size != 2<<30 || first.Seeders != 9 || first.DownloadURL != "https://owned.example/download.php?id=11" || first.DownloadFactor == nil || *first.DownloadFactor != 0 || first.HR != nil {
		t.Fatal(first)
	}
	if result.Resources[1].DownloadFactor != nil {
		t.Fatal("unknown promotion marked paid/free")
	}
	if first.Subtitle != "自有说明" || len(first.Labels) != 1 || first.Labels[0] != "国语" {
		t.Fatal(first)
	}
	raw, e := c.Torrent(context.Background(), first.TorrentID, first.DownloadURL)
	if e != nil || string(raw) != "owned synthetic torrent bytes" {
		t.Fatal(string(raw), e)
	}
	if _, e = c.Torrent(context.Background(), "11", "https://elsewhere.example/download.php?id=11"); e == nil {
		t.Fatal("unsafe download")
	}
}
func TestHDSKYMinimalTemplateStatsNeverInventZero(t *testing.T) {
	for _, mode := range []string{"zero", "seeding", "challenge"} {
		t.Run(mode, func(t *testing.T) {
			c := minimalHDSKY(t, mode)
			m, e := c.Metrics(context.Background())
			if e != nil || m.Name != "Owned" || m.Uploaded == nil || *m.Uploaded != 1<<40 || m.Downloaded == nil || *m.Downloaded != 1<<30 || m.Bonus == nil || *m.Bonus != 1234.5 || m.Level != "Power User" {
				t.Fatal(m, e)
			}
			if mode == "challenge" {
				if m.Seeding != nil || m.SeedingComplete || !m.Partial {
					t.Fatal("invented totals", m)
				}
				return
			}
			count, size := int64(0), int64(0)
			if mode == "seeding" {
				count = 1
				size = 2 << 30
			}
			if !m.SeedingComplete || m.Seeding == nil || *m.Seeding != count || m.SeedingBytes == nil || *m.SeedingBytes != size {
				t.Fatal(m)
			}
		})
	}
}
