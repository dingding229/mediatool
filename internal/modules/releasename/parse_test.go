package releasename

import (
	"strings"
	"testing"
)

func TestSceneHints(t *testing.T) {
	for _, tc := range []struct {
		name, title, kind, year, res string
		season, episode              int
	}{{"Princess.Mononoke.1997.1080p.BluRay.x265-GROUP.mkv", "Princess Mononoke", "movie", "1997", "1080p", 0, 0}, {"Shogun.2024.S01E04.2160p.WEB-DL.H265-TEAM.mkv", "Shogun", "series", "2024", "2160p", 1, 4}, {"Show.Name.2x03.720p.HDTV.x264-ABC.mkv", "Show Name", "series", "", "720p", 2, 3}, {"Dark.S00E01.WEBRip.AV1-GRP.mkv", "Dark", "series", "", "", 0, 1}, {"1917.2019.1080p.BluRay.x265-GRP.mkv", "1917", "movie", "2019", "1080p", 0, 0}, {"无间道.2002.1080p.BluRay.x264-Team.mkv", "无间道", "movie", "2002", "1080p", 0, 0}, {"Unknown.Title.mkv", "Unknown Title", "unknown", "", "", 0, 0}} {
		h := Parse(tc.name)
		if h.Title != tc.title || h.MediaType != tc.kind || h.Year != tc.year || h.Resolution != tc.res || h.Season != tc.season || h.Episode != tc.episode {
			t.Errorf("%s => %+v", tc.name, h)
		}
	}
}
func TestTemplateOptionalFieldsAndTraversal(t *testing.T) {
	h := Parse("Dark.S01E03.1080p.WEB-DL.x265-GRP.mkv")
	name, err := Render("{title} - S{season}E{episode} - {resolution}{ext}", h, "file.mkv")
	if err != nil || name != "Dark - S01E03 - 1080p.mkv" {
		t.Fatal(name, err)
	}
	h = Parse("Movie.2024.mkv")
	name, err = Render("{title} ({year}) - {resolution}{ext}", h, "file.mkv")
	if err != nil || name != "Movie (2024).mkv" {
		t.Fatal(name, err)
	}
	if _, err = Render("../../{title}", h, "file.mkv"); err == nil {
		t.Fatal("traversal allowed")
	}
	h.Title = "../unsafe"
	if _, err = Render("{title}{ext}", h, "file.mkv"); err == nil {
		t.Fatal("unsafe hint allowed")
	}
	h = Parse("Release.S01E01E03.mkv")
	if len(h.Episodes) != 2 || h.Episodes[1] != 3 || strings.Contains(h.Title, "S01") {
		t.Fatal(h)
	}
}

func TestDirectoryReleaseTokensAreNotRemovedAsExtensions(t *testing.T) {
	for _, name := range []string{"Owned.Film.2026", "Owned.Show.2026.S03E07", "Owned.Film.2026.1080p", "剧集.2026.S00E02"} {
		got, want := Parse(name), Parse(name+".mkv")
		if got.Title != want.Title || got.Year != want.Year || got.MediaType != want.MediaType || got.Season != want.Season || got.Episode != want.Episode || got.Resolution != want.Resolution || got.Extension != "" {
			t.Fatalf("directory=%+v filename=%+v", got, want)
		}
	}
}
