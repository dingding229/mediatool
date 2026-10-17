package catalog

import (
	"media-tool/internal/modules/nativelibrary"
	"reflect"
	"testing"
)

func TestMatchOutputsUsesSegmentBoundariesAndLongestDirectory(t *testing.T) {
	libs := []Library{{ID: "short", Directory: "电影/华语", CollectionType: nativelibrary.CollectionMovies}, {ID: "long", Directory: "电影/华语电影", CollectionType: nativelibrary.CollectionMovies}, {ID: "wrong", Directory: "电影/华语电影", CollectionType: nativelibrary.CollectionTVShows}}
	for _, tc := range []struct{ path, want string }{{"电影/华语电影/片.strm", "long"}, {"电影/华语电影2/片.strm", ""}, {"电影/华语电影/空 格.strm", "long"}, {"电影/华语电影", "long"}} {
		got := MatchOutputs([]Output{{AssetID: "a", Path: tc.path}}, libs, "movie")
		if got.LibraryID != tc.want {
			t.Fatal(tc, got)
		}
	}
}
func TestMatchOutputsFallbackAndNoOutputs(t *testing.T) {
	libs := []Library{{ID: "f:movie", Directory: "电影", CollectionType: nativelibrary.CollectionMovies}}
	got := MatchOutputs([]Output{{AssetID: "a", Path: "Other/asset.strm"}}, libs, "movie")
	if got.LibraryID != "" {
		t.Fatal(got)
	}
	if FallbackLibraryID("movie") != "f:movie" || FallbackLibraryID("series") != "f:series" {
		t.Fatal("fallback IDs")
	}
	if !reflect.DeepEqual(MatchOutputs(nil, libs, "movie"), Assignment{Source: "output_directory", Diagnostic: map[string]any{}}) {
		t.Fatal("empty outputs")
	}
}
