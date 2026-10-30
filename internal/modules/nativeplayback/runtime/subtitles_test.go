package runtime

import (
	"encoding/json"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/nativeplayback"
	"media-tool/internal/modules/subtitles"
	"strings"
	"testing"
)

func TestSubtitleAssociationAndScopedDelivery(t *testing.T) {
	for _, c := range []struct {
		file string
		want bool
	}{{"Episode.srt", true}, {"Episode.zh.chs.forced.ass", true}, {"Episode.eng.default.srt", true}, {"Episode10.srt", false}, {"Episode.srt.exe", false}, {"Other.srt", false}, {"other/Episode.srt", false}} {
		if got := subtitleMatches("/show/Episode.mkv", "/show/"+c.file); got != c.want {
			t.Fatal(c.file, got)
		}
	}
	v, e := subtitleStream(subtitle{id: "00000000-0000-4000-8000-000000000001", name: "Episode.zh.forced.srt"}, 3, "https://native.invalid", "12", "00000000-0000-4000-8000-000000000002", "user-token")
	if e != nil || !v.IsExternal || v.DeliveryMethod != "External" || !v.IsForced || v.Language != "chi" {
		t.Fatal(v, e)
	}
	raw, e := json.Marshal(v)
	if e != nil || !strings.Contains(string(raw), "DeliveryUrl") {
		t.Fatal(string(raw), e)
	}
}

func TestManagedSubtitleMergeStableIndicesAndEmbeddedOverlay(t *testing.T) {
	i := 2
	v := source{MediaSourceInfo: dto.MediaSourceInfo{Id: "00000000-0000-4000-8000-000000000002", MediaStreams: []dto.MediaStream{{Type: "Subtitle", Index: &i, DeliveryMethod: "Embed", Language: "eng"}}}}
	a := nativeplayback.Actor{Origin: "https://native.invalid", Token: "private"}
	subs := []subtitle{
		{entry: &subtitles.Entry{Source: "embedded", Index: 2, Overridden: true, Metadata: subtitles.Metadata{Language: "jpn", Title: "日本語", Forced: true}}},
		{id: "00000000-0000-4000-8000-000000000003", name: "f.chs.srt", entry: &subtitles.Entry{Source: "uploaded", Index: 65540, Extension: "srt", Metadata: subtitles.Metadata{Language: "chs", Title: "简体字幕", Default: true}}},
	}
	if e := addSubtitles(a, "12", &v, subs); e != nil {
		t.Fatal(e)
	}
	embedded, external := v.MediaStreams[0], v.MediaStreams[1]
	if embedded.IsExternal || embedded.DeliveryMethod != "Embed" || embedded.DeliveryUrl != "" || embedded.Language != "jpn" || !embedded.IsForced {
		t.Fatal(embedded)
	}
	if !external.IsExternal || external.DeliveryMethod != "External" || *external.Index != 65540 || external.DisplayTitle != "简体中文 · 简体字幕" || !external.IsDefault || v.DefaultSubtitleStreamIndex == nil || *v.DefaultSubtitleStreamIndex != 65540 {
		t.Fatal(v)
	}
	if !strings.Contains(external.DeliveryUrl, "/65540/Stream.srt") || !strings.Contains(external.DeliveryUrl, "api_key=private") {
		t.Fatal(external.DeliveryUrl)
	}
}

func TestI40SubtitleLanguagesAreVisibleToEmbyPlayers(t *testing.T) {
	for _, c := range []struct {
		language string
		tags     []string
		title    string
	}{
		{"chi", []string{"zh-Hans"}, "简体中文"}, {"chi", []string{"zh-Hant"}, "繁体中文"}, {"chi", []string{"zh-Hant", "en"}, "中英双语"}, {"eng", []string{"en"}, "英语"},
	} {
		for _, kind := range []string{"embedded", "uploaded"} {
			t.Run(c.title+"/"+kind, func(t *testing.T) {
				index := 5
				v := source{MediaSourceInfo: dto.MediaSourceInfo{Id: "00000000-0000-4000-8000-000000000002", MediaStreams: []dto.MediaStream{{Type: "Subtitle", Index: &index, DeliveryMethod: "Embed"}}}}
				entry := subtitles.Entry{Source: kind, Index: 5, Extension: "srt", Metadata: subtitles.Metadata{Language: c.language, LanguageTags: c.tags, Title: "sample", Revision: 1}}
				if kind == "uploaded" {
					entry.Index = 65536
					v.MediaStreams = nil
				}
				err := addSubtitles(nativeplayback.Actor{Origin: "https://native.invalid", Token: "synthetic"}, "12", &v, []subtitle{{id: "00000000-0000-4000-8000-000000000003", name: "sample.srt", entry: &entry}})
				if err != nil || len(v.MediaStreams) != 1 {
					t.Fatal(v, err)
				}
				stream := v.MediaStreams[0]
				if stream.Language != c.language || stream.DisplayTitle != c.title+" · sample" {
					t.Fatal(stream)
				}
				if kind == "embedded" && (stream.DeliveryUrl != "" || stream.DeliveryMethod != "Embed") {
					t.Fatal("embedded delivery changed", stream)
				}
				if kind == "uploaded" && (!strings.Contains(stream.DeliveryUrl, "/65536/Stream.srt") || stream.DeliveryMethod != "External") {
					t.Fatal("external delivery changed", stream)
				}
			})
		}
	}
}
