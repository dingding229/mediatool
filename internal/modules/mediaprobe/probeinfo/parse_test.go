package probeinfo

import (
	"encoding/json"
	"errors"
	"flag"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update synthetic probe contract goldens")

func ptr[T any](v T) *T { return &v }
func golden(t *testing.T, name string, v any) {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	file := filepath.Join("testdata", name+".json")
	if *update {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, raw, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(raw) {
		t.Fatalf("golden %s differs\n%s", name, raw)
	}
}
func TestFixtures(t *testing.T) {
	for _, name := range []string{"full", "hdr", "minimal"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", name+".input.json"))
			if err != nil {
				t.Fatal(err)
			}
			got, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			golden(t, name+".golden", got)
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var back Result
			if err := json.Unmarshal(encoded, &back); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, back) {
				t.Fatal("typed snapshot round trip")
			}
			if strings.Contains(string(encoded), "DO_NOT_STORE") || strings.Contains(string(encoded), "filename") {
				t.Fatal("private data persisted")
			}
		})
	}
}
func TestDisplayTitle(t *testing.T) {
	for _, tt := range []struct {
		s    Stream
		want string
	}{
		{Stream{Type: Video, Codec: "h264", Height: ptr(1080), VideoRange: SDR}, "1080p H264 SDR"},
		{Stream{Type: Audio, Codec: "aac", Language: "zho", Channels: ptr(6), IsDefault: true}, "Chinese AAC 5.1 Default"},
		{Stream{Type: Subtitle, Codec: "srt", Language: "eng", IsForced: true}, "English SRT Forced"},
		{Stream{Type: Audio, Codec: "flac", Language: "eng", Channels: ptr(8)}, "English FLAC 7.1"},
		{Stream{Type: Audio, Channels: ptr(3), IsHearingImpaired: true}, "3ch Hearing Impaired"},
		{Stream{Type: Audio, Channels: ptr(1)}, "Mono"}, {Stream{Type: Audio, Channels: ptr(2)}, "Stereo"},
		{Stream{Type: Video, Codec: "hevc", VideoRange: HDR, VideoRangeType: DOVI}, "HEVC Dolby Vision"},
	} {
		if got := DisplayTitle(tt.s); got != tt.want {
			t.Fatalf("%q != %q", got, tt.want)
		}
	}
}
func TestContainerLanguageRatesAndDepth(t *testing.T) {
	for _, tt := range []struct{ format, file, want string }{
		{"matroska,webm", "movie.mkv", "mkv"}, {"mov,mp4,m4a,3gp,3g2,mj2", "x.mp4", "mp4"},
		{"mpegts", "https://synthetic.invalid/a.m2ts?DO_NOT_STORE=1", "m2ts"}, {"mpegts", "a.ts", "ts"},
		{"mpegts", "%", "ts"}, {"avi", "x.avi", "avi"}, {"webm", "x.webm", "webm"}, {"unknown", "x", ""},
	} {
		if got := container(tt.format, tt.file); got != tt.want {
			t.Fatal(tt, got)
		}
	}
	for input, want := range map[string]string{"zh-CN": "zho", "chi": "zho", "zho": "zho", "en": "eng", "fre": "fra", "ger": "deu", "und": "", "": "", "$$$$": "", strings.Repeat("a", 33): ""} {
		if got := normalizeLanguage(input); got != want {
			t.Fatal(input, got, want)
		}
	}
	for _, bad := range []scalar{"0/0", "30/0", "NaN", "Inf", "0", "-1", "2000", "1/2/3", "1/no", "x"} {
		if rate(bad) != nil {
			t.Fatal("invalid rate", bad)
		}
	}
	if r := rate("30000/1001"); r == nil || math.Abs(float64(*r)-29.97003) > .001 {
		t.Fatal(r)
	}
	for pixel, want := range map[string]int{"yuv420p": 8, "yuv420p10le": 10, "p010le": 10, "gbrp12le": 12, "yuv444p16le": 16, "rgb24": 8} {
		if got := bitDepth(rawStream{Pixel: pixel}); got == nil || *got != want {
			t.Fatal(pixel, got)
		}
	}
	if bitDepth(rawStream{Pixel: "unknown"}) != nil || *bitDepth(rawStream{Bits: "24"}) != 24 {
		t.Fatal("depth bounds")
	}
}
func TestSubtitleWhitelistAndRange(t *testing.T) {
	for codec, want := range subtitleCodecs {
		s, ok := parseStream(rawStream{Index: ptr(7), Type: "subtitle", Codec: codec})
		if !ok || s.Index != 7 || s.Codec != want {
			t.Fatal(codec, s)
		}
		image := codec == "hdmv_pgs_subtitle" || codec == "dvd_subtitle" || codec == "dvb_subtitle"
		if s.IsTextSubtitleStream == image {
			t.Fatal("subtitle type")
		}
	}
	if _, ok := parseStream(rawStream{Index: ptr(0), Type: "subtitle", Codec: "unknown"}); ok {
		t.Fatal("unknown subtitle")
	}
	for _, tt := range []struct{ transfer, primaries, want string }{
		{"", "", SDR}, {"smpte2084", "bt2020", HDR10}, {"smpte2084", "bt709", HDR10}, {"smpte2084", "", HDR10}, {"arib-std-b67", "bt2020", HLG},
	} {
		s, _ := parseStream(rawStream{Index: ptr(0), Type: "video", Transfer: tt.transfer, Primaries: tt.primaries})
		if s.VideoRangeType != tt.want || (s.VideoRange == SDR) != (tt.want == SDR) {
			t.Fatal(s)
		}
	}
}
func TestMalformedAndBounds(t *testing.T) {
	for _, raw := range []string{
		``, `null`, `[]`, `{`, `{"format":[]}`, `{"format":{"duration":true}}`,
		`{"format":{"duration":"-1"}}`, `{"format":{"duration":"NaN"}}`, `{"format":{"duration":"Inf"}}`, `{"format":{"duration":"1e100"}}`,
		`{"streams":[{"codec_type":"video"}]}`, `{"streams":[{"codec_type":"audio","index":-1}]}`,
		`{"streams":[{"codec_type":"audio","index":65536}]}`, `{"streams":[{"codec_type":"audio","index":0},{"codec_type":"video","index":0}]}`,
		`{"format":{"duration":"` + strings.Repeat("1", 67) + `"}}`, strings.Repeat("x", MaxInputBytes+1),
	} {
		if _, err := Parse([]byte(raw)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid accepted: %.80s", raw)
		}
	}
	streams := make([]map[string]any, 65)
	for i := range streams {
		streams[i] = map[string]any{"index": i, "codec_type": "audio", "codec_name": "aac"}
	}
	raw, err := json.Marshal(map[string]any{"streams": streams})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(raw); err == nil {
		t.Fatal("65 streams")
	}
	raw, err = json.Marshal(map[string]any{"streams": streams[:64]})
	if err != nil {
		t.Fatal(err)
	}
	if r, err := Parse(raw); err != nil || len(r.Streams) != 64 {
		t.Fatal("64 boundary", err)
	}
	raw = []byte(`{"streams":[{"index":0,"codec_type":"audio","tags":{"title":"` + strings.Repeat("a", MaxStreamBytes) + `"}}]}`)
	if _, err := Parse(raw); err == nil {
		t.Fatal("single stream bytes bound")
	}
	raw = []byte(`{"streams":[{"index":0,"codec_type":"video","width":-1,"height":999999,"avg_frame_rate":"0/0","bit_rate":"N/A","bits_per_raw_sample":"100","display_aspect_ratio":"0:0"}]}`)
	r, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Streams[0]
	if s.Width != nil || s.Height != nil || s.AverageFrameRate != nil || s.BitRate != nil || s.BitDepth != nil || s.AspectRatio != "" {
		t.Fatal("invalid optional numeric not omitted")
	}
	for _, title := range []string{strings.Repeat("x", 257), "https://synthetic.invalid/?sig=DO_NOT_STORE", "/mnt/private", `C:\private`, "private\nsecret", "pickCode:secret", "abc\u200bdef", "%2Fprivate"} {
		if clean(title, 256) != "" {
			t.Fatal("unsafe title accepted")
		}
	}
}
func TestPersistenceGuardAndLegacy(t *testing.T) {
	var old Result
	if err := json.Unmarshal([]byte(`{"durationTicks":42,"filename":"DO_NOT_STORE"}`), &old); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(old)
	if err != nil || string(raw) != `{"durationTicks":42}` {
		t.Fatal("legacy changed", err)
	}
	for _, bad := range []Result{
		{SchemaVersion: 2}, {DurationTicks: -1}, {Streams: []Stream{{Type: "data"}}},
		{Streams: []Stream{{Type: Audio, Title: "https://synthetic.invalid/private"}}},
		{Streams: []Stream{{Type: Video, VideoRange: "unknown"}}}, {Streams: []Stream{{Type: Video, VideoRange: HDR10}}},
		{Streams: []Stream{{Type: Video, Rotation: ptr(361)}}},
		{Streams: []Stream{{Type: Video, Width: ptr(32769)}}},
		{Streams: []Stream{{Type: Audio, Channels: ptr(65)}}},
		{Streams: []Stream{{Type: Audio, SampleRate: ptr(768001)}}},
		{Streams: []Stream{{Type: Video, Level: ptr(1001)}}},
		{Streams: []Stream{{Type: Audio, Codec: strings.Repeat("x", 33)}}},
		{Streams: []Stream{{Type: Audio, AverageFrameRate: ptr(float32(math.NaN()))}}},
	} {
		if _, err := json.Marshal(bad); err == nil {
			t.Fatal("unsafe typed snapshot accepted")
		}
	}
}
func FuzzParse(f *testing.F) {
	for _, raw := range []string{`{}`, `{"format":{"duration":"1.5"}}`, `{"streams":[{"index":0,"codec_type":"video","codec_name":"h264"}]}`} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if r, err := Parse(raw); err == nil {
			if _, err := json.Marshal(r); err != nil {
				t.Fatal("parser produced invalid typed result")
			}
		}
	})
}
