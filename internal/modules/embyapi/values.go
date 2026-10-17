package embyapi

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid Emby request")

// Values preserves wire order even when repeated keys use different casing.
// Scalars reject contradictory duplicates rather than choosing a random value.
type Values struct{ entries map[string][]string }

func ParseValues(raw string) (Values, error) {
	v := Values{entries: make(map[string][]string)}
	if len(raw) > 32768 || strings.Count(raw, "&") >= 512 {
		return v, ErrInvalid
	}
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		if strings.Contains(pair, ";") {
			return Values{}, ErrInvalid
		}
		key, value, _ := strings.Cut(pair, "=")
		key, err := url.QueryUnescape(key)
		if err != nil || !validText(key, 128) || key == "" {
			return Values{}, ErrInvalid
		}
		value, err = url.QueryUnescape(value)
		if err != nil || !validText(value, 32768) {
			return Values{}, ErrInvalid
		}
		key = foldKey(key)
		v.entries[key] = append(v.entries[key], value)
	}
	return v, nil
}
func (v Values) Scalar(key string) (string, error) {
	values := v.entries[foldKey(key)]
	for _, value := range values {
		if value != values[0] {
			return "", ErrInvalid
		}
	}
	if len(values) == 0 {
		return "", nil
	}
	return values[0], nil
}
func (v Values) Boolean(key string) (*bool, error) {
	raw, err := v.Scalar(key)
	if err != nil {
		return nil, err
	}
	if len(v.entries[foldKey(key)]) == 0 {
		return nil, nil
	}
	var value bool
	switch strings.ToLower(raw) {
	case "true", "1":
		value = true
	case "false", "0":
		value = false
	default:
		return nil, ErrInvalid
	}
	return &value, nil
}
func (v Values) list(key string, max int) ([]string, error) {
	var out []string
	count := 0
	for _, raw := range v.entries[foldKey(key)] {
		for _, part := range strings.Split(raw, ",") {
			count++
			if count > max {
				return nil, ErrInvalid
			}
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out, nil
}
func validText(value string, max int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max && !strings.ContainsFunc(value, unicode.IsControl)
}

// ApiKey is a widespread spelling without an underscore, not just case folding.
func foldKey(key string) string {
	key = strings.ToLower(key)
	if key == "apikey" {
		return "api_key"
	}
	return key
}
