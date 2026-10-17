package nativelibrary

import (
	"encoding/json"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediaprobe/probeinfo"
	"strings"
	"testing"
)

func TestEmbeddedSubtitleGoldenNoDeliveryURL(t *testing.T) {
	result := probeinfo.Result{SchemaVersion: 1, Streams: []probeinfo.Stream{{Index: 2, Type: probeinfo.Audio, Codec: "aac"}, {Index: 7, Type: probeinfo.Subtitle, Codec: "srt", Language: "zho", IsTextSubtitleStream: true}}}
	var out dto.MediaSourceInfo
	applyProbeSource(&out, &result)
	s := out.MediaStreams[1]
	b, e := json.Marshal(s)
	if e != nil {
		t.Fatal(e)
	}
	const want = `{"DeliveryMethod":"Embed","IsTextSubtitleStream":true,"Type":"Subtitle","Codec":"srt","Language":"zho","DisplayTitle":"Chinese SRT","Index":7,"IsExternal":false}`
	if string(b) != want || s.Index == nil || *s.Index != 7 || strings.Contains(string(b), "DeliveryUrl") {
		t.Fatal(string(b))
	}
}
