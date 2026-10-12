// Package releaseattrs provides shared, non-authoritative release title attributes.
package releaseattrs

import (
	"media-tool/internal/modules/releasename"
	"strings"
)

func Parse(title string) map[string]string {
	h := releasename.Parse(title + ".mkv")
	r := strings.ToLower(h.Resolution)
	if r == "4k" {
		r = "2160p"
	}
	if r == "8k" {
		r = "4320p"
	}
	c := strings.ToLower(h.Codec)
	switch c {
	case "h264", "h.264", "x264":
		c = "h264"
	case "h265", "h.265", "x265", "hevc":
		c = "h265"
	}
	src := strings.ToLower(h.Source)
	if strings.NewReplacer("-", "", ".", "", "_", "", " ", "").Replace(src) == "webdl" {
		src = "web-dl"
	}
	return map[string]string{"resolution": r, "source": src, "codec": c}
}

func Valid(key, value string) bool {
	if value == "" {
		return true
	}
	choices := map[string][]string{
		"resolution": {"480p", "576p", "720p", "1080i", "1080p", "1440p", "2160p", "4320p"},
		"source":     {"web-dl", "webrip", "bluray", "bdrip", "brrip", "remux", "hdtv", "dvdrip", "dvd", "hdrip", "cam", "hdcam"},
		"codec":      {"h264", "h265", "av1", "mpeg2"},
	}
	for _, choice := range choices[key] {
		if value == choice {
			return true
		}
	}
	return false
}
