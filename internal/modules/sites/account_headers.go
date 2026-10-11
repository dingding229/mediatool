package sites

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"golang.org/x/net/http/httpguts"
)

var ErrAccountHeaders = errors.New("invalid PT request-header credentials")

// ParseAccountHeaders accepts only an explicit JSON object of literal secrets.
// Token iteration rejects duplicate JSON names, including case aliases; ordinary
// map unmarshalling would silently retain only the final credential value.
func ParseAccountHeaders(raw string) (map[string]string, error) {
	out := map[string]string{}
	if raw == "" {
		return out, nil
	}
	if len(raw) > 16384 {
		return nil, ErrAccountHeaders
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, ErrAccountHeaders
	}
	for decoder.More() {
		name, err := decoder.Token()
		key, ok := name.(string)
		if err != nil || !ok {
			return nil, ErrAccountHeaders
		}
		var value any
		if decoder.Decode(&value) != nil {
			return nil, ErrAccountHeaders
		}
		literal, ok := value.(string)
		canonical := http.CanonicalHeaderKey(key)
		if !ok || !validAccountHeader(key, literal) {
			return nil, ErrAccountHeaders
		}
		if _, duplicate := out[canonical]; duplicate {
			return nil, ErrAccountHeaders
		}
		out[canonical] = literal
		if len(out) > 16 {
			return nil, ErrAccountHeaders
		}
	}
	last, err := decoder.Token()
	if err != nil || last != json.Delim('}') {
		return nil, ErrAccountHeaders
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, ErrAccountHeaders
	}
	return out, nil
}

func validAccountHeader(name, value string) bool {
	if !httpguts.ValidHeaderFieldName(name) || len(name) > 128 || len(value) > 8192 || strings.TrimSpace(value) == "" || !httpguts.ValidHeaderFieldValue(value) {
		return false
	}
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "proxy-") || strings.HasPrefix(lower, "x-forwarded-") || strings.HasPrefix(lower, "sec-") {
		return false
	}
	switch lower {
	case "host", "cookie", "set-cookie", "content-length", "content-type", "connection", "keep-alive", "te", "trailer", "transfer-encoding", "upgrade", "forwarded", "expect", "range", "accept-encoding", "origin", "referer", "user-agent":
		return false
	}
	return true
}

func normalizedAccountHeaders(values map[string]string) (map[string]string, error) {
	if len(values) > 16 {
		return nil, ErrAccountHeaders
	}
	out := make(map[string]string, len(values))
	for name, value := range values {
		canonical := http.CanonicalHeaderKey(name)
		if !validAccountHeader(name, value) {
			return nil, ErrAccountHeaders
		}
		if _, duplicate := out[canonical]; duplicate {
			return nil, ErrAccountHeaders
		}
		out[canonical] = value
	}
	return out, nil
}
