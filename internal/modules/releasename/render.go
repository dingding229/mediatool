package releasename

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

var variables = regexp.MustCompile(`\{([A-Za-z_]+)\}`)

// Render supports optional release details without inventing missing metadata.
func Render(template string, h Hint, original string) (string, error) {
	if len(template) == 0 || len(template) > 512 || h.Title == "" {
		return "", errors.New("template requires an identified title")
	}
	if strings.Contains(template, "{season}") && h.MediaType != "series" {
		return "", errors.New("series template requires season/episode")
	}
	if (strings.Contains(template, "{episode}") || strings.Contains(template, "{season_episode}")) && (h.Episode < 1 || h.MediaType != "series") {
		return "", errors.New("episode identity required")
	}
	vars := map[string]string{"title": h.Title, "year": h.Year, "season": fmt.Sprintf("%02d", h.Season), "episode": fmt.Sprintf("%02d", h.Episode), "resolution": h.Resolution, "videoFormat": h.Resolution, "season_episode": fmt.Sprintf("S%02dE%02d", h.Season, h.Episode), "source": h.Source, "codec": h.Codec, "group": h.Group, "ext": h.Extension, "original": strings.TrimSuffix(original, path.Ext(original))}
	invalid := false
	name := variables.ReplaceAllStringFunc(template, func(m string) string {
		v, ok := vars[m[1:len(m)-1]]
		if !ok {
			invalid = true
		}
		return v
	})
	name = strings.ReplaceAll(name, " ()", "")
	name = strings.ReplaceAll(name, " []", "")
	for strings.Contains(name, " -  - ") {
		name = strings.ReplaceAll(name, " -  - ", " - ")
	}
	name = strings.TrimSpace(name)
	if h.Extension != "" {
		stem := strings.TrimSuffix(name, h.Extension)
		stem = strings.TrimRight(stem, " -")
		name = stem + h.Extension
	}
	if invalid || strings.ContainsAny(name, "/\\\r\n\x00{}") {
		return "", errors.New("invalid template output")
	}
	return name, nil
}
