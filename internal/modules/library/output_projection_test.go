package library

import (
	"encoding/xml"
	"reflect"
	"strings"
	"testing"

	"media-tool/internal/modules/metadata"
)

func TestSparseEmbyProjectionOnlyIndexesDeclaredEpisodes(t *testing.T) {
	item := metadata.Item{ID: 1, MediaType: "series", Title: "Owned Show", Year: "2026"}
	for _, test := range []struct {
		season int
		eps    []int
		want   []string
	}{{1, []int{3, 1}, []string{"S01E01", "S01E03"}}, {0, []int{1, 3}, []string{"S00E01", "S00E03"}}, {2, []int{1, 2, 3}, []string{"S02E01-E03"}}, {1, []int{1, 2, 4, 5}, []string{"S01E01", "S01E02", "S01E04", "S01E05"}}} {
		plans, err := AssetOutputProjections("/115/Owned/video.mkv", "owned-asset-identifier", item, &test.season, test.eps)
		if err != nil || len(plans) != len(test.want) {
			t.Fatal(plans, err)
		}
		union := []int{}
		for i, p := range plans {
			if !strings.Contains(p.Path, test.want[i]) || !p.WholeFilePlayback {
				t.Fatal("wrong alias", p)
			}
			union = append(union, p.Episodes...)
		}
		expected, _ := metadata.NormalizeEpisodes(test.season, test.eps)
		if !reflect.DeepEqual(union, expected) {
			t.Fatal("Emby output omitted/added coverage", union, expected)
		}
	}
}
func TestAliasNFOHasOwnEpisodeIdentityAndExplicitWholeFileNotice(t *testing.T) {
	season := 1
	item := metadata.Item{ID: 10, Title: "Owned Show", MediaType: "series", EpisodeDetails: []metadata.Episode{{ID: 101, SeriesID: 10, Season: 1, Number: 1, Title: "Owned one", Runtime: 20}, {ID: 103, SeriesID: 10, Season: 1, Number: 3, Title: "Owned three", Runtime: 21}}}
	plans, err := AssetOutputProjections("/115/Owned/file.mkv", "owned-asset-identifier", item, &season, []int{1, 3})
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range plans {
		parts, err := nfoProjectionFiles(p.Path, "asset", item, &season, p.Episodes, p.SourceEpisodes)
		if err != nil {
			t.Fatal(err)
		}
		var episode nfo
		if err = xml.Unmarshal(parts[1].data, &episode); err != nil {
			t.Fatal(err)
		}
		if episode.Episode == nil || *episode.Episode != []int{1, 3}[i] || episode.Runtime != nil || !strings.Contains(episode.Plot, "整个原文件") || !strings.Contains(episode.Plot, "1,3") {
			t.Fatal("alias implies clipping or wrong episode", episode)
		}
		if episode.Unique == nil || episode.Unique.Value != []string{"101", "103"}[i] {
			t.Fatal("aliases reuse representative provider identity")
		}
	}
}
func TestNonMigratingCorrectionCannotHideSparseConsumerMisinterpretation(t *testing.T) {
	if _, err := correctionProjections([]oldOutput{{Path: "TV/Owned/Season 01/Owned - S01E01E03.strm"}}, 1, []int{1, 3}); err == nil {
		t.Fatal("legacy chain accepted without layout correction")
	}
	entries := []oldOutput{{Path: "TV/Owned/Season 01/Owned - S01E01.strm"}, {Path: "TV/Owned/Season 01/Owned - S01E03.strm"}}
	if got, err := correctionProjections(entries, 1, []int{1, 3}); err != nil || len(got) != 2 {
		t.Fatal(err)
	}
	if _, err := correctionProjections(entries, 1, []int{1, 2, 3}); err == nil {
		t.Fatal("missing declared alias accepted")
	}
	if _, err := correctionProjections(entries, 2, []int{1, 3}); err == nil {
		t.Fatal("season correction did not require layout preview")
	}
}

func TestExistingApprovedProjectionUsesCoverageWithoutForcingASecondRename(t *testing.T) {
	season := 1
	paths := []string{"TV/Old approved folder/Season 01/Kept - S01E01 [edition].strm", "TV/Old approved folder/Season 01/Kept - S01E03 [edition].strm"}
	projections, err := ExistingOutputProjections(paths, "series", &season, []int{1, 3})
	if err != nil || len(projections) != 2 || projections[0].Path != paths[0] || projections[1].Path != paths[1] {
		t.Fatal("resume forced another output rename", err)
	}
	if _, err = ExistingOutputProjections([]string{paths[0], paths[0], paths[1]}, "series", &season, []int{1, 3}); err == nil {
		t.Fatal("duplicate episode alias counted twice")
	}
	if _, err = ExistingOutputProjections([]string{"TV/Old/Kept - S01E01E03.strm"}, "series", &season, []int{1, 3}); err == nil {
		t.Fatal("legacy expanded range accepted as complete")
	}
}
