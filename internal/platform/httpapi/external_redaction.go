package httpapi

import (
	"net/url"
	"regexp"
	"strings"
)

var externalCredentialPattern = regexp.MustCompile(`\b(?:eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+|mt_api_[a-f0-9]{64})\b`)

func externalLogText(v string) string {
	return externalCredentialPattern.ReplaceAllString(v, "[redacted]")
}
func externalLogURL(v string) string {
	u, e := url.Parse(v)
	if e != nil {
		return externalLogText(v)
	}
	if clean := externalLogText(u.Path); clean != u.Path {
		u.Path = clean
		u.RawPath = ""
	}
	q := u.Query()
	changed := false
	for k := range q {
		switch strings.ToLower(k) {
		case "token", "access_token", "api_key", "apikey", "authorization":
			q.Set(k, "[redacted]")
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return externalLogText(u.String())
}
