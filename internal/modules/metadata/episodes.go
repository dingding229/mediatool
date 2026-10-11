package metadata

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var episodePattern = regexp.MustCompile(`(?i)S(\d{1,3})(E\d{1,4}(?:(?:-E?\d{1,4})|(?:E\d{1,4}))*)`)
var episodeChain = regexp.MustCompile(`(?i)E?(\d{1,4})(?:-E?(\d{1,4}))?`)

func Episodes(filename string) (int, []int, bool) {
	matches := episodePattern.FindAllStringSubmatch(filename, -1)
	if len(matches) == 0 {
		return 0, nil, false
	}
	season := -1
	set := map[int]bool{}
	for _, m := range matches {
		s, _ := strconv.Atoi(m[1])
		if season != -1 && season != s {
			return 0, nil, false
		}
		season = s
		for _, part := range episodeChain.FindAllStringSubmatch(m[2], -1) {
			first, _ := strconv.Atoi(part[1])
			last := first
			if part[2] != "" {
				last, _ = strconv.Atoi(part[2])
			}
			if first < 1 || last < first || last-first > 100 {
				return 0, nil, false
			}
			for n := first; n <= last; n++ {
				set[n] = true
			}
			if len(set) > 1000 {
				return 0, nil, false
			}
		}
	}
	out := []int{}
	for n := range set {
		out = append(out, n)
	}
	sort.Ints(out)
	return season, out, len(out) > 0
}
func IsVideo(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".mkv", ".mp4", ".m4v", ".mov", ".avi", ".ts", ".m2ts", ".webm":
		return true
	}
	return false
}
func IsSample(name string) bool {
	return strings.EqualFold(strings.TrimSuffix(path.Base(name), path.Ext(name)), "sample") || strings.Contains(strings.ToLower(name), "/sample/")
}
