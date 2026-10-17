package nativepermissions

import (
	"slices"
	"strings"
)

// RatingLevels is our conservative age-equivalent scale (age × 5, 0–100),
// not a claim that mainland China has an official film-rating system. Chinese
// labels and age labels are supported when supplied by the TMDB regional data.
// Unknown/NR/unrated remain unknown and are denied for a restricted account.
var ratingLevels = map[string]int{
	"G": 0, "TV-Y": 0, "TV-G": 0, "ALL": 0, "全年龄": 0, "普遍级": 0,
	"TV-Y7": 35, "TV-Y7-FV": 35, "PG": 40, "TV-PG": 40, "保护级": 30,
	"PG-13": 65, "TV-14": 70, "R": 85, "NC-17": 90, "TV-MA": 90,
	"6+": 30, "7+": 35, "12+": 60, "13+": 65, "15+": 75, "16+": 80, "18+": 90,
	"辅12级": 60, "輔12級": 60, "辅15级": 75, "輔15級": 75, "限制级": 90, "限制級": 90,
}

func RatingLevel(raw string) (int, bool) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	for _, prefix := range []string{"CN:", "US:"} {
		value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
	}
	level, ok := ratingLevels[value]
	return level, ok
}

// RatingCodes returns a copy for SQL pre-pagination filtering using exactly the
// same allowlist as ParentalAllowed; callers must not invent a second mapping.
func RatingCodes() []string {
	keys := make([]string, 0, len(ratingLevels))
	for k := range ratingLevels {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
