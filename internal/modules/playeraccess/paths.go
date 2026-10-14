package playeraccess

import (
	"media-tool/internal/modules/strm"
	"path"
	"regexp"
	"strings"
)

var embyPlayback = regexp.MustCompile(`(?i)^/(?:Videos|Audio)/[^/]+(?:/|$)|^/Items/[^/]+/(?:PlaybackInfo|Download|File)(?:/|$)|^/LiveTv/(?:LiveStream|Recording)(?:/|$)|^/Sync/[^/]+/File(?:/|$)`)
var embyMetadata = regexp.MustCompile(`(?i)^/(?:Videos|Audio)/[^/]+/(?:Subtitles|Attachments|AdditionalParts)(?:/|$)`)
var memberPlayback = regexp.MustCompile(`^/api/v1/member/libraries/[^/]+/items/[^/]+/playback$`)

// Entry uses only the decoded routing path, never client-supplied forwarding
// headers. Case/nested Emby mounts follow the existing gateway normalization.
func Entry(raw string) string {
	normalized := path.Clean(raw)
	if normalized == strm.Endpoint {
		return "strm"
	}
	if memberPlayback.MatchString(normalized) {
		return "member"
	}
	isEmby := false
	for len(normalized) >= 5 && strings.EqualFold(normalized[:5], "/emby") && (len(normalized) == 5 || normalized[5] == '/') {
		isEmby = true
		normalized = path.Clean(normalized[5:])
		if normalized == "." {
			normalized = "/"
		}
	}
	if isEmby && embyPlayback.MatchString(normalized) && !embyMetadata.MatchString(normalized) {
		return "emby"
	}
	return ""
}
