package runtime

import (
	"encoding/json"
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
