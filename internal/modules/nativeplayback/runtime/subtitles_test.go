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
	if e != nil || !v.IsExternal || v.DeliveryMethod != "External" || !v.IsForced || v.Language != "zh" {
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
	if !external.IsExternal || external.DeliveryMethod != "External" || *external.Index != 65540 || external.DisplayTitle != "简体字幕" || !external.IsDefault || v.DefaultSubtitleStreamIndex == nil || *v.DefaultSubtitleStreamIndex != 65540 {
		t.Fatal(v)
	}
	if !strings.Contains(external.DeliveryUrl, "/65540/Stream.srt") || !strings.Contains(external.DeliveryUrl, "api_key=private") {
		t.Fatal(external.DeliveryUrl)
	}
}
