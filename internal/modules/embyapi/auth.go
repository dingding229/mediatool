package embyapi

import (
	"net/http"
	"net/url"
	"strings"
)

// Auth is unverified client input. Tokens never enter JSON. Conflicting tokens
// from any source are rejected; metadata keeps X-Emby-Authorization precedence.
type Auth struct {
	Client, Device, DeviceID, Version, UserID string
	Token                                     string `json:"-"`
}

func ParseAuth(r *http.Request, values Values) (Auth, error) {
	fields := map[string]string{}
	var tokens []string
	for _, name := range []string{"X-Emby-Authorization", "Authorization"} {
		raw, err := singleHeader(r, name)
		if err != nil {
			return Auth{}, err
		}
		if raw == "" {
			continue
		}
		word, rest, found := strings.Cut(strings.TrimSpace(raw), " ")
		if strings.EqualFold(word, "MediaBrowser") || strings.EqualFold(word, "Emby") {
			raw = strings.TrimSpace(rest)
		} else if name == "Authorization" || (found && !strings.Contains(word, "=")) {
			return Auth{}, ErrInvalid
		}
		parsed, err := authFields(raw)
		if err != nil {
			return Auth{}, err
		}
		tokens = append(tokens, parsed["token"])
		for key, value := range parsed {
			if fields[key] == "" {
				fields[key] = value
			}
		}
	}
	for _, name := range []string{"X-Emby-Token", "X-MediaBrowser-Token"} {
		value, err := singleHeader(r, name)
		if err != nil {
			return Auth{}, ErrInvalid
		}
		value, err = quotedValue(value)
		if err != nil || !validText(value, 512) {
			return Auth{}, ErrInvalid
		}
		tokens = append(tokens, value)
	}
	for _, key := range []string{"api_key", "X-Emby-Token"} {
		value, err := values.Scalar(key)
		if err != nil || !validText(value, 512) {
			return Auth{}, ErrInvalid
		}
		tokens = append(tokens, value)
	}
	queryUserID, err := values.Scalar("UserId")
	if err != nil || !validText(queryUserID, 64) {
		return Auth{}, ErrInvalid
	}
	if fields["userid"] == "" {
		fields["userid"] = queryUserID
	}
	var token string
	for _, value := range tokens {
		if value == "" {
			continue
		}
		if token != "" && token != value {
			return Auth{}, ErrInvalid
		}
		token = value
	}
	return Auth{fields["client"], fields["device"], fields["deviceid"], fields["version"], fields["userid"], token}, nil
}
func quotedValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return value, nil
	}
	if strings.HasPrefix(value, `"`) != strings.HasSuffix(value, `"`) {
		return "", ErrInvalid
	}
	if strings.HasPrefix(value, `"`) {
		if len(value) < 2 {
			return "", ErrInvalid
		}
		value = value[1 : len(value)-1]
	}
	return value, nil
}
func singleHeader(r *http.Request, name string) (string, error) {
	var result string
	count := 0
	for key, values := range r.Header {
		if !strings.EqualFold(key, name) {
			continue
		}
		for _, value := range values {
			count++
			if count > 8 || len(value) > 8192 || !validText(value, 8192) || (count > 1 && result != value) {
				return "", ErrInvalid
			}
			result = value
		}
	}
	return result, nil
}
func authFields(raw string) (map[string]string, error) {
	out := map[string]string{}
	for count := 0; strings.TrimSpace(raw) != ""; count++ {
		if count >= 32 {
			return nil, ErrInvalid
		}
		key, rest, ok := strings.Cut(strings.TrimSpace(raw), "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if !ok || key == "" || strings.ContainsAny(key, " ,\t\"\\") {
			return nil, ErrInvalid
		}
		rest = strings.TrimSpace(rest)
		var value string
		if strings.HasPrefix(rest, `"`) {
			var text []byte
			closed := false
			for i := 1; i < len(rest); i++ {
				if rest[i] == '"' {
					value = string(text)
					rest = rest[i+1:]
					closed = true
					break
				}
				if rest[i] == '\\' {
					i++
					if i == len(rest) {
						return nil, ErrInvalid
					}
				}
				text = append(text, rest[i])
			}
			if !closed {
				return nil, ErrInvalid
			}
			rest = strings.TrimSpace(rest)
			if rest != "" && rest[0] != ',' {
				return nil, ErrInvalid
			}
			raw = strings.TrimPrefix(rest, ",")
		} else {
			value, raw, _ = strings.Cut(rest, ",")
			value = strings.TrimSpace(value)
			if strings.ContainsAny(value, "\"\\") {
				return nil, ErrInvalid
			}
		}
		value, err := url.PathUnescape(value)
		maximum := 1024
		switch key {
		case "client":
			maximum = 128
		case "device", "deviceid":
			maximum = 256
		case "version", "userid":
			maximum = 64
		case "token":
			maximum = 512
		}
		if err != nil || !validText(value, maximum) {
			return nil, ErrInvalid
		}
		if prior, exists := out[key]; exists && prior != value {
			return nil, ErrInvalid
		}
		out[key] = value
	}
	return out, nil
}
