package nativelibrary

import (
	"encoding/json"
	"media-tool/internal/modules/embyapi/dto"
	"os"
	"path/filepath"
	"testing"
)

func TestT3EffectiveTextDTOGolden(t *testing.T) {
	rating := float32(6.7)
	out := []dto.BaseItemDto{}
	for _, kind := range []dto.ItemType{dto.Movie, dto.Series, dto.Season, dto.Episode} {
		x, e := dto.NewBaseItemDto("100101", "人工标题", kind)
		if e != nil {
			t.Fatal(e)
		}
		m := StoredMovieMetadata{OriginalTitle: "Original", SortName: "Manual Sort", Overview: "人工简介", Genres: []string{"剧情"}, Tags: []string{"测试"}, Tagline: "人工标语", CommunityRating: &rating, OfficialRating: "PG", PremiereDate: "2026-05-01", SeriesName: "人工剧集", SeasonName: "人工季"}
		ApplyEffectiveText(&x, m)
		out = append(out, x)
	}
	raw, e := json.MarshalIndent(out, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	raw = append(raw, '\n')
	path := filepath.Join("testdata", "projection", "t3-effective-text.json")
	if *updateNativeProjectionGolden {
		if e = os.WriteFile(path, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	expected, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if string(expected) != string(raw) {
		t.Fatalf("effective text DTO golden mismatch\n%s", raw)
	}
}
