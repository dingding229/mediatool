package classification

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/metadata"
	"reflect"
	"strings"
	"testing"
)

func TestClassificationDefaultsAndTemplates(t *testing.T) {
	p := Defaults()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		item metadata.Item
		want string
	}{{metadata.Item{MediaType: "series", GenreIDs: []int{16}, Countries: []string{"JP"}}, "动漫/日漫"}, {metadata.Item{MediaType: "movie", GenreIDs: []int{99}, Countries: []string{"US"}}, "电影/纪录片"}, {metadata.Item{MediaType: "movie", OriginalLanguage: "zh"}, "电影/华语电影"}, {metadata.Item{MediaType: "series"}, "电视剧/其他剧"}} {
		if got := p.Match(c.item).Directory; got != c.want {
			t.Fatal(got, c.want)
		}
	}
	name, _, err := Render(p, metadata.Item{ID: 1, MediaType: "movie", Title: "一生所爱", Year: "2026", OriginalLanguage: "en", Countries: []string{"US"}}, "Original.Release.2026.1080p.mkv", "{title} ({year})/{title} ({year}) - {videoFormat}", "", ".mkv", 0, nil)
	if err != nil || name != "电影/欧美电影/一生所爱 (2026)/一生所爱 (2026) - 1080p.mkv" || strings.Contains(name, "[") {
		t.Fatal(name, err)
	}
	strm, _, err := Render(p, metadata.Item{ID: 2, MediaType: "series", Title: "剧集", Year: "2026", Countries: []string{"CN"}}, "Show.S00E02E04.1080p.mkv", "", "{title} ({year})/Season {season}/{title} - {season_episode} - 第{episode}集", ".strm", 0, []int{2, 4})
	if err != nil || !strings.Contains(strm, "Season 00") || !strings.Contains(strm, "S00E02E04") {
		t.Fatal(strm, err)
	}
	bad := Defaults()
	bad.Groups[0].Directory = "../escape"
	if bad.Validate() == nil {
		t.Fatal("path escape accepted")
	}
}

func TestDefaultPolicySerializesCollectionFieldsAsArrays(t *testing.T) {
	record, err := (Repository{}).Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"genreIds":null`) || strings.Contains(string(raw), `"countries":null`) || strings.Contains(string(raw), `"languages":null`) || strings.Contains(string(raw), `"rules":null`) {
		t.Fatal(string(raw))
	}
}

func TestMatchGroupWithoutEnabledRulesUsesGroupDirectory(t *testing.T) {
	policy := Policy{Groups: []Group{{ID: "g", Name: "自定义电影", Directory: "自定义电影", MediaType: "movie", Enabled: true, Rules: []Rule{{ID: "disabled", Name: "失效", Directory: "失效", Enabled: false}}}}}
	got := policy.Match(metadata.Item{MediaType: "movie", Title: "Synthetic"})
	if got.Directory != "自定义电影" || got.GroupID != "g" || got.RuleID != "" {
		t.Fatalf("group fallback=%+v", got)
	}
}
func TestMatchDisabledGroupUsesTypeFallback(t *testing.T) {
	policy := Policy{Groups: []Group{{ID: "g", Name: "关闭组", Directory: "关闭组", MediaType: "movie", Enabled: false}}}
	got := policy.Match(metadata.Item{MediaType: "movie", Title: "Synthetic"})
	if got.Directory != "电影" || got.GroupID != "" {
		t.Fatalf("disabled group fallback=%+v", got)
	}
}
func TestMatchEnabledRulesWithNoMatchUsesTypeFallback(t *testing.T) {
	policy := Policy{Groups: []Group{{ID: "g", Name: "按地区", Directory: "按地区", MediaType: "movie", Enabled: true, Rules: []Rule{{ID: "cn", Name: "华语", Directory: "华语", Enabled: true, Countries: []string{"CN"}}}}}}
	got := policy.Match(metadata.Item{MediaType: "movie", Title: "Synthetic", Countries: []string{"US"}})
	if got.Directory != "电影" || got.GroupID != "" {
		t.Fatalf("unmatched rule fallback=%+v", got)
	}
}

func TestDecodeRecordMissingOrNullGroupsUseDefaultPolicy(t *testing.T) {
	want := Defaults()
	for _, raw := range []string{`{}`, `{"groups":null}`} {
		record, err := DecodeRecord([]byte(raw), 7)
		if err != nil {
			t.Fatal(err)
		}
		if len(record.Value.Groups) != len(want.Groups) {
			t.Fatalf("%s groups=%d want=%d", raw, len(record.Value.Groups), len(want.Groups))
		}
	}
}
func TestDecodeRecordOmittedNestedFieldsMatchOnlineLegacySemantics(t *testing.T) {
	raw := []byte(`{"groups":[{"id":"synthetic","name":"自定义","directory":"自定义","mediaType":"movie","enabled":true,"rules":[{"id":"rule","name":"规则","directory":"规则","enabled":true}]}]}`)
	record, err := DecodeRecord(raw, 7)
	if err != nil {
		t.Fatal(err)
	}
	defaults := Defaults()
	group := record.Value.Groups[0]
	if !reflect.DeepEqual(group.GenreIDs, defaults.Groups[0].GenreIDs) || len(group.Rules) != 1 || !reflect.DeepEqual(group.Rules[0].Countries, defaults.Groups[0].Rules[0].Countries) {
		t.Fatalf("legacy nested inheritance changed: %+v", group)
	}
}
