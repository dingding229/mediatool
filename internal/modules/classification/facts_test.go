package classification

import (
	"media-tool/internal/modules/metadata"
	"reflect"
	"testing"
)

func TestChineseCountryAndLanguageRulesRemainCorrect(t *testing.T) {
	p := Defaults()
	for _, country := range []string{"CN", "TW", "HK", "MO"} {
		for _, language := range []string{"zh", "cn", "yue"} {
			for _, kind := range []string{"movie", "series"} {
				i := metadata.Item{MediaType: kind, Countries: []string{country}, OriginalLanguage: language, GenreIDs: []int{18}}
				got := p.Match(i)
				want := "movie-chinese"
				if kind == "series" {
					want = "series-cn"
				}
				if got.RuleID != want {
					t.Fatalf("%s/%s/%s: %s", kind, country, language, got.RuleID)
				}
			}
		}
	}
}
func TestProviderFactsNormalizeAndOnlyApplyMatchingInputs(t *testing.T) {
	f, e := FactsFromItem(metadata.Item{GenreIDs: []int{18, 18}, Countries: []string{" tw ", "TW", "HK"}, OriginalLanguage: " ZH "})
	if e != nil || !f.Valid() {
		t.Fatal(f, e)
	}
	if !reflect.DeepEqual(f.Countries, []string{"TW", "HK"}) || len(f.GenreIDs) != 1 || f.OriginalLanguage != "zh" {
		t.Fatal(f)
	}
	base := metadata.Item{Title: "保留原题名", Overview: "保留简介", PosterPath: "/poster.jpg", Genres: []string{"原题材名"}, Countries: []string{"US"}, OriginalLanguage: "en", GenreIDs: []int{99}}
	missing := base
	missing.Countries = nil
	missing.GenreIDs = nil
	missing.OriginalLanguage = ""
	got := f.Apply(missing)
	if got.Title != base.Title || got.Overview != base.Overview || got.PosterPath != base.PosterPath || !reflect.DeepEqual(got.Genres, base.Genres) || got.OriginalLanguage != "zh" || got.Countries[0] != "TW" {
		t.Fatal(got)
	}
	if !reflect.DeepEqual(f.Apply(base), base) {
		t.Fatal("known ingest facts overwritten")
	}
	empty, e := FactsFromItem(metadata.Item{})
	if e != nil || !empty.Valid() || !reflect.DeepEqual(empty.Apply(base), base) {
		t.Fatal(empty, e)
	}
	if !reflect.DeepEqual((Facts{Version: 999}).Apply(base), base) {
		t.Fatal("unknown version changed input")
	}
	for _, i := range []metadata.Item{{Countries: []string{"../secret"}}, {OriginalLanguage: "https://invalid"}, {GenreIDs: []int{-1}}, {GenreIDs: []int{100001}}} {
		if _, e := FactsFromItem(i); e == nil {
			t.Fatal("invalid facts accepted", i)
		}
	}
}
