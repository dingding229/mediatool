package library

import (
	"encoding/xml"
	"media-tool/internal/modules/metadata"
	"strings"
	"testing"
)

func TestEpisodeNFOUsesActualEpisodeIdentityAndDoesNotPolluteSharedShow(t *testing.T) {
	season := 1
	item := metadata.Item{ID: 99, MediaType: "series", Title: "Owned Show", Year: "2025", Overview: "show overview", EpisodeDetails: []metadata.Episode{{ID: 1001, SeriesID: 99, Season: 1, Number: 1, Title: "Actual Episode", Overview: "episode & plot", AirDate: "2026-01-01", Runtime: 30}}}
	files, err := nfoFilesCoverage("TV/Owned Show/Season 01/S01E01.strm", "owned", item, &season, []int{1})
	if err != nil || len(files) != 2 {
		t.Fatal(files, err)
	}
	var show, episode nfo
	if xml.Unmarshal(files[0].data, &show) != nil || xml.Unmarshal(files[1].data, &episode) != nil {
		t.Fatal("invalid XML")
	}
	if show.Title != "Owned Show" || show.Plot != "show overview" || show.Unique == nil || show.Unique.Value != "99" {
		t.Fatal("episode contaminated shared show")
	}
	if episode.Title != "Actual Episode" || !strings.Contains(episode.Plot, "episode & plot") || episode.Premiered != "2026-01-01" || episode.Aired != "2026-01-01" || episode.Year != "2026" || episode.Runtime == nil || *episode.Runtime != 30 || episode.Unique == nil || episode.Unique.Value != "1001" || episode.ShowTitle != "Owned Show" {
		t.Fatal("episode fields lost", episode)
	}
	item.EpisodeDetails[0].AirDate = ""
	item.EpisodeDetails[0].Runtime = 0
	files, err = nfoFilesCoverage("TV/Owned/Season 01/a.strm", "owned", item, &season, []int{1})
	if err != nil {
		t.Fatal(err)
	}
	xml.Unmarshal(files[1].data, &episode)
	if strings.Contains(string(files[1].data), "<year>") || strings.Contains(string(files[1].data), "<runtime>") {
		t.Fatal("unknown episode values invented")
	}
	if _, err = nfoFilesCoverage("TV/Owned/Season 01/a.strm", "owned", item, &season, []int{2}); err == nil {
		t.Fatal("wrong episode fallback silently accepted")
	}
}
