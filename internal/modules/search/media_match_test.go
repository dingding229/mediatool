package search

import (
	"media-tool/internal/modules/metadata"
	"testing"
)

func TestTMDBIdentityDoesNotTrustSubtitleAlias(t *testing.T) {
	i := metadata.Item{ID: 1381078, MediaType: "movie", Title: "一生所爱", OriginalTitle: "Love of Your Life", Year: "2026"}
	for _, c := range []struct {
		title string
		want  bool
	}{{"Love.of.Your.Life.2026.1080p.AMZN.WEB-DL.H264", true}, {"[一生所爱].Love.of.Your.Life.2026.1080p.mkv", true}, {"Bad Blood AKA Mauvais Sang 1986 2160p USA Blu-ray", false}, {"A.Chinese.Odyssey.Zhi.Zun.Bao.2022.2160p.WEB-DL", false}, {"Love.of.Your.Life.2018.1080p", false}, {"Love.of.Your.Life.S01E01.2026.1080p", false}} {
		if got := MatchesMedia(Resource{Title: c.title, Metadata: map[string]any{"subtitle": "一生所爱 / Love of Your Life (2026)"}}, i); got != c.want {
			t.Fatal(c.title, got)
		}
	}
}
