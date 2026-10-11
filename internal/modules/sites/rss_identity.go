package sites

import (
	"net/url"
	"regexp"
	"strings"
)

// These routes are deliberately finite. UNIT3D's rss.php declares the singular
// /torrent/download/{id}.{rsskey}; its normal torrent routes use /torrents/.
// Unknown paths must not be guessed by taking the first digits of a URL/key.
var feedRoutes = []struct {
	pattern  *regexp.Regexp
	download bool
}{
	{regexp.MustCompile(`^/torrent/download/([0-9]{1,30})\.[A-Za-z0-9_-]{1,256}$`), true},
	{regexp.MustCompile(`^/torrents/download/([0-9]{1,30})(?:\.[A-Za-z0-9_-]{1,256})?$`), true},
	{regexp.MustCompile(`^/torrents/([0-9]{1,30})$`), false},
	{regexp.MustCompile(`^/detail/([0-9]{1,30})$`), false},
}
var feedIDPattern = regexp.MustCompile(`^[0-9]{1,30}$`)

// feedLinkIdentity returns a torrent identity, not a feed GUID. URL and query
// credentials never become identities or baseline digests.
func feedLinkIdentity(u *url.URL) (string, bool, error) {
	if u == nil {
		return "", false, nil
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", false, ErrFeed
	}
	id := ""
	merge := func(candidate string) error {
		if !feedIDPattern.MatchString(candidate) || (id != "" && id != candidate) {
			return ErrFeed
		}
		id = candidate
		return nil
	}
	for _, key := range []string{"id", "tid", "torrentId"} {
		if values, exists := q[key]; exists {
			if len(values) != 1 || merge(values[0]) != nil {
				return "", false, ErrFeed
			}
		}
	}
	download := false
	for _, route := range feedRoutes {
		if match := route.pattern.FindStringSubmatch(u.Path); len(match) > 0 {
			if merge(match[1]) != nil {
				return "", false, ErrFeed
			}
			download = route.download
			break
		}
	}
	if id != "" {
		switch strings.ToLower(u.Path) {
		case "/download.php", "/download", "/dl.php":
			download = true
		}
	}
	return id, download, nil
}

// Only known public detail paths may enter public resource metadata. Download
// paths (notably UNIT3D's rsskey suffix) stay exclusively in encrypted hints.
func feedDetailsPath(path string) bool {
	if strings.EqualFold(path, "/details.php") {
		return true
	}
	for _, route := range feedRoutes {
		if !route.download && route.pattern.MatchString(path) {
			return true
		}
	}
	return false
}
