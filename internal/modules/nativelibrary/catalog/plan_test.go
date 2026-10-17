package catalog

import (
	"media-tool/internal/modules/classification"
	"media-tool/internal/modules/nativelibrary"
	"reflect"
	"testing"
)

func TestPlanDefaultsUsesRenderedDirectoriesAndStableOrder(t *testing.T) {
	plans, err := Plan(classification.Defaults(), nativelibrary.GranularityRule)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 19 {
		t.Fatalf("default libraries=%d", len(plans))
	}
	for i, item := range plans {
		if item.SortOrder != i {
			t.Fatal("sort order", item)
		}
		if item.ID == "" || item.Directory == "" || item.CollectionType == "" {
			t.Fatal(item)
		}
	}
	want := []struct{ id, dir, name string }{{"r:movie:movie-documentary", "电影/纪录片", "纪录片 · 电影"}, {"r:series:series-doc", "电视剧/纪录片", "纪录片 · 电视剧"}}
	for _, expected := range want {
		found := false
		for _, item := range plans {
			if item.ID == expected.id {
				found = true
				if item.Directory != expected.dir || item.Name != expected.name {
					t.Fatal(item)
				}
			}
		}
		if !found {
			t.Fatal("missing", expected.id)
		}
	}
	rendered := classification.Match{Directory: "电影/纪录片"}
	found := false
	for _, item := range plans {
		if item.ID == "r:movie:movie-documentary" {
			found = true
			if rendered.Directory != item.Directory {
				t.Fatal("directory drift", rendered.Directory, item.Directory)
			}
		}
	}
	if !found {
		t.Fatal("rendered rule missing")
	}
}
func TestPlanRuleGroupFallbackAndGranularity(t *testing.T) {
	policy := classification.Policy{Groups: []classification.Group{{ID: "movie", Name: "电影", Directory: "电影", MediaType: "movie", Enabled: true, Rules: []classification.Rule{{ID: "disabled", Name: "失效", Directory: "失效", Enabled: false}}}, {ID: "series", Name: "电视剧", Directory: "电视剧", MediaType: "series", Enabled: true, Rules: []classification.Rule{{ID: "r1", Name: "纪录片", Directory: "纪录片", Enabled: true}}}, {ID: "off", Name: "关闭", Directory: "关闭", MediaType: "movie", Enabled: false}}}
	rule, err := Plan(policy, nativelibrary.GranularityRule)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]string{rule[0].ID, rule[1].ID}, []string{"g:movie", "r:series:r1"}) {
		t.Fatal(rule)
	}
	group, err := Plan(policy, nativelibrary.GranularityGroup)
	if err != nil {
		t.Fatal(err)
	}
	if len(group) != 2 || group[0].ID != "g:movie" || group[1].ID != "g:series" || group[0].CollectionType != nativelibrary.CollectionMovies || group[1].CollectionType != nativelibrary.CollectionTVShows {
		t.Fatal(group)
	}
}
func TestPlanSameNamesWithinGroupRemainUniqueAndInvalidGranularityRejected(t *testing.T) {
	policy := classification.Policy{Groups: []classification.Group{{ID: "g", Name: "同名", Directory: "g", MediaType: "movie", Enabled: true, Rules: []classification.Rule{{ID: "a", Name: "重复", Directory: "a", Enabled: true}, {ID: "b", Name: "重复", Directory: "b", Enabled: true}}}}}
	plans, err := Plan(policy, nativelibrary.GranularityRule)
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].Name == plans[1].Name {
		t.Fatal("duplicate display names", plans)
	}
	if _, err := Plan(policy, nativelibrary.LibraryGranularity("invalid")); err == nil {
		t.Fatal("invalid granularity accepted")
	}
}
func TestPlanDisabledAndDeletedPoliciesProduceNoNewLibraries(t *testing.T) {
	p := classification.Policy{Groups: []classification.Group{{ID: "g", Name: "关闭", Directory: "关闭", MediaType: "movie", Enabled: false}}}
	plans, err := Plan(p, nativelibrary.GranularityRule)
	if err != nil || len(plans) != 0 {
		t.Fatal(plans, err)
	}
}
