// Package wishlist is a fixed-provider read facade. It exposes identities, not pages or credentials.
package wishlist

import (
	"context"
	"errors"
	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
	"io"
	"media-tool/internal/adapters/tmdb"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/playback"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Request struct {
	Source, ID string
	Page       int
}
type Entry struct {
	SourceID, Title, Year, MediaType string
	TMDBID                           int
}
type Page struct {
	Items             []Entry
	HasMore           bool
	State             string
	RetryAfterSeconds int
}
type Service struct{ Connections *integrations.Manager }

func idOK(s string, numeric bool) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			continue
		}
		if !numeric && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_') {
			continue
		}
		return false
	}
	return true
}
func (s Service) Fetch(ctx context.Context, in Request) (Page, error) {
	out := Page{Items: []Entry{}, State: "unavailable"}
	if s.Connections == nil || in.Page < 1 || in.Page > 1000 || !idOK(in.ID, in.Source != "douban-wish") {
		return out, errors.New("invalid source selection")
	}
	release, ok := admit(ctx)
	if !ok {
		return out, errors.New("source request budget")
	}
	defer release()
	if in.Source == "tmdb-list" {
		c, e := s.Connections.TMDB(ctx)
		if e != nil {
			return out, e
		}
		id, e := strconv.Atoi(in.ID)
		if e != nil {
			return out, e
		}
		items, more, e := c.PublicList(ctx, id, in.Page)
		if e != nil {
			var limited *tmdb.ResponseError
			if errors.As(e, &limited) && limited.Status == 429 {
				out.State = "limited"
				out.RetryAfterSeconds = 300
				return out, nil
			}
			return out, e
		}
		for _, v := range items {
			out.Items = append(out.Items, Entry{SourceID: fmtID(v.MediaType, v.ID), Title: strings.ReplaceAll(v.Title, c.Token, "[redacted]"), Year: v.Year, MediaType: v.MediaType, TMDBID: v.ID})
		}
		out.State = "ok"
		out.HasMore = more
		return out, nil
	}
	var path string
	switch in.Source {
	case "douban-wish":
		path = "/people/" + in.ID + "/wish?start=" + strconv.Itoa((in.Page-1)*15) + "&sort=time&rating=all&filter=all&mode=grid"
	case "douban-subject":
		if in.Page != 1 {
			return out, errors.New("single subject has no pagination")
		}
		path = "/subject/" + in.ID + "/"
	default:
		return out, errors.New("source unsupported")
	}
	base, e := s.Connections.HTTPClient(ctx, "ptsite")
	if e != nil {
		return out, e
	}
	client := *base
	client.Jar = nil
	client.Timeout = 10 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	// This facade never accepts a URL. Bind public DNS at dial time unless the
	// operator has configured the existing proxy; controlled transports are tests.
	if s.Connections.OutboundHTTP == nil {
		tr := base.Transport.(*http.Transport).Clone()
		if tr.Proxy == nil {
			tr.DialContext = (playback.Policy{AllowedHosts: []string{"movie.douban.com"}}).DialContext
		}
		client.Transport = tr
		defer tr.CloseIdleConnections()
	}
	req, e := http.NewRequestWithContext(ctx, "GET", "https://movie.douban.com"+path, nil)
	if e != nil {
		return out, e
	}
	req.Header.Set("User-Agent", "MediaTool-PublicWishlist/2")
	req.Header.Set("Accept", "text/html")
	resp, e := client.Do(req)
	if e != nil {
		return out, e
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	closed := resp.Body.Close()
	if e != nil || closed != nil || len(raw) > 1<<20 {
		return out, errors.New("source body limit")
	}
	if resp.StatusCode == 429 {
		out.State = "limited"
		out.RetryAfterSeconds = 300
		return out, nil
	}
	if resp.StatusCode != 200 {
		out.State = "unavailable"
		return out, nil
	}
	return Parse(raw, in)
}
func fmtID(kind string, id int) string { return "tmdb:" + kind + ":" + strconv.Itoa(id) }
func text(n *html.Node) string {
	if n == nil {
		return ""
	}
	return strings.Join(strings.Fields(htmlquery.InnerText(n)), " ")
}
func year(s string) string { r := regexp.MustCompile(`(?:19|20)[0-9]{2}`); return r.FindString(s) }
func title(s string) string {
	s = strings.TrimSpace(strings.Split(s, " / ")[0])
	if len([]rune(s)) > 200 {
		return ""
	}
	return s
}

// Parse accepts only known public DOM structures. Login/captcha/markup drift is uncertain.
func Parse(raw []byte, in Request) (Page, error) {
	out := Page{Items: []Entry{}, State: "uncertain"}
	if len(raw) > 1<<20 {
		return out, errors.New("source body limit")
	}
	if !htmlBudget(raw) {
		return out, nil
	}
	doc, e := htmlquery.Parse(strings.NewReader(string(raw)))
	if e != nil {
		return out, e
	}
	if htmlquery.FindOne(doc, `//input[@type='password' or @name='captcha-solution']|//form[contains(@action,'login') or contains(@action,'captcha')]`) != nil {
		return out, nil
	}
	if in.Source == "douban-subject" {
		name := title(text(htmlquery.FindOne(doc, `//*[@property='v:itemreviewed']`)))
		y := year(text(htmlquery.FindOne(doc, `//span[@class='year']`)))
		if name == "" || y == "" {
			return out, nil
		}
		out.Items = []Entry{{SourceID: "douban:" + in.ID, Title: name, Year: y}}
		out.State = "ok"
		return out, nil
	}
	grid := htmlquery.FindOne(doc, `//div[contains(concat(' ',normalize-space(@class),' '),' grid-view ')]`)
	if grid == nil {
		return out, nil
	}
	nodes := htmlquery.Find(grid, `./div[contains(concat(' ',normalize-space(@class),' '),' item ')]`)
	if len(nodes) > 20 {
		return out, nil
	}
	seen := map[string]bool{}
	for _, n := range nodes {
		a := htmlquery.FindOne(n, `.//li[contains(concat(' ',normalize-space(@class),' '),' title ')]/a`)
		if a == nil {
			return out, nil
		}
		u, e := url.Parse(htmlquery.SelectAttr(a, "href"))
		if e != nil || u.Scheme != "https" || u.Host != "movie.douban.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return out, nil
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 2 || parts[0] != "subject" || !idOK(parts[1], true) {
			return out, nil
		}
		name := title(text(a))
		y := year(text(htmlquery.FindOne(n, `.//li[contains(concat(' ',normalize-space(@class),' '),' intro ')]`)))
		if name == "" || seen[parts[1]] {
			return out, nil
		}
		seen[parts[1]] = true
		out.Items = append(out.Items, Entry{SourceID: "douban:" + parts[1], Title: name, Year: y})
	}
	next := htmlquery.FindOne(doc, `//span[contains(concat(' ',normalize-space(@class),' '),' next ')]/a`)
	out.HasMore = next != nil
	out.State = "ok"
	return out, nil
}
