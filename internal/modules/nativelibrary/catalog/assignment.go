package catalog

import (
	"media-tool/internal/modules/nativelibrary"
	"path"
	"strings"
	"unicode/utf8"
)

type Output struct{ AssetID, Path string }
type Assignment struct {
	LibraryID, OutputPath string
	Source                string
	Diagnostic            map[string]any
}

func cleanSegments(value string) ([]string, bool) {
	if value == "" || !utf8.ValidString(value) || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return nil, false
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return nil, false
	}
	return strings.Split(cleaned, "/"), true
}
func segmentPrefix(directory, output string) (int, bool) {
	want, ok := cleanSegments(directory)
	if !ok {
		return 0, false
	}
	have, ok := cleanSegments(output)
	if !ok || len(have) < len(want) {
		return 0, false
	}
	for i := range want {
		if want[i] != have[i] {
			return 0, false
		}
	}
	return len(want), true
}
func MatchOutputs(outputs []Output, libraries []Library, mediaType string) Assignment {
	best := Assignment{}
	bestDepth := -1
	for _, output := range outputs {
		for _, library := range libraries {
			expected := nativelibrary.CollectionMovies
			if mediaType == "series" {
				expected = nativelibrary.CollectionTVShows
			}
			if library.CollectionType != expected {
				continue
			}
			depth, ok := segmentPrefix(library.Directory, output.Path)
			if !ok || depth < bestDepth {
				continue
			}
			candidate := Assignment{LibraryID: library.ID, OutputPath: output.Path, Source: "output_directory", Diagnostic: map[string]any{}}
			if depth > bestDepth || candidate.LibraryID < best.LibraryID || candidate.OutputPath < best.OutputPath {
				best = candidate
				bestDepth = depth
			}
		}
	}
	if bestDepth >= 0 {
		return best
	}
	return Assignment{Diagnostic: map[string]any{}, Source: "output_directory"}
}
func FallbackLibraryID(mediaType string) string {
	if mediaType == "series" {
		return nativelibrary.FallbackSeriesLibraryID
	}
	if mediaType == "movie" {
		return nativelibrary.FallbackMovieLibraryID
	}
	return ""
}
