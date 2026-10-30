// Package releasename recognizes Scene/0day release names as metadata hints,
// never an authoritative TMDB identity or permission to modify files.
package releasename

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

type Hint struct {
	Title      string   `json:"title"`
	Year       string   `json:"year"`
	MediaType  string   `json:"mediaType"`
	Season     int      `json:"season"`
	Episode    int      `json:"episode"`
	Episodes   []int    `json:"episodes"`
	Resolution string   `json:"resolution"`
	Source     string   `json:"source"`
	Codec      string   `json:"codec"`
	Group      string   `json:"group"`
	Extension  string   `json:"extension"`
	Confidence string   `json:"confidence"`
	Warnings   []string `json:"warnings"`
}

var seasonEpisode = regexp.MustCompile(`(?i)(?:^|[. _\-\[(])S(\d{1,3})[. _-]*E(\d{1,4})(?:(?:[. _-]*E)(\d{1,4}))?`)
var seasonOnly = regexp.MustCompile(`(?i)(?:^|[. _\-\[(])S(\d{1,3})(?:$|[. _\-\])])`)
var xEpisode = regexp.MustCompile(`(?i)(?:^|[. _\-\[(])(\d{1,2})x(\d{1,4})(?:\D|$)`)
var year = regexp.MustCompile(`(?:19|20)\d{2}`)
var resolution = regexp.MustCompile(`(?i)(?:^|[. _\-])((?:480|576|720|1080|1440|2160|4320)[pi]|4k|8k)(?:[. _\-]|$)`)
var source = regexp.MustCompile(`(?i)(?:^|[. _\-])(WEB[. _-]?DL|WEBRip|BluRay|BDRip|BRRip|Remux|HDTV|DVDRip|DVD|HDRip|CAM|HDCAM)(?:[. _\-]|$)`)
var codec = regexp.MustCompile(`(?i)(?:^|[. _\-])(x26[45]|h[. _]?26[45]|HEVC|AV1|MPEG2)(?:[. _\-]|$)`)
var chinese = regexp.MustCompile(`第(\d{1,3})季[. _-]*第?(\d{1,4})集`)
var group = regexp.MustCompile(`-([A-Za-z0-9][A-Za-z0-9_-]{1,32})$`)
var separators = regexp.MustCompile(`[._]+`)

func Parse(filename string) Hint {
	n := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	ext := path.Ext(n)
	// Directory/release titles may end in .2026, .S03E07 or .1080p.
	// Those are recognition evidence, not a filename extension.
	if len(ext) > 8 || year.FindString(ext) == strings.TrimPrefix(ext, ".") || seasonEpisode.MatchString(ext) || seasonOnly.MatchString(ext) || xEpisode.MatchString(ext) || resolution.MatchString(ext) {
		ext = ""
	}
	stem := strings.TrimSuffix(n, ext)
	h := Hint{MediaType: "unknown", Extension: ext, Confidence: "low", Episodes: []int{}, Warnings: []string{}}
	cut := len(stem)
	for _, rx := range []*regexp.Regexp{seasonEpisode, xEpisode, chinese} {
		if m := rx.FindStringSubmatchIndex(stem); m != nil {
			h.MediaType = "series"
			h.Season, _ = strconv.Atoi(stem[m[2]:m[3]])
			h.Episode, _ = strconv.Atoi(stem[m[4]:m[5]])
			h.Episodes = []int{h.Episode}
			cut = min(cut, m[0])
			if len(m) > 6 && m[6] >= 0 {
				end, _ := strconv.Atoi(stem[m[6]:m[7]])
				if end != h.Episode {
					h.Episodes = append(h.Episodes, end)
					h.Warnings = append(h.Warnings, "多集文件：只保留明确集号，不猜测中间覆盖")
				}
			}
			break
		}
	}
	// A season pack is series evidence even without an episode number.
	if h.MediaType == "unknown" {
		if m := seasonOnly.FindStringSubmatchIndex(stem); m != nil {
			h.MediaType = "series"
			h.Season, _ = strconv.Atoi(stem[m[2]:m[3]])
			cut = min(cut, m[0])
		}
	}
	for _, m := range year.FindAllStringIndex(stem, -1) {
		beforeOK := m[0] == 0 || strings.ContainsAny(stem[m[0]-1:m[0]], ". _-[(")
		afterOK := m[1] == len(stem) || strings.ContainsAny(stem[m[1]:m[1]+1], ". _-])")
		prefix := strings.Trim(stem[:m[0]], " ._-[(")
		if beforeOK && afterOK && prefix != "" {
			h.Year = stem[m[0]:m[1]]
			cut = min(cut, m[0])
			if h.MediaType == "unknown" {
				h.MediaType = "movie"
			}
			break
		}
	}

	for _, pair := range []struct {
		rx     *regexp.Regexp
		target *string
	}{{resolution, &h.Resolution}, {source, &h.Source}, {codec, &h.Codec}} {
		if m := pair.rx.FindStringSubmatchIndex(stem); m != nil {
			*pair.target = stem[m[2]:m[3]]
			cut = min(cut, m[0])
		}
	}
	if m := group.FindStringSubmatchIndex(stem); m != nil && m[0] >= cut {
		h.Group = stem[m[2]:m[3]]
	}
	h.Title = strings.Trim(separators.ReplaceAllString(stem[:cut], " "), " -[]()")
	h.Title = strings.Join(strings.Fields(h.Title), " ")
	if h.Resolution != "" {
		h.Resolution = strings.ToLower(h.Resolution)
		if h.Resolution == "4k" {
			h.Resolution = "2160p"
		}
		if h.Resolution == "8k" {
			h.Resolution = "4320p"
		}
	}
	if h.Title != "" && h.MediaType != "unknown" {
		h.Confidence = "high"
	}
	if h.Year == "" {
		h.Warnings = append(h.Warnings, "未识别明确年份；不会从标题数字猜造年份")
	}
	if h.MediaType == "unknown" {
		h.Warnings = append(h.Warnings, "无年份或季集标记，类型需人工确认")
	}
	if h.Title == "" {
		h.Warnings = append(h.Warnings, "无法识别标题")
	}
	return h
}
