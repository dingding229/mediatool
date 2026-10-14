package classification

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/metadata"
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
