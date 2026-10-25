package metadataedit

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidationLimitsNormalizationAndEmptyLocks(t *testing.T) {
	cases := []Values{{"title": json.RawMessage(`""`)}, {"title": raw(strings.Repeat("中", 501))}, {"originalTitle": raw(strings.Repeat("a", 501))}, {"sortName": raw(strings.Repeat("a", 501))}, {"overview": raw(strings.Repeat("a", 10001))}, {"tagline": raw(strings.Repeat("a", 1001))}, {"year": raw(1799)}, {"year": raw(2101)}, {"year": raw(2025.5)}, {"premiereDate": raw("2025-02-29")}, {"premiereDate": raw("2024-13-01")}, {"communityRating": raw(-0.1)}, {"communityRating": raw(10.1)}, {"genres": raw([]string{strings.Repeat("a", 101)})}, {"tags": raw(make([]string, 51))}, {"genres": raw(nil)}, {"countries": raw([]string{"TW"})}, {"tags": raw([]string{"bad\x00tag"})}}
	for _, v := range cases {
		p := Patch{TargetKey: "fixture", Set: v}
		if e := p.Validate(); !errors.Is(e, ErrInvalid) {
			t.Fatalf("accepted %v: %v", v, e)
		}
	}
	p := Patch{TargetKey: "fixture", Set: Values{"title": raw(strings.Repeat("中", 500)), "genres": raw([]string{" 剧情 ", "", "剧情", "动作"}), "tags": raw([]string{}), "communityRating": raw(6.66), "premiereDate": raw("2024-02-29"), "year": raw(2100), "overview": raw(strings.Repeat("中", 10000))}}
	if e := p.Validate(); e != nil {
		t.Fatal(e)
	}
	if string(p.Set["genres"]) != `["剧情","动作"]` || string(p.Set["communityRating"]) != "6.7" {
		t.Fatal(p.Set)
	}
	p = Patch{TargetKey: "fixture", Set: Values{"overview": raw(""), "communityRating": raw(nil), "genres": raw([]string{})}}
	if e := p.Validate(); e != nil {
		t.Fatal(e)
	}
	got := Merge(Values{"overview": raw("provider"), "genres": raw([]string{"Drama"}), "communityRating": raw(8)}, p.Set)
	if string(got["overview"]) != `""` || string(got["communityRating"]) != "null" || string(got["genres"]) != "[]" {
		t.Fatal(got)
	}
	p = Patch{TargetKey: "fixture", Set: Values{"title": raw("Title")}, Restore: []string{"title"}}
	if !errors.Is(p.Validate(), ErrInvalid) {
		t.Fatal("set/restore conflict accepted")
	}
}
func raw(v any) json.RawMessage { r, _ := json.Marshal(v); return r }
