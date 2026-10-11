package library

import (
	"context"
	"media-tool/internal/modules/metadata"
	"strings"
	"testing"
)

func TestDefaultNamingTemplatesPreserveEditionAndSparseCoverage(t *testing.T) {
	ctx := context.Background()
	asset := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	movie := metadata.Item{Title: "Owned Film", Year: "2026", MediaType: "movie"}
	p, err := AssetOutputProjections("/115/Owned.Film.2026.1080p.WEB-DL-GRP.mkv", asset, movie, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err = ApplyOutputTemplates(ctx, nil, "/115/Owned.Film.2026.1080p.WEB-DL-GRP.mkv", asset, movie, p)
	if err != nil || p[0].Path != "Movies/Owned Film (2026)/Owned Film (2026) - 1080p [aaaaaaaa].strm" {
		t.Fatal(p, err)
	}
	season := 1
	series := metadata.Item{Title: "Owned Show", Year: "2026", MediaType: "series"}
	p, err = AssetOutputProjections("/115/Owned.Show.S01E01E03.1080p.mkv", asset, series, &season, []int{1, 3})
	if err != nil {
		t.Fatal(err)
	}
	p, err = ApplyOutputTemplates(ctx, nil, "/115/Owned.Show.S01E01E03.1080p.mkv", asset, series, p)
	if err != nil || len(p) != 2 || !strings.Contains(p[0].Path, "S01E01") || !strings.Contains(p[1].Path, "S01E03") {
		t.Fatal(p, err)
	}
}
