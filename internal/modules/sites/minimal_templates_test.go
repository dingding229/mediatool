package sites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"media-tool/internal/modules/releasename"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

var minimalSites = []string{"chdbits", "ssd", "ourbits", "ubits", "ttg"}

func minimalSiteConfig(t *testing.T, site string) Config {
	t.Helper()
	raw, e := os.ReadFile("../../../examples/sites/" + site + ".json")
	if e != nil {
		t.Fatal(e)
	}
	c, e := Decode(raw, []byte(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Requests) != 4 {
		t.Fatal("non-core requests", c.Requests)
	}
	for _, key := range []string{"user_basic_info", "user_details", "search", "seeding_statistics"} {
		if len(c.Requests[key]) == 0 {
			t.Fatal(key)
		}
	}
	for key := range object(c.Requests["search"]["fields"]) {
		if !containsName(resourceFieldNames, key) {
			t.Fatal("unused resource field", key)
		}
	}
	if Compatibility(c)["accountVerified"] != false {
		t.Fatal("fixture claimed live account verification")
	}
	for _, r := range c.Requests {
		if _, exists := r["disabled_extends"]; exists {
			t.Fatal("inheritance remained")
		}
	}
	c.Domain = "https://owned.example"
	c.AdditionalDomains = nil
	return c
}
func containsName(names []string, key string) bool {
	for _, name := range names {
		if name == key {
			return true
		}
	}
	return false
}
func minimalProfile(site string) string {
	body := `<html><body><a href="logout.php">退出</a><a href="userdetails.php?id=42"><b>Owned</b></a><div>上传量：1 TiB 下载量：1 GiB 分享率：1024</div><a href="mybonus.php" title="1,234.5">1,234.5</a>`
	if site == "ttg" {
		body += `<font>上传量</font><font><a>1 TiB</a></font><font>下载量</font><font><a>1 GiB</a></font>`
	}
	return body + `</body></html>`
}
func minimalResourcePage(site, id, kind, next string) string {
	title := "Owned.Movie.2026.1080p.WEB-DL.x265-TEAM"
	promotion := ""
	hr := ""
	switch kind {
	case "free":
		promotion = `<img class="pro_free">`
		if site == "ssd" {
			promotion = `<span class="torrent-pro-icon torrent-pro-free"></span>`
		}
		if site == "ttg" {
			promotion = `<img alt="free">`
		}
	case "half":
		promotion = `<img class="pro_50pctdown">`
		if site == "ssd" {
			promotion = `<span class="torrent-pro-icon torrent-pro-halfdown"></span>`
		}
		if site == "ttg" {
			promotion = `<img alt="50%">`
		}
	case "third":
		promotion = `<img class="pro_30pctdown">`
		if site == "ssd" {
			promotion = `<span class="torrent-pro-icon torrent-pro-thirtypercent"></span>`
		}
		if site == "ttg" {
			promotion = `<img alt="30%">`
		}
	}
	// CHD/TTG durations come from actual badge text; OURBITS/UBITS only prove HR
	// presence. Missing badges do not prove an exemption.
	if kind == "hr" || kind == "hr14" || kind == "hr0" {
		days := "14"
		if kind == "hr0" {
			days = "0"
		}
		switch site {
		case "chdbits":
			hr = `<div class="circle"><div class="circle-text">` + days + `</div></div>`
		case "ourbits", "ubits":
			hr = `<img class="hitandrun" title="H&amp;R required">`
		}
	}
	if site == "ttg" {
		// Upstream sixth child is the site-specific HR icon. Non-HR images have no
		// numeric title and must not fabricate a duration.
		badges := `<img><img><img><img>`
		if strings.HasPrefix(kind, "hr") {
			days := "14"
			if kind == "hr0" {
				days = "0"
			}
			badges += `<img title="H&amp;R ` + days + ` days">`
		} else {
			if kind == "unrelated_badge" {
				badges += `<img title="2160p 0day">`
			} else {
				badges += `<img title="no duration proof">`
			}
		}
		return fmt.Sprintf(`<table id="torrent_table"><tr id="torrent_%s"><td>movie</td><td><div class="name_left"><a href="/t/%s/"><b>%s<span>自有说明</span></b></a>%s<span><b>国语</b></span></div>%s<a class="dl_a" href="/dl/%s/">下载</a></td><td>3</td><td>4</td><td>5</td><td>6</td><td>2 GiB</td><td>8</td><td><b><a><font>9</font></a></b><b><a>1</a></b></td></tr></table>%s`, id, id, title, badges, promotion, id, next)
	}
	detail := fmt.Sprintf(`<a href="details.php?id=%s" title="%s"><b>%s</b></a>`, id, title, title)
	description := `自有说明`
	labels := `<span title="">国语</span>`
	switch site {
	case "chdbits":
		description = `<font class="subtitle">自有说明<div style="display:block"><div class="tag">国语</div></div></font>`
		labels = ""
	case "ssd":
		description = `<div class="torrent-smalldescr"><span>自有说明</span><a><span>国语</span></a></div>`
		labels = ""
	case "ourbits":
		labels = `<div><a href="?tags[]=owned"><span>国语</span></a></div>`
	}
	return fmt.Sprintf(`<table class="torrents"><tr><th>Type</th></tr><tr><td>movie</td><td><table class="torrentname"><tr><td class="embedded">%s%s%s%s%s</td></tr></table><a href="download.php?id=%s">下载</a></td><td>3</td><td>4</td><td class="rowfollow">2 GiB</td><td>9</td><td>1</td></tr></table>%s`, detail, description, labels, promotion, hr, id, next)
}
func minimalStats(site, mode string) string {
	if mode == "challenge" {
		return `<form action="login.php">登录挑战</form>`
	}
	if mode == "blank" {
		return ""
	}
	if site == "ssd" {
		switch mode {
		case "zero":
			return `<b>0</b> 个种子，总大小 <b>0 B</b>`
		case "challenge_numbers":
			return `<form action="login.php"><b>2</b><b>2 GiB</b></form>`
		case "bad_count":
			return `<b>-1</b><b>2 GiB</b>`
		case "negative_size":
			return `<b>2</b><b>-2 GiB</b>`
		case "partial_text":
			return `<b>2</b><b>2 GiB（本页）</b>`
		case "fractional_count":
			return `<b>1.5</b><b>2 GiB</b>`
		case "overflow":
			return `<b>9223372036854775809</b><b>2 GiB</b>`
		case "bad_size":
			return `<b>2</b><b>unknown</b>`
		case "contradictory":
			return `<b>0</b><b>2 GiB</b>`
		case "duplicate":
			return `<b>2</b><b>2 GiB</b><b>2</b><b>2 GiB</b>`
		default:
			return `<b>2</b> 个种子，总大小 <b>2 GiB</b>`
		}
	}
	if site == "ubits" {
		switch mode {
		case "zero":
			return `<div>0 条记录，总大小：0 B</div>`
		case "challenge_numbers":
			return `<form action="login.php"><div>2 条记录，总大小：2 GiB</div></form>`
		case "bad_count":
			return `<div>-1 条记录，总大小：2 GiB</div>`
		case "negative_size":
			return `<div>2 条记录，总大小：-2 GiB</div>`
		case "partial_text":
			return `<div>2 条记录，总大小：2 GiB（本页）</div>`
		case "fractional_count":
			return `<div>1.5 条记录，总大小：2 GiB</div>`
		case "overflow":
			return `<div>9223372036854775809 条记录，总大小：2 GiB</div>`
		case "bad_size":
			return `<div>2 条记录，总大小：unknown</div>`
		case "contradictory":
			return `<div>0 条记录，总大小：2 GiB</div>`
		case "duplicate":
			return `<div>2 条记录，总大小：2 GiB</div><div>2 条记录，总大小：2 GiB</div>`
		default:
			return `<div><div>2 条记录，总大小：2 GiB</div></div>`
		}
	}
	if mode == "zero" {
		return "没有记录"
	}
	// TTG has a distinct fourth-column list in the userdetails page.
	rows := `<table><tr><th>Title</th><th>Peers</th><th>Size</th></tr><tr><td>Owned</td><td>9</td><td>2 GiB</td></tr></table>`
	if site == "ttg" {
		rows = `<table><tr><th>Title</th></tr><tr><td>Owned</td><td>9</td><td>1</td><td>2 GiB</td></tr></table>`
		return `<table><tr><td>当前上传</td><td>` + rows + `</td></tr></table>`
	}
	return rows
}
func minimalTemplateClient(t *testing.T, site, statsMode, searchMode string) Client {
	t.Helper()
	c := minimalSiteConfig(t, site)
	return Client{Config: c, Account: Account{Endpoint: c.Domain, Cookie: "owned-fixture-only"}, HTTP: &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "owned.example" || r.Header.Get("Cookie") != "owned-fixture-only" {
			t.Fatal("request escaped own fixture", r.URL.Host)
		}
		body := minimalProfile(site)
		switch r.URL.Path {
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "42" || r.URL.Query().Get("type") != "seeding" {
				t.Fatal("statistics identity", r.URL)
			}
			if r.Header.Get("Referer") != c.Domain+"/userdetails.php?id=42" {
				t.Fatal("referer")
			}
			body = minimalStats(site, statsMode)
		case "/userdetails.php":
			body = `<table><tr><td>等级</td><td><img title="Power User"></td></tr></table>`
			if site == "ttg" {
				body += minimalStats(site, statsMode)
			}
		case "/torrents.php", "/browse.php":
			field := "search"
			if site == "ttg" {
				field = "search_field"
				if r.URL.Query().Get("c") != "M" || r.URL.Query().Has("search") {
					t.Fatal("TTG search contract", r.URL)
				}
			}
			if r.URL.Query().Get(field) != "Owned" {
				t.Fatal("search keyword lost", r.URL)
			}
			if searchMode == "challenge" {
				body = `<form>验证登录</form>`
				break
			}
			if searchMode == "empty" {
				if site == "ttg" {
					body = `<table id="torrent_table"><tr><th>标题</th></tr></table>`
				} else {
					body = `<table class="torrents"><tr><th>标题</th></tr></table>`
				}
				break
			}
			id := "11"
			kind := "free"
			next := ""
			if searchMode == "pages" || searchMode == "unsafe_next" {
				q := r.URL.Query()
				q.Set("page", "1")
				href := r.URL.Path + "?" + q.Encode()
				if searchMode == "unsafe_next" {
					href = "https://elsewhere.example" + href
				}
				next = `<a rel="next" href="` + strings.ReplaceAll(href, "&", "&amp;") + `">下一页</a>`
				if r.URL.Query().Get("page") == "1" {
					id = "12"
					kind = "unknown"
					next = ""
				}
			} else if searchMode != "" {
				kind = searchMode
			}
			body = minimalResourcePage(site, id, kind, next)
		default:
			if r.URL.Path == "/download.php" || strings.HasPrefix(r.URL.Path, "/dl/") {
				body = "owned synthetic torrent bytes"
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}}
}
func TestMinimalTemplatesSearchDownloadAndUnknownFields(t *testing.T) {
	for _, site := range minimalSites {
		t.Run(site, func(t *testing.T) {
			c := minimalTemplateClient(t, site, "valid", "pages")
			r, e := c.SearchAll(context.Background(), SearchQuery{Keyword: "Owned"})
			if e != nil || !r.Complete || r.Pages != 2 || len(r.Resources) != 2 {
				t.Fatal(r, e)
			}
			first := r.Resources[0]
			if first.Title != "Owned.Movie.2026.1080p.WEB-DL.x265-TEAM" || first.Size != 2<<30 || first.Seeders != 9 || first.Leechers != 1 || first.DownloadFactor == nil || *first.DownloadFactor != 0 || first.Subtitle != "自有说明" || len(first.Labels) != 1 || first.Labels[0] != "国语" {
				t.Fatal(first)
			}
			if first.HR != nil || r.Resources[1].DownloadFactor != nil {
				t.Fatal("missing badge marked known", r.Resources)
			}
			expected := "https://owned.example/download.php?id=11"
			if site == "ttg" {
				expected = "https://owned.example/dl/11/"
			}
			if first.DownloadURL != expected {
				t.Fatal("invented download", first.DownloadURL)
			}
			raw, e := c.Torrent(context.Background(), first.TorrentID, first.DownloadURL)
			if e != nil || string(raw) != "owned synthetic torrent bytes" {
				t.Fatal(string(raw), e)
			}
			if _, e = c.Torrent(context.Background(), "11", "https://elsewhere.example/dl/11"); e == nil {
				t.Fatal("unsafe download accepted")
			}
		})
	}
}
func TestMinimalTemplatesPromotionHRAndSearchEvidence(t *testing.T) {
	for _, site := range minimalSites {
		for _, mode := range []string{"unknown", "half", "third", "hr14", "hr0", "unrelated_badge", "empty", "challenge", "unsafe_next"} {
			t.Run(site+"/"+mode, func(t *testing.T) {
				c := minimalTemplateClient(t, site, "valid", mode)
				r, e := c.SearchAll(context.Background(), SearchQuery{Keyword: "Owned"})
				if mode == "challenge" {
					if !errors.Is(e, ErrSearchResponse) {
						t.Fatal("challenge became empty", r, e)
					}
					return
				}
				if e != nil {
					t.Fatal(r, e)
				}
				if mode == "empty" {
					if !r.Complete || len(r.Resources) != 0 {
						t.Fatal("declared empty container", r)
					}
					return
				}
				if mode == "unsafe_next" {
					if r.Complete || r.Pages != 1 {
						t.Fatal("unsafe pagination", r)
					}
					return
				}
				v := r.Resources[0]
				if mode == "half" || mode == "third" {
					factor := 0.5
					if mode == "third" {
						factor = 0.3
					}
					if v.DownloadFactor == nil || *v.DownloadFactor != factor {
						t.Fatal(v)
					}
				}
				if (mode == "unknown" || mode == "unrelated_badge") && v.HR != nil {
					t.Fatal("HR absence not unknown", v)
				}
				if strings.HasPrefix(mode, "hr") {
					switch site {
					case "chdbits", "ttg":
						if v.HR == nil || *v.HR != (mode != "hr0") {
							t.Fatal(v)
						}
					case "ourbits", "ubits":
						if v.HR == nil || !*v.HR {
							t.Fatal(v)
						}
					case "ssd":
						if v.HR != nil {
							t.Fatal("SSD unverified HR selector", v)
						}
					}
				}
			})
		}
	}
}
func TestMinimalTemplatesAccountMetricsAndSeedingModes(t *testing.T) {
	for _, site := range minimalSites {
		for _, mode := range []string{"valid", "zero", "challenge", "blank"} {
			t.Run(site+"/"+mode, func(t *testing.T) {
				c := minimalTemplateClient(t, site, mode, "free")
				m, e := c.Metrics(context.Background())
				if e != nil || m.Name != "Owned" || m.Uploaded == nil || *m.Uploaded != 1<<40 || m.Downloaded == nil || *m.Downloaded != 1<<30 || m.Ratio == nil || *m.Ratio != 1024 || m.Bonus == nil || *m.Bonus != 1234.5 || m.Level != "Power User" {
					raw, _ := json.Marshal(m)
					t.Fatal(string(raw), e)
				}
				known := mode == "valid" || mode == "zero" && site != "ttg"
				if !known {
					if m.Seeding != nil || m.SeedingBytes != nil || m.SeedingComplete || !m.Partial {
						t.Fatal("invented totals", m)
					}
					return
				}
				count := int64(1)
				if site == "ssd" || site == "ubits" {
					count = 2
				}
				size := int64(2 << 30)
				if mode == "zero" {
					count = 0
					size = 0
				}
				if !m.SeedingComplete || m.Seeding == nil || *m.Seeding != count || m.SeedingBytes == nil || *m.SeedingBytes != size {
					t.Fatal(m)
				}
			})
		}
	}
}
func TestSummarySeedingRejectsInvalidOrAmbiguousTotals(t *testing.T) {
	for _, site := range []string{"ssd", "ubits"} {
		for _, mode := range []string{"bad_count", "bad_size", "contradictory", "duplicate", "negative_size", "partial_text", "fractional_count", "overflow", "challenge_numbers"} {
			t.Run(site+"/"+mode, func(t *testing.T) {
				m, e := minimalTemplateClient(t, site, mode, "free").Metrics(context.Background())
				if e != nil || !m.Partial || m.SeedingComplete || m.Seeding != nil || m.SeedingBytes != nil {
					raw, _ := json.Marshal(m)
					t.Fatal(string(raw), e)
				}
			})
		}
	}
}
func TestTemplateIndexHasOnlySelfContainedDefinitions(t *testing.T) {
	raw, e := os.ReadFile("../../../examples/sites/index.json")
	if e != nil {
		t.Fatal(e)
	}
	var idx struct {
		Templates      []struct{ ID, File string }
		CommonRequired bool
	}
	if json.Unmarshal(raw, &idx) != nil || len(idx.Templates) != 6 || idx.CommonRequired {
		t.Fatal("index")
	}
	ids := map[string]bool{}
	for _, x := range idx.Templates {
		data, e := os.ReadFile("../../../examples/sites/" + x.File)
		if e != nil {
			t.Fatal(e)
		}
		c, e := Decode(data, []byte(`{}`))
		if e != nil || c.ID != x.ID || ids[c.ID] {
			t.Fatal(c, e)
		}
		ids[c.ID] = true
	}
}
func TestSummaryResponseIdentityCannotFollowRedirectToAnotherPage(t *testing.T) {
	c := minimalTemplateClient(t, "ssd", "valid", "free")
	base, _ := url.Parse(c.Config.Domain)
	rule := c.Config.Requests["seeding_statistics"]
	c.HTTP = &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
		target := *base
		target.Path = "/elsewhere"
		copy := *r
		copy.URL = &target
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`<b>2</b><b>2 GiB</b>`)), Header: http.Header{}, Request: &copy}, nil
	})}
	_, _, _, complete, _ := c.collectSeeding(context.Background(), rule, map[string]string{"userId": "42", "domain": c.Config.Domain})
	if complete {
		t.Fatal("wrong identity accepted")
	}
}

func TestMinimalTemplatesRequireLoginBeforeSearching(t *testing.T) {
	for _, site := range minimalSites {
		t.Run(site, func(t *testing.T) {
			c := minimalTemplateClient(t, site, "valid", "free")
			requests := 0
			c.HTTP = &http.Client{Transport: metricsTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.URL.Path != "" && r.URL.Path != "/" {
					t.Fatal("unverified login performed business request", r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`<form action="login.php">登录</form>`)), Header: http.Header{}, Request: r}, nil
			})}
			if _, e := c.SearchAll(context.Background(), SearchQuery{Keyword: "Owned"}); !errors.Is(e, ErrAuthentication) || requests != 1 {
				t.Fatal("unverified search", e, requests)
			}
		})
	}
}
func TestMinimalTemplatesPreserveReleaseTypeAndMultiDigitHR(t *testing.T) {
	for _, site := range minimalSites {
		for _, release := range []string{"Owned.Movie.2026.2160p.WEB-DL.x265-TEAM", "Owned.Show.2026.S01E03.1080p.WEB-DL.x265-TEAM"} {
			t.Run(site+"/"+release, func(t *testing.T) {
				c := minimalSiteConfig(t, site)
				body := strings.ReplaceAll(minimalResourcePage(site, "11", "hr14", ""), "Owned.Movie.2026.1080p.WEB-DL.x265-TEAM", release)
				doc, e := parseDocument([]byte(body), "CssSelector")
				if e != nil {
					t.Fatal(e)
				}
				rule := c.Requests["search"]
				rows, e := doc.values(text(object(rule["list"])["selector"]), nil)
				if e != nil || len(rows) != 1 {
					t.Fatal(rows, e)
				}
				fields, e := mapFieldsSelected(doc, object(rule["fields"]), rows[0], nil, resourceFieldNames, nil)
				if e != nil {
					t.Fatal(e)
				}
				resource, ok := (Client{Config: c, Account: Account{Endpoint: c.Domain}}).resource(fields)
				if !ok || resource.Title != release {
					t.Fatal(resource)
				}
				hint := releasename.Parse(resource.Title)
				kind := "movie"
				if strings.Contains(release, "S01E03") {
					kind = "series"
					if hint.Season != 1 || hint.Episode != 3 {
						t.Fatal(hint)
					}
				}
				if hint.MediaType != kind || hint.Year != "2026" {
					t.Fatal(hint)
				}
				if site == "chdbits" || site == "ttg" {
					if text(fields["hr_days"]) != "14" {
						t.Fatal("truncated H&R duration", fields["hr_days"])
					}
				}
			})
		}
	}
}
func TestSummaryDeclarationsAndPageScopedNumbersCannotProduceTotals(t *testing.T) {
	for _, site := range []string{"ssd", "ubits"} {
		for _, bad := range []string{"page", "scope", "list", "missing_size", "non_get", "bad_summary"} {
			t.Run(site+"/"+bad, func(t *testing.T) {
				c := minimalTemplateClient(t, site, "valid", "free")
				rule := clone(c.Config.Requests["seeding_statistics"])
				switch bad {
				case "page":
					rule["path"] = text(rule["path"]) + "&page=1"
				case "scope":
					rule["path"] = text(rule["path"]) + "&limit=20"
				case "list":
					rule["list"] = Map{"selector": "//tr"}
				case "missing_size":
					delete(object(rule["fields"]), "size")
				case "non_get":
					rule["method"] = "POST"
				case "bad_summary":
					rule["summary"] = true
				}
				_, _, _, complete, _ := c.collectSeeding(context.Background(), rule, map[string]string{"userId": "42", "domain": c.Config.Domain})
				if complete {
					t.Fatal("unproved complete totals", bad)
				}
			})
		}
	}
}

func TestMinimalListStatisticsFollowOnlyIdentityBoundActualPages(t *testing.T) {
	for _, site := range []string{"chdbits", "ourbits", "ttg"} {
		t.Run(site, func(t *testing.T) {
			c := minimalTemplateClient(t, site, "valid", "free")
			transport := c.HTTP.Transport
			pages := 0
			c.HTTP.Transport = metricsTransport(func(r *http.Request) (*http.Response, error) {
				response, e := transport.RoundTrip(r)
				if e != nil {
					return response, e
				}
				target := r.URL.Path == "/getusertorrentlistajax.php" || site == "ttg" && r.URL.Path == "/userdetails.php"
				if !target {
					return response, nil
				}
				raw, e := io.ReadAll(response.Body)
				response.Body.Close()
				if e != nil {
					return nil, e
				}
				body := string(raw)
				if r.URL.Query().Get("page") == "1" {
					body = strings.ReplaceAll(body, "Owned", "Owned-2")
					pages++
				} else {
					q := r.URL.Query()
					q.Set("page", "1")
					body += `<a href="` + r.URL.Path + "?" + strings.ReplaceAll(q.Encode(), "&", "&amp;") + `">下一页</a>`
				}
				response.Body = io.NopCloser(strings.NewReader(body))
				return response, nil
			})
			m, e := c.Metrics(context.Background())
			if e != nil || !m.SeedingComplete || m.Seeding == nil || *m.Seeding != 2 || m.SeedingBytes == nil || *m.SeedingBytes != 4<<30 || m.SeedingPages != 2 || pages != 1 {
				raw, _ := json.Marshal(m)
				t.Fatal(string(raw), e, pages)
			}
		})
	}
}
