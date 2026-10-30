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

func TestSeasonPacksAndChineseTitles(t *testing.T) {
	for _, name := range []string{"Where the Mask Ends S01 2026 2160p YK WEB-DL H.265-HHWEB", "Jia.Jin.Chai.S01.2026.1080p", "嫁金钗.2026.S01", "嫁金钗.S01.2026.WEB-DL"} {
		h := Parse(name)
		if h.MediaType != "series" || h.Season != 1 || h.Episode != 0 || len(h.Episodes) != 0 || h.Year != "2026" {
			t.Fatal(name, h)
		}
		if h.Title != "Where the Mask Ends" && h.Title != "Jia Jin Chai" && h.Title != "嫁金钗" {
			t.Fatal(h)
		}
	}
	for _, name := range []string{"Movie.S01omething.2026.1080p", "Movie.AVS01.2026.1080p"} {
		if Parse(name).MediaType == "series" {
			t.Fatal("partial season token", name)
		}
	}
}
