package probeinfo

import (
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
	"strconv"
	"strings"
)

// DisplayTitle is the sole title policy: resolution/codec/range for video;
// English language name/codec/layout/default/forced/accessibility otherwise.
// Untrusted track titles are stored separately, never interpolated here.
func DisplayTitle(s Stream) string {
	var parts []string
	if s.Type == Video {
		if s.Height != nil {
			parts = append(parts, strconv.Itoa(*s.Height)+"p")
		}
		label := s.VideoRangeType
		if strings.HasPrefix(label, DOVI) {
			label = "Dolby Vision"
		}
		if label == "" {
			label = s.VideoRange
		}
		parts = append(parts, strings.ToUpper(s.Codec), label)
	} else {
		if s.Language != "" {
			parts = append(parts, display.English.Languages().Name(language.Make(s.Language)))
		}
		parts = append(parts, strings.ToUpper(s.Codec))
		if s.Type == Audio && s.Channels != nil {
			layout := map[int]string{1: "Mono", 2: "Stereo", 6: "5.1", 8: "7.1"}[*s.Channels]
			if layout == "" {
				layout = strconv.Itoa(*s.Channels) + "ch"
			}
			parts = append(parts, layout)
		}
		if s.IsDefault {
			parts = append(parts, "Default")
		}
		if s.IsForced {
			parts = append(parts, "Forced")
		}
		if s.IsHearingImpaired {
			parts = append(parts, "Hearing Impaired")
		}
	}
	var nonempty []string
	for _, p := range parts {
		if p != "" {
			nonempty = append(nonempty, p)
		}
	}
	return strings.Join(nonempty, " ")
}
