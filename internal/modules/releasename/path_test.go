package releasename

import "testing"

func TestUserFolderTemplatesAliasesAndBoundaries(t *testing.T) {
	movie := Hint{Title: "幽灵公主", Year: "1997", MediaType: "movie", Resolution: "1080p", Extension: ".mkv"}
	got, err := RenderPath("{title} ({year})/{title} ({year}) - {videoFormat}", movie, "Movie.mkv")
	if err != nil || got != "幽灵公主 (1997)/幽灵公主 (1997) - 1080p.mkv" {
		t.Fatal(got, err)
	}
	series := Hint{Title: "自有剧集", Year: "2026", MediaType: "series", Season: 0, Episode: 4, Extension: ".mkv"}
	got, err = RenderPath("{title} ({year})/Season {season}/{title} - {season_episode} - 第{episode}集", series, "Show.mkv")
	if err != nil || got != "自有剧集 (2026)/Season 00/自有剧集 - S00E04 - 第04集.mkv" {
		t.Fatal(got, err)
	}
	for _, template := range []string{"/{title}", "../{title}", "{title}/../{title}", "{title}//{title}", "C:/{title}", "{title}/{unknown}", "{title}/."} {
		if _, err = RenderPath(template, movie, "Movie.mkv"); err == nil {
			t.Fatal("unsafe template accepted", template)
		}
	}
	movie.Title = "../escaped"
	if _, err = RenderPath("{title}/{title}", movie, "Movie.mkv"); err == nil {
		t.Fatal("title injected path")
	}
}
