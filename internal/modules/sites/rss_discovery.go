package sites

import (
	"context"
	"errors"
	"github.com/antchfx/htmlquery"
	"net/url"
	"strings"
)

func (c Client) DiscoverRSS(ctx context.Context) (string, string, error) {
	t, ok := GoTemplate(c.Config.ID)
	if !ok {
		return "", "", ErrConfig
	}
	if e := c.Test(ctx); e != nil {
		return "", "", e
	}
	vars := c.vars("", 1)
	basic := c.Config.Requests["user_basic_info"]
	values, e := c.readMetricFields(ctx, basic, []string{"id"}, vars)
	if e != nil {
		return "", "", e
	}
	if id := text(values["id"]); id != "" {
		vars["userId"] = id
	}
	validate := func(raw string) (string, error) {
		u, e := url.Parse(strings.TrimSpace(raw))
		if e != nil || u.User != nil || u.Fragment != "" {
			return "", ErrConfig
		}
		base, e := url.Parse(c.Account.Endpoint)
		if e != nil {
			return "", ErrConfig
		}
		u = base.ResolveReference(u)
		if !sameRequestOrigin(base, u) {
			return "", ErrConfig
		}
		path := strings.ToLower(u.Path)
		if !strings.Contains(path, "rss") || strings.HasSuffix(path, "getrss.php") {
			return "", ErrConfig
		}
		if _, e := c.RSS(ctx, u.String()); e != nil {
			return "", e
		}
		return u.String(), nil
	}
	for _, path := range append([]string{""}, t.RSS.Pages...) {
		raw, _, e := c.call(ctx, Map{"parser": "XPath", "method": "GET", "path": path}, vars, nil)
		if e != nil {
			continue
		}
		doc, e := parseDocument(raw, "XPath")
		if e != nil {
			continue
		}
		nodes, e := htmlquery.QueryAll(doc.html, `//a[contains(@href,'rss')]/@href | //input[contains(@value,'rss')]/@value`)
		if e != nil {
			continue
		}
		for _, node := range nodes {
			if result, e := validate(htmlquery.InnerText(node)); e == nil {
				return result, vars["userId"], nil
			}
		}
	}
	if len(t.RSS.Generate) > 0 {
		raw, mode, e := c.call(ctx, t.RSS.Generate, vars, nil)
		if e == nil {
			doc, e := parseDocument(raw, mode)
			if e == nil {
				values, e := mapFields(doc, object(t.RSS.Generate["fields"]), nil, vars)
				if e == nil {
					if result, e := validate(text(values["content"])); e == nil {
						return result, vars["userId"], nil
					}
				}
			}
		}
	}
	return "", vars["userId"], errors.New("站点未返回可确认的RSS地址；不会生成猜测链接")
}
