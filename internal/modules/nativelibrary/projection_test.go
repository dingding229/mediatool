package nativelibrary

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi/dto"
)

var updateNativeProjectionGolden = flag.Bool("update-native-projection", false, "update native media projection goldens")

func projectionFixtures(t *testing.T) map[string]dto.BaseItemDto {
	t.Helper()
	stamp := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	rating := float32(8.4)
	runtime := int64(72000000000)
	fullMovie, err := MovieDTO(Movie{
		ID: "100101", ServerID: strings.Repeat("a", 32), ParentID: "100001", Name: "Synthetic Film", Year: "2026", TMDBID: 900101,
		Metadata: StoredMovieMetadata{OriginalTitle: "Synthetic Original", SortName: "Synthetic Film, The", Overview: "Synthetic overview", Genres: []string{"Drama", "Science Fiction"}, PosterPath: "/synthetic-poster.jpg", BackdropPath: "/synthetic-backdrop.jpg", PremiereDate: "2026-01-02", CommunityRating: &rating, OfficialRating: "PG-13", ImdbID: "tt900101"},
		Details:  &TMDBDetails{LogoPath: "/film-logo.png", Backdrops: []string{"/synthetic-backdrop.jpg", "/second-bg.jpg"}}, CreatedAt: stamp, Sources: []MovieSource{{ID: "asset-101", Container: "mkv", RunTimeTicks: &runtime}, {ID: "asset-102", Container: "mp4"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	minMovie, err := MovieDTO(Movie{ID: "100102", ParentID: "100001", Name: "Minimal Film", Year: "", TMDBID: 900102, CreatedAt: stamp})
	if err != nil {
		t.Fatal(err)
	}
	fullLibrary, err := LibraryDTO(Library{ItemID: "100001", ServerID: strings.Repeat("a", 32), Name: "Synthetic Movies", CollectionType: CollectionMovies})
	if err != nil {
		t.Fatal(err)
	}
	minLibrary, err := LibraryDTO(Library{ItemID: "100002", Name: "Minimal Library", CollectionType: CollectionTVShows})
	if err != nil {
		t.Fatal(err)
	}
	withDetails, err := MovieDTO(Movie{ID: "100103", ParentID: "100001", Name: "Snapshot Film", Year: "2026", TMDBID: 900103, CreatedAt: stamp, Details: &TMDBDetails{StoredMovieMetadata: StoredMovieMetadata{PremiereDate: "2026-03-04", OfficialRating: "PG-13", ImdbID: "tt900103", Genres: []string{"Drama"}, CommunityRating: func() *float32 { v := float32(8.8); return &v }()}, RuntimeMinutes: 120, People: []TMDBPerson{{ID: 901, Name: "Synthetic Actor", Type: "Actor", Role: "Lead", ImagePath: "/actor.jpg"}}}})
	if err != nil {
		t.Fatal(err)
	}
	return map[string]dto.BaseItemDto{"movie-full": fullMovie, "movie-min": minMovie, "movie-tmdb": withDetails, "library-full": fullLibrary, "library-min": minLibrary}
}

func TestNativeProjectionGoldens(t *testing.T) {
	for name, value := range projectionFixtures(t) {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", "projection", name+".json")
			got, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			if *updateNativeProjectionGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var gotJSON, wantJSON any
			if err := json.Unmarshal(got, &gotJSON); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(want, &wantJSON); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotJSON, wantJSON) {
				t.Fatalf("golden mismatch\nwant=%s\ngot=%s", want, got)
			}
			var roundtrip dto.BaseItemDto
			if err := json.Unmarshal(want, &roundtrip); err != nil {
				t.Fatal(err)
			}
			remarshal, err := json.Marshal(roundtrip)
			if err != nil {
				t.Fatal(err)
			}
			if !jsonEqual(remarshal, want) {
				t.Fatalf("golden is not round-trip stable: %s", want)
			}
			if bytes.Contains(want, []byte("null")) {
				t.Fatal("projection golden contains null")
			}
			if strings.Contains(string(want), "/115/") || strings.Contains(string(want), "pickCode") || strings.Contains(string(want), "strm302") {
				t.Fatal("sensitive value in golden")
			}
		})
	}
}
func jsonEqual(a, b []byte) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

func validProjectionMovie() Movie {
	return Movie{ID: "100101", ParentID: "100001", Name: "Synthetic Film", TMDBID: 900101, CreatedAt: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)}
}
func TestMovieProjectionRejectsUnsafeOrIncompleteValues(t *testing.T) {
	for name, change := range map[string]func(*Movie){
		"id":              func(m *Movie) { m.ID = "not-numeric" },
		"parent":          func(m *Movie) { m.ParentID = "123" },
		"tmdb":            func(m *Movie) { m.TMDBID = 0 },
		"empty-name":      func(m *Movie) { m.Name = " " },
		"empty-created":   func(m *Movie) { m.CreatedAt = time.Time{} },
		"year":            func(m *Movie) { m.Year = "not-year" },
		"year-bound":      func(m *Movie) { m.Year = "10000" },
		"container":       func(m *Movie) { m.Sources = []MovieSource{{ID: "synthetic-asset", Container: "bad/ext"}} },
		"source-id":       func(m *Movie) { m.Sources = []MovieSource{{Container: "mkv"}} },
		"imdb":            func(m *Movie) { m.Metadata.ImdbID = "http://bad" },
		"premiere":        func(m *Movie) { m.Metadata.PremiereDate = "not-a-date" },
		"rating":          func(m *Movie) { v := float32(11); m.Metadata.CommunityRating = &v },
		"overview-secret": func(m *Movie) { m.Metadata.Overview = "/115/private" },
	} {
		t.Run(name, func(t *testing.T) {
			movie := validProjectionMovie()
			change(&movie)
			if _, err := MovieDTO(movie); err == nil {
				t.Fatal("invalid projection accepted")
			}
		})
	}
	for _, lib := range []Library{
		{ItemID: "100001", Name: "x", CollectionType: CollectionMovies, ServerID: "/secret"},
		{ItemID: "100001", Name: "x", CollectionType: "unknown"},
		{ItemID: "bad", Name: "x", CollectionType: CollectionMovies},
	} {
		if _, err := LibraryDTO(lib); err == nil {
			t.Fatal("invalid library accepted")
		}
	}
}
func TestProjectionKeepsZeroValuesAndOnlySyntheticLeafPaths(t *testing.T) {
	movie := validProjectionMovie()
	movie.Name = "Synthetic / Part\\One .. : ? & # % = Title"
	movie.Metadata.PosterPath = "/synthetic-raw-poster.jpg"
	movie.Metadata.BackdropPath = "/synthetic-raw-backdrop.jpg"
	zero := int64(0)
	rating := float32(0)
	movie.RunTimeTicks = &zero
	movie.Metadata.CommunityRating = &rating
	movie.Sources = []MovieSource{{ID: "synthetic-source", Container: "MP4", RunTimeTicks: &zero}}
	item, err := MovieDTO(movie)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(item.Path, `/\\:?&=#%`) || strings.Contains(item.Path, "..") || !strings.HasSuffix(item.Path, ".mp4") {
		t.Fatal(item.Path)
	}
	if item.ImageTags["Primary"] != ImageTag(movie.Metadata.PosterPath) || len(item.ImageTags["Primary"]) != 64 {
		t.Fatal(item.ImageTags)
	}
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"RunTimeTicks":0`, `"CommunityRating":0`, `"UserData":{`, `"Genres":[]`, `"MediaStreams":[]`, `"People":[]`} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("missing %s in %s", want, raw)
		}
	}
	if bytes.Contains(raw, []byte(movie.Metadata.PosterPath)) || bytes.Contains(raw, []byte(movie.Metadata.BackdropPath)) {
		t.Fatal("raw image path leaked")
	}
	if item.MediaSources[0].Path != "" {
		t.Fatal("stream URLs must wait for T31")
	}
}
