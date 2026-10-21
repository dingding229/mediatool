package metadata

import (
	"media-tool/internal/platform/mediaext"
	"reflect"
	"strings"
	"testing"
)

func TestEpisodeCoverage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		season   int
		episodes []int
		ok       bool
	}{{"Show.S01E01.mkv", 1, []int{1}, true}, {"Show.S01E01E02E03.mkv", 1, []int{1, 2, 3}, true}, {"Show.S02E01-E04.mkv", 2, []int{1, 2, 3, 4}, true}, {"Show.S01E01-S02E02.mkv", 0, nil, false}, {"Show.S01.mkv", 0, nil, false}, {"Show.S00E01.mp4", 0, []int{1}, true}, {"Show.S01E01-1000.mkv", 0, nil, false}} {
		t.Run(tc.name, func(t *testing.T) {
			s, eps, ok := Episodes(tc.name)
			if s != tc.season || ok != tc.ok || !reflect.DeepEqual(eps, tc.episodes) {
				t.Fatal(s, eps, ok)
			}
		})
	}
}

func TestIsVideoUsesSharedPolicy(t *testing.T) {
	for _, ext := range mediaext.VideoExtensions() {
		if !IsVideo("Show.S01E01" + strings.ToUpper(ext)) {
			t.Fatal("video missed", ext)
		}
	}
	for _, ext := range append(mediaext.SubtitleExtensions(), ".iso", ".nfo", ".strm", "") {
		if IsVideo("Show.S01E01" + ext) {
			t.Fatal("nonvideo accepted", ext)
		}
	}
}
