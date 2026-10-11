package sites

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strings"
)

// Explicit definition payloads own their contract. Framework-generated JSON is
// only a fallback for a definition that declares neither body nor form_data.
func requestPayload(rule Map, vars map[string]string, fallbackJSON any) (io.Reader, string, error) {
	body, hasBody := rule["body"]
	form, hasForm := rule["form_data"]
	if hasBody && hasForm {
		return nil, "", ErrConfig
	}
	if hasBody {
		resolved, err := substituteJSON(body, vars, 0)
		if err != nil {
			return nil, "", err
		}
		raw, err := json.Marshal(resolved)
		if err != nil {
			return nil, "", ErrConfig
		}
		return bytes.NewReader(raw), "application/json", nil
	}
	if hasForm {
		mapping, ok := form.(map[string]any)
		if !ok {
			return nil, "", ErrConfig
		}
		values := url.Values{}
		for key, value := range mapping {
			entries, err := parameterValues(value, vars)
			if err != nil {
				return nil, "", err
			}
			for _, entry := range entries {
				values.Add(key, entry)
			}
		}
		return strings.NewReader(values.Encode()), "application/x-www-form-urlencoded", nil
	}
	if fallbackJSON != nil {
		raw, err := json.Marshal(fallbackJSON)
		if err != nil {
			return nil, "", ErrConfig
		}
		return bytes.NewReader(raw), "application/json", nil
	}
	return nil, "", nil
}

func (c Client) defaultSearchJSON(q SearchQuery) any {
	rule := c.Config.Requests["search"]
	_, body := rule["body"]
	_, form := rule["form_data"]
	if c.Config.Schema != "mTorrent" || body || form {
		return nil
	}
	return Map{"mode": "normal", "keyword": q.Keyword, "pageNumber": q.Page, "pageSize": 100, "categories": []string{}}
}

func usesPagePlaceholder(value any) bool {
	switch v := value.(type) {
	case string:
		return strings.Contains(v, "{page}")
	case Map:
		for _, entry := range v {
			if usesPagePlaceholder(entry) {
				return true
			}
		}
	case []any:
		for _, entry := range v {
			if usesPagePlaceholder(entry) {
				return true
			}
		}
	}
	return false
}
func (c Client) searchHasPagination() bool {
	rule := c.Config.Requests["search"]
	for _, key := range []string{"path", "params", "body", "form_data"} {
		if usesPagePlaceholder(rule[key]) {
			return true
		}
	}
	return c.defaultSearchJSON(SearchQuery{}) != nil
}
