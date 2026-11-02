package cacheview

import (
	"encoding/json"
	"testing"
)

func TestTMDBConditionsProjection(t *testing.T) {
	for _, tt := range []struct {
		raw, operation, kind, year, language string
		id                                   int
	}{
		{`{"query":"嫁金钗","items":[],"mediaType":"auto","year":"2026","language":"zh-CN"}`, "search", "auto", "2026", "zh-CN", 0},
		{`{"query":"嫁金钗","item":{"id":12,"year":"2026"},"mediaType":"series"}`, "details", "series", "2026", "", 12},
	} {
		var m map[string]any
		if e := json.Unmarshal([]byte(tt.raw), &m); e != nil {
			t.Fatal(e)
		}
		got := tmdbConditions(m)
		if got.Operation != tt.operation || got.MediaType != tt.kind || got.Year != tt.year || got.Language != tt.language || got.TMDBID != tt.id {
			t.Fatalf("%+v", got)
		}
	}
}
