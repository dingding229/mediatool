package search

import (
	"encoding/json"
	"media-tool/internal/modules/metadata"
	"os"
	"strings"
	"testing"
)

func i29Media() metadata.Item {
	return metadata.Item{ID: 300146, MediaType: "series", Title: "嫁金钗", OriginalTitle: "嫁金钗", Year: "2026", IMDBID: "tt42576855", CandidateTitles: []string{"Jia Jin Chai", "Where the Mask Ends", "Transfer Gold Hairpin"}}
}
func TestI29RealReleaseShapes(t *testing.T) {
	raw, err := os.ReadFile("testdata/i29-release-samples.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct{ Site, Title, Subtitle string }
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		t.Run(r.Site+"/"+r.Title, func(t *testing.T) {
			// A broadcaster prefix is not a verified TMDB alias: don't strip arbitrary
			// words or let the Chinese subtitle override unrelated release identity.
			want := !strings.HasPrefix(r.Title, "CCTV-8 ")
			if got := MatchesMedia(Resource{Title: r.Title, Metadata: map[string]any{"subtitle": r.Subtitle}}, i29Media()); got != want {
				t.Fatal(got, want)
			}
		})
	}
}
func TestI29IdentityEvidence(t *testing.T) {
	for _, c := range []struct {
		name, title, kind string
		ids               map[string]any
		want              bool
	}{
		{"pinyin", "Jia.Jin.Chai.S01.2026.1080p", "series", nil, true},
		{"translation", "Where.the.Mask.Ends.S01E01.2026.2160p", "series", nil, true},
		{"Chinese", "嫁金钗.2026.S01.1080p", "series", nil, true},
		{"year conflict", "Where.the.Mask.Ends.S01.2019.1080p", "series", nil, false},
		{"type conflict", "Where.the.Mask.Ends.2026.1080p", "series", nil, false},
		{"movie subtitle only", "Unrelated.Movie.2026.1080p", "movie", nil, false},
		{"series subtitle only", "Unrelated.Show.S01.2026.1080p", "series", nil, false},
		{"tmdb conflict", "Where.the.Mask.Ends.S01.2026.1080p", "series", map[string]any{"tmdbId": 42}, false},
		{"tmdb strong", "Different.Title.S01.2026.1080p", "series", map[string]any{"tmdbId": "300146"}, true},
		{"imdb strong", "Different.Title.S01.2026.1080p", "series", map[string]any{"imdbId": "tt42576855"}, true},
		{"conflicting ids", "Where.the.Mask.Ends.S01.2026.1080p", "series", map[string]any{"tmdbId": 42, "imdbId": "tt42576855"}, false},
		{"imdb conflict", "Where.the.Mask.Ends.S01.2026.1080p", "series", map[string]any{"tmdbId": 300146, "imdbId": "tt11111111"}, false},
		{"strong type conflict", "Different.Title.2026.1080p", "series", map[string]any{"tmdbId": 300146}, false},
		{"strong year conflict", "Different.Title.S01.2019.1080p", "series", map[string]any{"tmdbId": 300146}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			i := i29Media()
			i.MediaType = c.kind
			m := map[string]any{"subtitle": "嫁金钗 2026 第1季"}
			for k, v := range c.ids {
				m[k] = v
			}
			if got := MatchesMedia(Resource{Title: c.title, Metadata: m}, i); got != c.want {
				t.Fatal(got, c.want)
			}
		})
	}
}
