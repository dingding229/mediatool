package dto_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"media-tool/internal/modules/embyapi/dto"
)

func TestProbeDTOExtensions(t *testing.T) {
	item, err := dto.NewBaseItemDto("100101", "Synthetic Probe Movie", dto.Movie)
	if err != nil {
		t.Fatal(err)
	}
	item.Container = "mkv"
	item.Size = ptr(int64(0))
	item.Bitrate = ptr(int64(0))
	item.Width = ptr(1920)
	item.Height = ptr(1080)
	stream := dto.MediaStream{Type: "Video", Codec: "hevc", Index: ptr(0), Profile: "Main 10", Level: ptr(0), RealFrameRate: ptr(float32(24)), PixelFormat: "yuv420p10le", AspectRatio: "16:9", VideoRange: "HDR", VideoRangeType: "HDR10", IsInterlaced: true, Rotation: ptr(0), ChannelLayout: "5.1", Title: "Synthetic Track", IsTextSubtitleStream: true, IsHearingImpaired: true}
	source, err := dto.NewMediaSourceInfo("")
	if err != nil {
		t.Fatal(err)
	}
	source.Width = ptr(1920)
	source.Height = ptr(1080)
	source.MediaStreams = dto.Array[dto.MediaStream]{stream}
	item.MediaSources = dto.Array[dto.MediaSourceInfo]{source}
	raw, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	file := filepath.Join("testdata", "emby", "probe_extensions_full.json")
	if *update {
		if err := os.WriteFile(file, raw, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(raw) {
		t.Fatalf("probe extension golden differs\n%s", raw)
	}
	inspect(t, raw)
	var back dto.BaseItemDto
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	again, err := json.MarshalIndent(back, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(again)+"\n" != string(raw) {
		t.Fatal("extension roundtrip lost a value")
	}
	for _, field := range []string{`"Index": 0`, `"Level": 0`, `"Rotation": 0`, `"Size": 0`, `"Bitrate": 0`} {
		if !strings.Contains(string(raw), field) {
			t.Fatal("valid zero omitted", field)
		}
	}
	// Existing no-probe wire shape remains unchanged, including [] arrays.
	minimal, err := dto.NewBaseItemDto("100101", "Synthetic", dto.Movie)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := json.Marshal(minimal)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(plain, &keys); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"Container", "Size", "Bitrate", "Width", "Height"} {
		if _, ok := keys[field]; ok {
			t.Fatal("probe field emitted without a probe", field)
		}
	}
	if string(keys["MediaStreams"]) != "[]" {
		t.Fatal("no probe must not emit null streams")
	}
}
