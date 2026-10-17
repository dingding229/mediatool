package mediausers

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var managementToken = regexp.MustCompile(`(?i)[0-9a-f]{64}`)

// ManagementLabel is for display-only titles from client-controlled device
// headers or imported metadata. It never alters stored values or credentials.
// Management endpoints deliberately do NOT inherit the raw administrator log
// policy: a player must not smuggle a resolver/credential into a device name.
func ManagementLabel(value string) string {
	const hidden = "[名称已隐藏]"
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 256 || strings.ContainsFunc(value, unicode.IsControl) {
		return hidden
	}
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return hidden
	}
	lower := strings.ToLower(decoded)
	if strings.HasPrefix(lower, "/") || strings.HasPrefix(lower, "\\") || managementToken.MatchString(decoded) {
		return hidden
	}
	for _, marker := range []string{"://", ":\\", "/115/", "/root/", "/data/", "/mnt/", "strm302", "pickcode=", "pick_code=", "apikey=", "api_key=", "filekey=", "token=", ".115cdn.net/"} {
		if strings.Contains(lower, marker) {
			return hidden
		}
	}
	return value
}
