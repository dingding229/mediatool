package sites

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
	"golang.org/x/net/http/httpguts"
)

type Account struct {
	ID, Endpoint, UserID, Cookie, APIKey, Passkey string
	RequestHeaders                                map[string]string
}
type Client struct {
	Config                Config
	Account               Account
	HTTP                  *http.Client
	SourceRevision        int64
	DefinitionFingerprint string
}

func (c Client) base(rule Map) (*url.URL, error) {
	raw := c.Account.Endpoint
	if rule["use_api"] == true && c.Config.API != "" {
		raw = c.Config.API
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return nil, ErrConfig
	}
	return u, nil
}
func (c Client) allowed(target *url.URL) bool {
	if target.User != nil || target.Scheme != "http" && target.Scheme != "https" {
		return false
	}
	for _, raw := range append([]string{c.Account.Endpoint, c.Config.Domain, c.Config.API}, c.Config.AdditionalDomains...) {
		u, err := url.Parse(raw)
		if err == nil && sameRequestOrigin(u, target) {
			return true
		}
	}
	return false
}
func (c Client) call(ctx context.Context, rule Map, vars map[string]string, customJSON any) ([]byte, string, error) {
	return c.callWithTarget(ctx, rule, vars, customJSON, nil)
}

// callWithTarget exposes only the final, allowed request URL to the feed parser,
// so relative links are resolved against the feed after redirects, not the site root.
func (c Client) callWithTarget(ctx context.Context, rule Map, vars map[string]string, customJSON any, finalTarget **url.URL) ([]byte, string, error) {
	if err := validateRequestExecution(rule); err != nil {
		return nil, "", err
	}
	base, err := c.base(rule)
	if err != nil {
		return nil, "", err
	}
	relative, err := url.Parse(substitute(text(rule["path"]), vars))
	if err != nil {
		return nil, "", ErrConfig
	}
	target := base.ResolveReference(relative)
	if !c.allowed(target) {
		return nil, "", ErrConfig
	}
	method := text(rule["method"])
	if method == "" {
		method = "GET"
	}
	values := url.Values{}
	for key, value := range object(rule["params"]) {
		entries, err := parameterValues(value, vars)
		if err != nil {
			return nil, "", err
		}
		for _, entry := range entries {
			values.Add(key, entry)
		}
	}
	query := target.Query()
	body, contentType, err := requestPayload(rule, vars, customJSON)
	if err != nil {
		return nil, "", err
	}
	for key, list := range values {
		for _, entry := range list {
			query.Add(key, entry)
		}
	}
	target.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, "", ErrRequest
	}
	declared, rawHeaders, err := declaredRequestHeaders(rule, vars)
	if err != nil {
		return nil, "", err
	}
	request.Header = declared
	if !sameRequestOrigin(base, target) && (len(rawHeaders) > 0 || request.Body != nil || request.Method != "GET" && request.Method != "HEAD") {
		// The definition's request material belongs to its selected base/API
		// origin. An allowed download origin is not implicit credential consent.
		return nil, "", ErrConfig
	}
	requiredHeaders, err := requiredRequestHeaderNames(rule)
	if err != nil {
		return nil, "", err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if c.Account.Cookie != "" {
		accountURL, _ := url.Parse(c.Account.Endpoint)
		if accountURL != nil && sameRequestOrigin(accountURL, target) {
			request.Header.Set("Cookie", c.Account.Cookie)
		}
	}
	accountHeaders, err := normalizedAccountHeaders(c.Account.RequestHeaders)
	if err != nil {
		return nil, "", ErrConfig
	}
	for _, header := range requiredHeaders {
		if !sameRequestOrigin(base, target) {
			return nil, "", ErrConfig
		}
		if (strings.EqualFold(header, "x-api-key") || strings.EqualFold(header, "apitoken")) && c.Account.APIKey != "" {
			if !httpguts.ValidHeaderFieldValue(c.Account.APIKey) {
				return nil, "", ErrConfig
			}
			request.Header.Set(header, c.Account.APIKey)
		} else if supplied, exists := accountHeaders[header]; exists {
			// Encrypted operator values are literal, not templates. Only names
			// required by this exact request are attached at its selected origin.
			request.Header.Set(header, supplied)
		} else if strings.TrimSpace(request.Header.Get(header)) == "" || !completeHeaderTemplate(rawHeaders[header], vars) {
			return nil, "", ErrAuthentication
		}
	}
	if _, configured := request.Header["User-Agent"]; !configured {
		request.Header.Set("User-Agent", "MediaTool/0.3")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	copy := *client
	// Cookie is an explicit, origin-bound account credential. A caller's Jar
	// must not silently broaden it to another origin after our redirect checks.
	copy.Jar = nil
	leftOrigin := false
	copy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || !c.allowed(req.URL) {
			return ErrRequest
		}
		if len(via) == 0 {
			return nil
		}
		previous := via[len(via)-1].URL
		if strings.EqualFold(previous.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
			return ErrRequest
		}
		if !sameRequestOrigin(req.URL, via[0].URL) {
			leftOrigin = true
		}
		if leftOrigin {
			// 307/308 may replay the original POST/form/JSON bytes; they can
			// contain credentials even if the sensitive headers were removed.
			if req.Method != "GET" && req.Method != "HEAD" || req.Body != nil {
				return ErrRequest
			}
			for key := range rawHeaders {
				req.Header.Del(key)
			}
			for _, key := range requiredHeaders {
				req.Header.Del(key)
			}
			for _, key := range []string{"Cookie", "Authorization", "Proxy-Authorization", "X-Api-Key", "APITOKEN", "X-Csrf-Token", "Referer"} {
				req.Header.Del(key)
			}
			// Go may re-copy initial headers on later redirects. Once the chain
			// leaves the credential origin, no later hop restores them.
			req.Header.Set("User-Agent", "MediaTool/0.3")
		}
		return nil
	}
	response, err := copy.Do(request)
	if err != nil {
		return nil, "", ErrRequest
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, "", ErrAuthentication
	}
	if response.StatusCode != 200 {
		return nil, "", ErrRequest
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(raw) > 8<<20 {
		return nil, "", ErrRequest
	}
	parser, _ := requestParser(rule)
	if !strings.EqualFold(parser, "JsonPath") && !strings.EqualFold(parser, "None") {
		var reader io.Reader
		var err error
		if c.Config.Encoding != "" {
			reader, err = charset.NewReaderLabel(c.Config.Encoding, bytes.NewReader(raw))
		} else {
			reader, err = charset.NewReader(bytes.NewReader(raw), response.Header.Get("Content-Type"))
		}
		if err != nil {
			return nil, "", ErrRequest
		}
		raw, err = io.ReadAll(io.LimitReader(reader, (8<<20)+1))
		if err != nil || len(raw) > 8<<20 {
			return nil, "", ErrRequest
		}
	}
	if finalTarget != nil {
		resolved := request.URL
		if response.Request != nil && response.Request.URL != nil {
			resolved = response.Request.URL
		}
		if !c.allowed(resolved) {
			return nil, "", ErrRequest
		}
		copy := *resolved
		*finalTarget = &copy
	}
	return raw, parser, nil
}
func (c Client) vars(keyword string, page int) map[string]string {
	return map[string]string{"keyword": keyword, "page": text(page), "userId": c.Account.UserID, "passkey": c.Account.Passkey}
}
func (c Client) Test(ctx context.Context) error {
	rule := c.Config.Requests["user_basic_info"]
	fields := Map{}
	if c.Config.Schema == "mTorrent" {
		rule = c.Config.Requests["profile"]
		if c.Account.APIKey == "" || c.Account.UserID == "" {
			return ErrAuthentication
		}
		fields = Map{"username": object(object(rule["fields"])["username"])}
	} else {
		fields = Map{"is_login": object(object(rule["fields"])["is_login"])}
	}
	if err := validateDocumentRequest(rule); err != nil {
		return err
	}
	raw, mode, err := c.call(ctx, rule, c.vars("", 1), nil)
	if err != nil {
		return err
	}
	d, err := parseDocument(raw, mode)
	if err != nil {
		return err
	}
	values, err := mapFields(d, fields, nil, c.vars("", 1))
	if err != nil {
		return err
	}
	if c.Config.Schema == "mTorrent" {
		if blank(values["username"]) {
			return ErrAuthentication
		}
	} else if values["is_login"] != true {
		return ErrAuthentication
	}
	return nil
}
func (c Client) Search(ctx context.Context, q SearchQuery) ([]Resource, error) {
	page, err := c.searchPage(ctx, q)
	return page.Resources, err
}

func (c Client) searchPage(ctx context.Context, q SearchQuery) (SearchPageResult, error) {
	result := SearchPageResult{Resources: []Resource{}}
	if q.Page < 1 {
		q.Page = 1
	}
	rule := c.Config.Requests["search"]
	if err := validateDocumentRequest(rule); err != nil {
		return result, err
	}
	vars := c.vars(q.Keyword, q.Page)
	custom := c.defaultSearchJSON(q)
	if _, _, err := requestPayload(rule, vars, custom); err != nil {
		return result, err
	}
	if err := c.Test(ctx); err != nil {
		return result, err
	}
	raw, mode, err := c.call(ctx, rule, vars, custom)
	if err != nil {
		return result, err
	}
	d, err := parseDocument(raw, mode)
	if err != nil {
		return result, err
	}
	selector := text(object(rule["list"])["selector"])
	if strings.TrimSpace(selector) == "" {
		return result, ErrConfig
	}
	var list []any
	if strings.EqualFold(mode, "JsonPath") {
		selected, exists := d.jsonValue(selector, nil)
		var valid bool
		list, valid = selected.([]any)
		if !exists || !valid {
			return result, ErrSearchResponse
		}
	} else {
		list, err = d.values(selector, nil)
	}
	if err != nil {
		return result, err
	}
	result.RawRows = len(list)
	out := []Resource{}
	parentRules := object(rule["fields"])
	flat := object(rule["flat_map_fields"])
	for _, row := range list {
		fields, err := mapFieldsSelected(d, parentRules, row, vars, resourceFieldNames, nil)
		if err != nil {
			return result, err
		}
		rows := []any{row}
		if len(flat) > 0 {
			rows, err = d.values(text(flat["selector"]), row)
			if err != nil {
				return result, err
			}
		}
		for _, child := range rows {
			combined := fields
			if len(flat) > 0 {
				inner, err := mapFieldsSelected(d, object(flat["fields"]), child, vars, resourceFieldNames, fields)
				if err != nil {
					return result, err
				}
				combined = clone(fields)
				for key, value := range inner {
					combined[key] = value
				}
			}
			resource, ok := c.resource(combined)
			if !ok {
				continue
			}
			if len(out) == SearchResourceLimit {
				result.Resources = out
				result.Truncated = true
				return result, nil
			}
			out = append(out, resource)
		}
	}
	if len(list) > 0 && len(out) == 0 {
		return result, ErrSearchResponse
	}
	result.Resources = out
	return result, nil
}

var resourceFieldNames = []string{"id", "title", "download", "size", "seeders", "leechers", "description", "details", "hr", "hr_days", "downloadvolumefactor", "labels"}

func (c Client) resource(fields Map) (Resource, bool) {
	id, title := text(fields["id"]), text(fields["title"])
	if strings.TrimSpace(id) == "" || strings.TrimSpace(title) == "" {
		return Resource{}, false
	}
	size, _ := checkedInteger(fields["size"], 63)
	seeders, _ := checkedInteger(fields["seeders"], 31)
	leechers, _ := checkedInteger(fields["leechers"], 31)
	resource := Resource{SiteID: c.Config.ID, TorrentID: id, Title: title, DownloadURL: c.resolveURL(text(fields["download"])), Size: size, Seeders: int(seeders), Leechers: int(leechers), Subtitle: text(fields["description"]), DetailsURL: c.resolveURL(text(fields["details"]))}
	if c.Config.HasHR != nil && !*c.Config.HasHR {
		known := false
		resource.HR = &known
	} else if value, ok := fields["hr"].(bool); ok {
		resource.HR = &value
	} else if days, valid := checkedInteger(fields["hr_days"], 31); valid {
		has := days > 0
		resource.HR = &has
	}
	if factor, valid := checkedNumber(fields["downloadvolumefactor"]); valid && factor >= 0 {
		resource.DownloadFactor = &factor
	}
	seen := map[string]bool{}
	for _, raw := range slice(fields["labels"]) {
		label := strings.TrimSpace(text(raw))
		if label != "" && !seen[label] {
			resource.Labels = append(resource.Labels, label)
			seen[label] = true
		}
	}
	return resource, true
}

func (c Client) resolveURL(raw string) string {
	if raw == "" {
		return ""
	}
	base, _ := url.Parse(c.Account.Endpoint)
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	target := base.ResolveReference(u)
	if !c.allowed(target) {
		return ""
	}
	return target.String()
}
func (c Client) Torrent(ctx context.Context, id, downloadHint string) ([]byte, error) {
	if c.Config.Schema == "mTorrent" {
		rule := c.Config.Requests["gen_dl_token"]
		if err := validateDocumentRequest(rule); err != nil {
			return nil, err
		}
		raw, mode, err := c.call(ctx, rule, map[string]string{"id": id, "userId": c.Account.UserID}, nil)
		if err != nil {
			return nil, err
		}
		d, err := parseDocument(raw, mode)
		if err != nil {
			return nil, err
		}
		fields, err := mapFields(d, object(rule["fields"]), nil, nil)
		if err != nil {
			return nil, err
		}
		downloadHint = text(fields["url"])
	}
	if downloadHint == "" {
		return nil, errors.New("site did not provide a supported torrent download URL")
	}
	target, err := url.Parse(downloadHint)
	if err != nil || !c.allowed(target) {
		return nil, ErrConfig
	}
	requestRule := Map{"path": downloadHint, "method": "GET", "parser": "None"}
	raw, _, err := c.call(ctx, requestRule, nil, nil)
	return raw, err
}
