package sites

import (
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// Match HTTP origins, not just a host spelling. Case and an explicit default
// port do not create a new origin; a different scheme or port does.
func sameRequestOrigin(a, b *url.URL) bool {
	if a == nil || b == nil || a.User != nil || b.User != nil || !strings.EqualFold(a.Scheme, b.Scheme) || !strings.EqualFold(a.Hostname(), b.Hostname()) {
		return false
	}
	port := func(u *url.URL) string {
		if value := u.Port(); value != "" {
			return value
		}
		if strings.EqualFold(u.Scheme, "https") {
			return "443"
		}
		if strings.EqualFold(u.Scheme, "http") {
			return "80"
		}
		return ""
	}
	return port(a) == port(b)
}

// A definition may supply a required header explicitly. This executes that
// declaration, not a guessed bearer prefix, browser token or CSRF bootstrap.
func declaredRequestHeaders(rule Map, vars map[string]string) (http.Header, map[string]string, error) {
	headers := http.Header{}
	rawValues := map[string]string{}
	if raw, exists := rule["headers"]; exists {
		values, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, ErrConfig
		}
		for name, value := range values {
			literal, ok := value.(string)
			canonical := http.CanonicalHeaderKey(name)
			if !ok || !httpguts.ValidHeaderFieldName(name) || canonical == "Host" || len(literal) > 16384 {
				return nil, nil, ErrConfig
			}
			if _, duplicate := rawValues[canonical]; duplicate {
				return nil, nil, ErrConfig
			}
			expanded := substitute(literal, vars)
			if len(expanded) > 16384 || !httpguts.ValidHeaderFieldValue(expanded) {
				return nil, nil, ErrConfig
			}
			headers.Set(canonical, expanded)
			rawValues[canonical] = literal
		}
	}
	return headers, rawValues, nil
}

func requiredRequestHeaderNames(rule Map) ([]string, error) {
	raw, exists := rule["required_headers"]
	if !exists {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, ErrConfig
	}
	names := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		name, ok := raw.(string)
		if !ok || !httpguts.ValidHeaderFieldName(name) || http.CanonicalHeaderKey(name) == "Host" {
			return nil, ErrConfig
		}
		name = http.CanonicalHeaderKey(name)
		if seen[name] {
			return nil, ErrConfig
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}

func completeHeaderTemplate(literal string, vars map[string]string) bool {
	for _, placeholder := range placeholder.FindAllString(literal, -1) {
		value, exists := vars[placeholder[1:len(placeholder)-1]]
		if !exists || strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}
