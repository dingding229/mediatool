package filemanager

import (
	"encoding/json"
	"errors"
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/metadata"
	"os"
	"path"
	"strings"
	"testing"
)

func TestOrganizeLayoutMatchesIngestionGolden(t *testing.T) {
	raw, err := os.ReadFile("testdata/organize-layout.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Source          string
		Media           metadata.Item
		Season, Episode int
		Path, Category  string
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	policy := classification.Defaults()
	movie := "{title} ({year})/{title} ({year}) - {videoFormat}"
	series := "{title} ({year})/Season {season}/{title} - {season_episode}"
	for _, c := range cases {
		t.Run(c.Media.MediaType, func(t *testing.T) {
			in := Input{Operation: "organize", Title: "Untrusted stale title", Season: c.Season, Episode: c.Episode}
			got, err := organizeLayout(in, File{Name: c.Source}, c.Media, policy, movie, series)
			if err != nil {
				t.Fatal(err)
			}
			episodes := []int{}
			if c.Media.MediaType == "series" {
				episodes = []int{c.Episode}
			}
			ingestion, match, err := classification.Render(policy, c.Media, c.Source, movie, series, ".mkv", c.Season, episodes)
			if err != nil || path.Join(got.Destination, got.Name) != ingestion || ingestion != c.Path || got.Category != match.Directory || got.Category != c.Category {
				t.Fatalf("file=%+v ingestion=%s match=%+v err=%v", got, ingestion, match, err)
			}
			if got.Title != c.Media.Title || got.TMDBID != c.Media.ID {
				t.Fatal("metadata lost", got)
			}
			if c.Media.MediaType == "movie" && (got.Season != 0 || got.Episode != 0 || strings.Contains(got.Name, "S08")) {
				t.Fatal("movie kept season", got)
			}
			in.Category = "../../escape"
			if _, err = organizeLayout(in, File{Name: c.Source}, c.Media, policy, movie, series); !errors.Is(err, ErrInvalid) {
				t.Fatal("arbitrary category accepted", err)
			}
		})
	}
}
func TestMovieNamingNeverRendersSeasonFields(t *testing.T) {
	for _, template := range []string{"{title} S{season}{ext}", "{title} E{episode}{ext}", "{title} {season_episode}{ext}"} {
		_, err := FileName(Input{Title: "Film", MediaType: "movie", Season: 3, Episode: 7, Template: template}, File{Name: "Film.mkv"})
		if !errors.Is(err, ErrInvalid) {
			t.Fatal(template, err)
		}
		_, err = organizeLayout(Input{Template: template, Season: 3, Episode: 7}, File{Name: "Film.mkv"}, metadata.Item{Title: "Film", MediaType: "movie"}, classification.Defaults(), "{title}", "{title}")
		if !errors.Is(err, ErrInvalid) {
			t.Fatal(template, err)
		}
	}
}
func TestOrganizeManualCategoryAndDirectoryStayExplicit(t *testing.T) {
	media := metadata.Item{Title: "Film", MediaType: "movie", Year: "2026"}
	got, err := organizeLayout(Input{Destination: "Workspace", Category: "电影/华语电影", Name: "custom.mkv"}, File{Name: "old.mkv"}, media, classification.Defaults(), "{title}", "{title}")
	if err != nil || got.Destination != "Workspace" || got.Name != "custom.mkv" {
		t.Fatal(got, err)
	}
	got, err = organizeLayout(Input{}, File{Name: "Film.2026", Directory: true}, media, classification.Defaults(), "{title}", "{title}")
	if err != nil || got.Name != "Film.2026" || got.Destination != "电影/其他电影" {
		t.Fatal(got, err)
	}
}

func TestMultiEpisodeFilesStillNeedAnExplicitCompleteName(t *testing.T) {
	media := metadata.Item{Title: "Show", MediaType: "series", Year: "2026"}
	in := Input{Season: 1, Episode: 1}
	if _, err := organizeLayout(in, File{Name: "Show.S01E01E02.mkv"}, media, classification.Defaults(), "{title}", "{title}/{season_episode}"); !errors.Is(err, ErrInvalid) {
		t.Fatal("multi-episode default filename accepted", err)
	}
	in.Name = "Show - S01E01E02.mkv"
	if got, err := organizeLayout(in, File{Name: "Show.S01E01E02.mkv"}, media, classification.Defaults(), "{title}", "{title}/{season_episode}"); err != nil || got.Name != in.Name {
		t.Fatal("explicit complete name rejected", got, err)
	}
}
