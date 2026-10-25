package classification

import (
	"errors"
	"regexp"
	"strings"

	"media-tool/internal/modules/metadata"
)

// Facts is a provider-only classification snapshot, independent of display
// metadata and physical output assignment. Version marks a completed fetch even
// when the provider genuinely has no country/language, preventing refresh loops.
type Facts struct {
	Version          int      `json:"version"`
	GenreIDs         []int    `json:"genreIds"`
	Countries        []string `json:"countries"`
	OriginalLanguage string   `json:"originalLanguage"`
}

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)
var languageCode = regexp.MustCompile(`^([a-z]{2,3})?$`)

func (f Facts) Valid() bool {
	if f.Version != 1 || f.GenreIDs == nil || len(f.GenreIDs) > 200 || f.Countries == nil || len(f.Countries) > 128 || !languageCode.MatchString(f.OriginalLanguage) {
		return false
	}
	for _, id := range f.GenreIDs {
		if id < 1 || id > 100000 {
			return false
		}
	}
	for _, code := range f.Countries {
		if !countryCode.MatchString(code) {
			return false
		}
	}
	return true
}
func FactsFromItem(i metadata.Item) (Facts, error) {
	out := Facts{Version: 1, GenreIDs: []int{}, Countries: []string{}, OriginalLanguage: strings.ToLower(strings.TrimSpace(i.OriginalLanguage))}
	genres, countries := map[int]bool{}, map[string]bool{}
	for _, id := range i.GenreIDs {
		if !genres[id] {
			out.GenreIDs = append(out.GenreIDs, id)
			genres[id] = true
		}
	}
	for _, code := range i.Countries {
		code = strings.ToUpper(strings.TrimSpace(code))
		if !countries[code] {
			out.Countries = append(out.Countries, code)
			countries[code] = true
		}
	}
	if !out.Valid() {
		return Facts{}, errors.New("分类事实格式无效")
	}
	return out, nil
}

// Apply changes ONLY the matching inputs. Already known ingest facts retain priority; missing inputs
// are filled from the provider cache. title/rating/artwork and saved paths are never edited.
func (f Facts) Apply(i metadata.Item) metadata.Item {
	if !f.Valid() {
		return i
	}
	if len(i.GenreIDs) == 0 && len(f.GenreIDs) > 0 {
		i.GenreIDs = append([]int(nil), f.GenreIDs...)
	}
	if len(i.Countries) == 0 && len(f.Countries) > 0 {
		i.Countries = append([]string(nil), f.Countries...)
	}
	if i.OriginalLanguage == "" && f.OriginalLanguage != "" {
		i.OriginalLanguage = f.OriginalLanguage
	}
	return i
}
