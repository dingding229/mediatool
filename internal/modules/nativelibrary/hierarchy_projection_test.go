package nativelibrary

import (
	"encoding/json"
	"media-tool/internal/modules/embyapi/dto"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func hierarchyFixtures(t *testing.T) map[string]dto.BaseItemDto {
	t.Helper()
	stamp := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	rating := float32(8.7)
	seriesDetails := &TMDBDetails{StoredMovieMetadata: StoredMovieMetadata{OriginalTitle: "Synthetic Series Original", SortName: "Synthetic Series", Overview: "series overview", Genres: []string{"Drama"}, PosterPath: "/series.jpg", BackdropPath: "/backdrop.jpg", PremiereDate: "2025-01-02", CommunityRating: &rating, OfficialRating: "TV-14", ImdbID: "tt123456"}, Status: "Ended", EndDate: "2026-03-01", People: []TMDBPerson{{ID: 1, Name: "Lead", Type: "Actor", ImagePath: "/lead.jpg"}}}
	series, err := SeriesDTO(SeriesProjection{ID: "100201", ServerID: strings.Repeat("b", 32), ParentID: "100001", Name: "Synthetic Series", Year: "2025", TMDBID: 90201, Details: seriesDetails, SeasonCount: 2, EpisodeCount: 3, CreatedAt: stamp})
	if err != nil {
		t.Fatal(err)
	}
	seasonDetails := &TMDBDetails{StoredMovieMetadata: StoredMovieMetadata{PosterPath: "/special.jpg"}, Name: "特别篇", Episodes: []TMDBEpisode{{Number: 1, RuntimeMinutes: 24}}}
	season, err := SeasonDTO(SeasonProjection{ID: "100202", SeriesID: "100201", ParentID: "100201", SeriesName: "Synthetic Series", Name: "特别篇", Year: "2025", TMDBID: 90201, Number: 0, Details: seasonDetails, SeriesDetails: seriesDetails, EpisodeCount: 1, CreatedAt: stamp})
	if err != nil {
		t.Fatal(err)
	}
	runtime := int64(24 * 60 * 10000000)
	episode, err := EpisodeDTO(EpisodeProjection{ID: "100203", SeriesID: "100201", SeasonID: "100202", ParentID: "100202", SeriesName: "Synthetic Series", SeasonName: "特别篇", Name: "Synthetic Episode", Overview: "episode overview", Year: "2025", TMDBID: 90201, Number: 1, NumberEnd: 3, Season: 0, AirDate: "2025-02-03", RuntimeTicks: &runtime, StillPath: "/still.jpg", SeriesDetails: seriesDetails, Sources: []MovieSource{{ID: "asset-1", Container: "mkv", RunTimeTicks: &runtime}, {ID: "asset-2", Container: "mp4"}}, CreatedAt: stamp})
	if err != nil {
		t.Fatal(err)
	}
	local, err := EpisodeDTO(EpisodeProjection{ID: "100204", SeriesID: "100201", SeasonID: "100202", ParentID: "100202", SeriesName: "Synthetic Series", SeasonName: "特别篇", Name: "Synthetic Local Frame", TMDBID: 90201, Number: 2, Season: 0, LocalStillTag: StillImageTag(strings.Repeat("c", 64), strings.Repeat("d", 64)), FallbackPoster: "/special.jpg", SeriesDetails: seriesDetails, CreatedAt: stamp})
	if err != nil {
		t.Fatal(err)
	}
	poster, err := EpisodeDTO(EpisodeProjection{ID: "100205", SeriesID: "100201", SeasonID: "100202", ParentID: "100202", SeriesName: "Synthetic Series", SeasonName: "特别篇", Name: "Synthetic Poster Fallback", TMDBID: 90201, Number: 3, Season: 0, FallbackPoster: "/special.jpg", SeriesDetails: seriesDetails, CreatedAt: stamp})
	if err != nil {
		t.Fatal(err)
	}
	return map[string]dto.BaseItemDto{"series": series, "season": season, "episode": episode, "episode-local": local, "episode-poster": poster}
}
func TestHierarchyProjectionGoldens(t *testing.T) {
	for name, item := range hierarchyFixtures(t) {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", "projection", name+"-hierarchy.json")
			got, err := json.MarshalIndent(item, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			if *updateNativeProjectionGolden {
				if err := os.WriteFile(path, got, 0600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var a, b any
			if json.Unmarshal(got, &a) != nil || json.Unmarshal(want, &b) != nil || !reflect.DeepEqual(a, b) {
				t.Fatalf("hierarchy golden mismatch\nwant=%s\ngot=%s", want, got)
			}
			if strings.Contains(string(got), "/115/") || strings.Contains(string(got), "pickCode") {
				t.Fatal("private value leaked")
			}
		})
	}
}
func TestHierarchyProjectionSpecialAndMultiEpisodeRules(t *testing.T) {
	items := hierarchyFixtures(t)
	season := items["season"]
	if season.IndexNumber == nil || *season.IndexNumber != 0 || season.Name != "特别篇" {
		t.Fatal(season)
	}
	ep := items["episode"]
	if ep.IndexNumberEnd == nil || *ep.IndexNumberEnd != 3 || len(ep.MediaSources) != 2 || ep.ParentIndexNumber == nil || *ep.ParentIndexNumber != 0 {
		t.Fatal(ep)
	}
	if ep.SeriesPrimaryImageTag == "" || len(ep.ParentBackdropImageTags) != 1 || ep.ParentBackdropItemId != "100201" {
		t.Fatal(ep)
	}
	if ep.Path != "Synthetic Episode.mkv" {
		t.Fatal(ep.Path)
	}
}

func TestStillTagChangesWithFingerprint(t *testing.T) {
	a := StillImageTag(strings.Repeat("a", 64), strings.Repeat("b", 64))
	b := StillImageTag(strings.Repeat("c", 64), strings.Repeat("b", 64))
	if a == b || len(a) != 64 || StillImageTag("", strings.Repeat("b", 64)) != "" {
		t.Fatal("fingerprint tag contract")
	}
	fixtures := hierarchyFixtures(t)
	if fixtures["episode-local"].ImageTags["Primary"] != StillImageTag(strings.Repeat("c", 64), strings.Repeat("d", 64)) || fixtures["episode-poster"].ImageTags["Primary"] != ImageTag("/special.jpg") {
		t.Fatal("image priority")
	}
}
