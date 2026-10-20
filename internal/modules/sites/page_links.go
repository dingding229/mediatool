package sites

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/antchfx/htmlquery"
)

// Pagination links are never generic credential-bearing URLs: only a GET page
// cursor on this exact query/path/origin is allowed to change.
func validatedPageLink(current *url.URL, raw string) (*url.URL, error) {
	if current == nil || len(raw) > 4096 || strings.ContainsAny(raw, "\\\x00\r\n") {
		return nil, ErrConfig
	}
	ref, err := url.Parse(raw)
	if err != nil || ref.User != nil || ref.Fragment != "" {
		return nil, ErrConfig
	}
	next := current.ResolveReference(ref)
	if !sameRequestOrigin(current, next) || next.Path != current.Path || next.RawPath != current.RawPath {
		return nil, ErrConfig
	}
	before, err := url.ParseQuery(current.RawQuery)
	if err != nil {
		return nil, ErrConfig
	}
	after, err := url.ParseQuery(next.RawQuery)
	if err != nil {
		return nil, ErrConfig
	}
	cursor := regexp.MustCompile(`^(page|pagenumber|page_number|offset|start)$`)
	changed := false
	for key, values := range after {
		if len(values) != 1 {
			return nil, ErrConfig
		}
		if cursor.MatchString(strings.ToLower(key)) {
			if !regexp.MustCompile(`^\d{1,7}$`).MatchString(values[0]) {
				return nil, ErrConfig
			}
			if before.Get(key) != values[0] {
				changed = true
			}
			continue
		}
		if len(before[key]) != 1 || before.Get(key) != values[0] {
			return nil, ErrConfig
		}
	}
	for key, values := range before {
		if cursor.MatchString(strings.ToLower(key)) {
			continue
		}
		if len(values) != 1 || len(after[key]) != 1 || after.Get(key) != values[0] {
			return nil, ErrConfig
		}
	}
	if !changed {
		return nil, ErrConfig
	}
	next.RawQuery = after.Encode()
	return next, nil
}

func nexusCompatible(c Config) bool {
	return c.ID == "hdsky" || c.Schema == "hdsky" || c.Schema == "NexusPHP"
}

// linkNavigation is explicit for declared next_page fields. The narrow built-in
// Nexus/HDSKY fallback follows only an actual next anchor in a known result DOM.
func (c Client) linkNavigation(d document, rule Map, vars map[string]string, target *url.URL) (*url.URL, bool, bool, error) {
	list := object(rule["list"])
	var raw string
	if next, ok := list["next_page"]; ok {
		field := object(next)
		if len(field) == 0 {
			return nil, true, false, ErrConfig
		}
		values, err := mapFields(d, Map{"next": field}, nil, vars)
		if err != nil {
			return nil, true, false, err
		}
		if blank(values["next"]) {
			return nil, true, true, nil
		}
		value, ok := values["next"].(string)
		if !ok {
			return nil, true, false, ErrConfig
		}
		raw = value
	} else if nexusCompatible(c.Config) && d.html != nil && target != nil && target.Path == "/torrents.php" {
		containers, err := htmlquery.QueryAll(d.html, `//table[contains(concat(' ',normalize-space(@class),' '),' torrents ')]`)
		if err != nil || len(containers) != 1 {
			return nil, false, false, nil
		}
		links, err := htmlquery.QueryAll(d.html, `//a[@href]`)
		if err != nil {
			return nil, true, false, ErrConfig
		}
		for _, link := range links {
			label := strings.Join(strings.Fields(htmlquery.InnerText(link)), "")
			if !regexp.MustCompile(`(?i)^(?:下一[页頁]|next|nextpage|[›»]|下一[页頁][›»])$`).MatchString(label) {
				continue
			}
			for _, a := range link.Attr {
				if a.Key == "href" {
					if raw != "" && raw != a.Val {
						return nil, true, false, ErrConfig
					}
					raw = a.Val
				}
			}
		}
		if raw == "" {
			return nil, true, true, nil
		}
	} else {
		return nil, false, false, nil
	}
	next, err := validatedPageLink(target, raw)
	return next, true, false, err
}

func relativeRowSelector(selector string) string {
	// Upstream seeding fields use /td[n] relative to a selected tr. htmlquery's
	// absolute XPath would resolve the document root; make this contract explicit.
	if strings.HasPrefix(selector, "/") && !strings.HasPrefix(selector, "//") {
		return "." + selector
	}
	return selector
}
