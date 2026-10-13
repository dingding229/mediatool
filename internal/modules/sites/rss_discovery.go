package sites

import (
	"context"
	"errors"
	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
	"net/url"
	"sort"
	"strings"
)

func (c Client) ValidateRSS(ctx context.Context, raw string) (string, error) {
	u, e := url.Parse(strings.TrimSpace(raw))
	base, b := url.Parse(c.Account.Endpoint)
	if e != nil || b != nil || u.User != nil || u.Fragment != "" {
		return "", ErrConfig
	}
	u = base.ResolveReference(u)
	// Some generator pages emit an HTTP link even when accessed over HTTPS.
	// Upgrade only that exact same-host URL, then prove the actual HTTPS feed.
	if base.Scheme == "https" && u.Scheme == "http" && u.Host == base.Host {
		u.Scheme = "https"
	}
	if !sameRequestOrigin(base, u) {
		return "", ErrConfig
	}
	path := strings.ToLower(u.Path)
	if !strings.Contains(path, "rss") || strings.HasSuffix(path, "getrss.php") || strings.HasSuffix(path, "rsstools.php") || strings.HasSuffix(path, "myrss.php") {
		return "", ErrConfig
	}
	if _, e = c.RSS(ctx, u.String()); e != nil {
		return "", e
	}
	return u.String(), nil
}
func feedCandidates(doc *html.Node) []string {
	nodes, e := htmlquery.QueryAll(doc, `//a[contains(@href,'rss')]/@href | //input[contains(@value,'rss')]/@value | //textarea`)
	if e != nil {
		return nil
	}
	out := []string{}
	for _, n := range nodes {
		v := strings.TrimSpace(htmlquery.InnerText(n))
		if len(v) > 0 && len(v) <= 8192 {
			out = append(out, v)
		}
	}
	score := func(raw string) int {
		u, e := url.Parse(raw)
		if e != nil {
			return -1
		}
		n := 0
		for _, key := range []string{"passkey", "authkey", "rsskey", "token"} {
			if u.Query().Get(key) != "" {
				n += 10
			}
		}
		if u.Query().Get("linktype") == "dl" {
			n++
		}
		return n
	}
	sort.SliceStable(out, func(i, j int) bool { return score(out[i]) > score(out[j]) })
	return out
}
func (c Client) DiscoverRSS(ctx context.Context) (string, string, error) {
	t, ok := GoTemplate(c.Config.ID)
	if !ok {
		return "", "", ErrConfig
	}
	if e := c.Test(ctx); e != nil {
		return "", "", e
	}
	vars := c.vars("", 1)
	values, e := c.readMetricFields(ctx, c.Config.Requests["user_basic_info"], []string{"id"}, vars)
	if e == nil && text(values["id"]) != "" {
		vars["userId"] = text(values["id"])
	}
	seen := map[string]bool{}
	validate := func(candidates []string) (string, bool) {
		for _, raw := range candidates {
			if seen[raw] {
				continue
			}
			seen[raw] = true
			if len(seen) > 32 {
				return "", false
			}
			if out, e := c.ValidateRSS(ctx, raw); e == nil {
				return out, true
			}
		}
		return "", false
	}
	for _, path := range append([]string{""}, t.RSS.Pages...) {
		var target *url.URL
		raw, mode, e := c.callWithTarget(ctx, Map{"parser": "XPath", "method": "GET", "path": path}, vars, nil, &target)
		if e != nil {
			continue
		}
		doc, e := parseDocument(raw, mode)
		if e != nil || doc.html == nil {
			continue
		}
		if out, ok := validate(feedCandidates(doc.html)); ok {
			return out, vars["userId"], nil
		}
		rule, ok := rssFormRule(doc.html, target, text(t.RSS.Generate["path"]), text(t.RSS.Generate["check_all_prefix"]))
		if !ok {
			continue
		}
		if t.RSS.FormFeedPath != "" {
			u, _ := url.Parse(t.RSS.FormFeedPath)
			u = target.ResolveReference(u)
			q := url.Values{}
			controls := object(rule["form_data"])
			if len(controls) == 0 {
				controls = object(rule["params"])
			}
			for name, value := range controls {
				entries, err := parameterValues(value, vars)
				if err != nil {
					continue
				}
				for _, v := range entries {
					if v != "" {
						q.Add(name, v)
					}
				}
			}
			for name, value := range t.RSS.FormFeedParams {
				entries, err := parameterValues(value, vars)
				if err != nil {
					continue
				}
				q.Del(name)
				for _, v := range entries {
					q.Add(name, v)
				}
			}
			u.RawQuery = q.Encode()
			if out, ok := validate([]string{u.String()}); ok {
				return out, vars["userId"], nil
			}
			continue
		}
		raw, mode, e = c.call(ctx, rule, vars, nil)
		if e != nil {
			continue
		}
		generated, e := parseDocument(raw, mode)
		if e != nil || generated.html == nil {
			continue
		}
		if out, ok := validate(feedCandidates(generated.html)); ok {
			return out, vars["userId"], nil
		}
	}
	return "", vars["userId"], errors.New("未取得可验证的RSS地址，可手动填写同站RSS后保存验证")
}

// Reproduce only a declared generator form's successful controls (including
// per-session hidden CSRF fields). Never submit profile/preferences forms.
func rssFormRule(doc *html.Node, page *url.URL, generator string, checkAllPrefix string) (Map, bool) {
	if page == nil || generator == "" {
		return nil, false
	}
	expected, e := url.Parse(generator)
	if e != nil {
		return nil, false
	}
	expected = page.ResolveReference(expected)
	forms, _ := htmlquery.QueryAll(doc, "//form")
	for _, form := range forms {
		action := htmlquery.SelectAttr(form, "action")
		u, e := url.Parse(action)
		if e != nil {
			continue
		}
		u = page.ResolveReference(u)
		if !sameRequestOrigin(page, u) || u.Path != expected.Path {
			continue
		}
		method := strings.ToUpper(htmlquery.SelectAttr(form, "method"))
		if method == "" {
			method = "GET"
		}
		if method != "GET" && method != "POST" {
			continue
		}
		fields := Map{}
		nodes, _ := htmlquery.QueryAll(form, ".//input[@name] | .//select[@name] | .//textarea[@name]")
		for _, n := range nodes {
			if len(fields) > 160 {
				return nil, false
			}
			name := htmlquery.SelectAttr(n, "name")
			if len(name) > 100 || strings.ContainsAny(name, "\r\n\x00") {
				return nil, false
			}
			if hasAttr(n, "disabled") {
				continue
			}
			typ := strings.ToLower(htmlquery.SelectAttr(n, "type"))
			if typ == "file" || typ == "submit" || typ == "button" || typ == "reset" {
				continue
			}
			if (typ == "checkbox" || typ == "radio") && !hasAttr(n, "checked") && !(typ == "checkbox" && checkAllPrefix != "" && strings.HasPrefix(name, checkAllPrefix) && numericNameSuffix(strings.TrimPrefix(name, checkAllPrefix))) {
				continue
			}
			value := htmlquery.SelectAttr(n, "value")
			if n.Data == "select" {
				options, _ := htmlquery.QueryAll(n, ".//option[@selected]")
				if len(options) == 0 {
					options, _ = htmlquery.QueryAll(n, ".//option[1]")
				}
				if len(options) > 0 {
					value = htmlquery.SelectAttr(options[0], "value")
				}
			}
			if n.Data == "textarea" {
				value = htmlquery.InnerText(n)
			}
			if typ == "checkbox" && value == "" {
				value = "on"
			}
			if len(value) > 8192 {
				return nil, false
			}
			if old, ok := fields[name].([]any); ok {
				fields[name] = append(old, value)
			} else {
				fields[name] = []any{value}
			}
		}
		rule := Map{"parser": "XPath", "method": method, "path": u.String(), "headers": Map{"Referer": page.String(), "Origin": page.Scheme + "://" + page.Host}}
		if method == "POST" {
			rule["form_data"] = fields
		} else {
			rule["params"] = fields
		}
		return rule, true
	}
	return nil, false
}
func hasAttr(n *html.Node, name string) bool {
	for _, a := range n.Attr {
		if a.Key == name {
			return true
		}
	}
	return false
}

func numericNameSuffix(s string) bool {
	if len(s) == 0 || len(s) > 8 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (c Client) AuthenticatedUserID(ctx context.Context) (string, error) {
	values, err := c.readMetricFields(ctx, c.Config.Requests["user_basic_info"], []string{"id"}, c.vars("", 1))
	return text(values["id"]), err
}
