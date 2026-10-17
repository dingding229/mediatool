package probeinfo

import (
	"bytes"
	"encoding/json"
	"math"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/language"
)

type scalar string

func (s *scalar) UnmarshalJSON(raw []byte) error {
	if len(raw) > 66 {
		return ErrInvalid
	}
	if bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if len(raw) > 0 && raw[0] == '"' {
		return json.Unmarshal(raw, (*string)(s))
	}
	var n json.Number
	if json.Unmarshal(raw, &n) != nil {
		return ErrInvalid
	}
	*s = scalar(n)
	return nil
}
func integer[T ~int | ~int64](s scalar, lo, hi int64) *T {
	n, err := strconv.ParseInt(string(s), 10, 64)
	if err != nil || n < lo || n > hi {
		return nil
	}
	v := T(n)
	return &v
}
func rate(s scalar) *float32 {
	parts := strings.Split(string(s), "/")
	if len(parts) > 2 {
		return nil
	}
	n, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return nil
	}
	if len(parts) == 2 {
		d, e := strconv.ParseFloat(parts[1], 64)
		if e != nil || d <= 0 {
			return nil
		}
		n /= d
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 || n > 1000 {
		return nil
	}
	v := float32(n)
	return &v
}

type rawStream struct {
	Index      *int   `json:"index"`
	Type       string `json:"codec_type"`
	Codec      string `json:"codec_name"`
	Profile    string `json:"profile"`
	Level      scalar `json:"level"`
	Width      scalar `json:"width"`
	Height     scalar `json:"height"`
	BitRate    scalar `json:"bit_rate"`
	Average    scalar `json:"avg_frame_rate"`
	Real       scalar `json:"r_frame_rate"`
	Pixel      string `json:"pix_fmt"`
	Depth      scalar `json:"bits_per_raw_sample"`
	Bits       scalar `json:"bits_per_sample"`
	Aspect     string `json:"display_aspect_ratio"`
	Transfer   string `json:"color_transfer"`
	Primaries  string `json:"color_primaries"`
	FieldOrder string `json:"field_order"`
	Channels   scalar `json:"channels"`
	Layout     string `json:"channel_layout"`
	SampleRate scalar `json:"sample_rate"`
	Tags       struct {
		Language, Title string
		Rotate          scalar
	} `json:"tags"`
	Disposition struct {
		Default, Forced int
		Hearing         int `json:"hearing_impaired"`
		Attached        int `json:"attached_pic"`
	} `json:"disposition"`
	SideData []struct {
		Type          string `json:"side_data_type"`
		Rotation      scalar `json:"rotation"`
		DVProfile     scalar `json:"dv_profile"`
		Compatibility scalar `json:"dv_bl_signal_compatibility_id"`
	} `json:"side_data_list"`
}

func Parse(raw []byte) (Result, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > MaxInputBytes || raw[0] != '{' {
		return Result{}, ErrInvalid
	}
	var in struct {
		Format struct {
			Name     string `json:"format_name"`
			Filename string `json:"filename"`
			Duration scalar `json:"duration"`
			BitRate  scalar `json:"bit_rate"`
			Size     scalar `json:"size"`
		} `json:"format"`
		Streams []json.RawMessage `json:"streams"`
	}
	if json.Unmarshal(raw, &in) != nil || len(in.Streams) > MaxStreams {
		return Result{}, ErrInvalid
	}
	r := Result{SchemaVersion: SchemaVersion, Container: container(in.Format.Name, in.Format.Filename), BitRate: integer[int64](in.Format.BitRate, 0, math.MaxInt64), Size: integer[int64](in.Format.Size, 0, math.MaxInt64)}
	d := string(in.Format.Duration)
	if d != "" && d != "N/A" {
		n, err := strconv.ParseFloat(d, 64)
		ticks := math.Round(n * 1e7)
		if err != nil || n < 0 || math.IsNaN(ticks) || math.IsInf(ticks, 0) || ticks >= float64(math.MaxInt64) {
			return Result{}, ErrInvalid
		}
		r.DurationTicks = int64(ticks)
	}
	seen := map[int]bool{}
	for _, streamJSON := range in.Streams {
		if len(streamJSON) > MaxStreamBytes {
			return Result{}, ErrInvalid
		}
		var s rawStream
		if json.Unmarshal(streamJSON, &s) != nil {
			return Result{}, ErrInvalid
		}
		if s.Type != "video" && s.Type != "audio" && s.Type != "subtitle" || s.Disposition.Attached != 0 {
			continue
		}
		if s.Index == nil || *s.Index < 0 || *s.Index > 65535 || seen[*s.Index] {
			return Result{}, ErrInvalid
		}
		seen[*s.Index] = true
		out, ok := parseStream(s)
		if ok {
			r.Streams = append(r.Streams, out)
		}
	}
	sort.Slice(r.Streams, func(i, j int) bool { return r.Streams[i].Index < r.Streams[j].Index })
	return r, r.Validate()
}

func parseStream(s rawStream) (Stream, bool) {
	out := Stream{Index: *s.Index, Codec: clean(s.Codec, 32), Profile: clean(s.Profile, 64), BitRate: integer[int64](s.BitRate, 0, math.MaxInt64), Language: normalizeLanguage(s.Tags.Language), Title: clean(s.Tags.Title, 256), IsDefault: s.Disposition.Default != 0, IsForced: s.Disposition.Forced != 0}
	switch s.Type {
	case "video":
		out.Type = Video
		out.Level = integer[int](s.Level, 0, 1000)
		out.Width = integer[int](s.Width, 1, 32768)
		out.Height = integer[int](s.Height, 1, 32768)
		out.AverageFrameRate = rate(s.Average)
		out.RealFrameRate = rate(s.Real)
		out.PixelFormat = clean(s.Pixel, 64)
		out.BitDepth = bitDepth(s)
		if parts := strings.Split(s.Aspect, ":"); len(parts) == 2 && integer[int](scalar(parts[0]), 1, 100000) != nil && integer[int](scalar(parts[1]), 1, 100000) != nil {
			out.AspectRatio = s.Aspect
		}
		out.VideoRangeType = SDR
		if s.Transfer == "smpte2084" {
			out.VideoRangeType = HDR10
		}
		if s.Transfer == "arib-std-b67" {
			out.VideoRangeType = HLG
		}
		out.Rotation = integer[int](s.Tags.Rotate, -360, 360)
		for _, side := range s.SideData {
			if strings.EqualFold(side.Type, "DOVI configuration record") && integer[int](side.DVProfile, 1, 10) != nil {
				out.VideoRangeType = DOVI
				switch side.Compatibility {
				case "1":
					out.VideoRangeType = DOVIWithHDR10
				case "4":
					out.VideoRangeType = DOVIWithHLG
				case "2":
					out.VideoRangeType = DOVIWithSDR
				}
			}
			if side.Type == "Display Matrix" {
				out.Rotation = integer[int](side.Rotation, -360, 360)
			}
		}
		out.VideoRange = SDR
		if out.VideoRangeType != SDR {
			out.VideoRange = HDR
		}
		out.IsInterlaced = s.FieldOrder == "tt" || s.FieldOrder == "bb" || s.FieldOrder == "tb" || s.FieldOrder == "bt"
	case "audio":
		out.Type = Audio
		out.Channels = integer[int](s.Channels, 1, 64)
		out.ChannelLayout = clean(s.Layout, 64)
		out.SampleRate = integer[int](s.SampleRate, 1, 768000)
	case "subtitle":
		out.Type = Subtitle
		codec, ok := subtitleCodecs[s.Codec]
		if !ok {
			return Stream{}, false
		}
		out.Codec = codec
		out.IsTextSubtitleStream = codec != "pgssub" && codec != "dvdsub" && codec != "dvb_subtitle"
		out.IsHearingImpaired = s.Disposition.Hearing != 0
	}
	out.DisplayTitle = DisplayTitle(out)
	return out, true
}

var subtitleCodecs = map[string]string{"subrip": "srt", "srt": "srt", "ass": "ass", "ssa": "ssa", "mov_text": "mov_text", "webvtt": "webvtt", "text": "text", "hdmv_pgs_subtitle": "pgssub", "dvd_subtitle": "dvdsub", "dvb_subtitle": "dvb_subtitle"}

func normalizeLanguage(s string) string {
	if len(s) > 32 || s == "" {
		return ""
	}
	tag, err := language.Parse(s)
	if err != nil {
		return ""
	}
	base, _, _ := tag.Raw()
	if base.String() == "und" {
		return ""
	}
	return base.ISO3()
}
func bitDepth(s rawStream) *int {
	if v := integer[int](s.Depth, 1, 64); v != nil {
		return v
	}
	if v := integer[int](s.Bits, 1, 64); v != nil {
		return v
	}
	p := s.Pixel
	for _, depth := range []string{"9", "10", "12", "14", "16"} {
		if strings.Contains(p, "p"+depth) || strings.Contains(p, "p0"+depth) {
			v, err := strconv.Atoi(depth)
			if err == nil {
				return &v
			}
		}
	}
	if strings.HasPrefix(p, "yuv") || strings.HasPrefix(p, "yuva") || p == "nv12" || p == "nv21" || p == "rgb24" || p == "bgr24" {
		v := 8
		return &v
	}
	return nil
}
func container(name, filename string) string {
	for _, name := range strings.Split(name, ",") {
		switch name {
		case "matroska":
			return "mkv"
		case "webm":
			return "webm"
		case "mov", "mp4", "m4a", "3gp", "3g2", "mj2":
			return "mp4"
		case "mpegts":
			u, err := url.Parse(filename)
			if err == nil && strings.EqualFold(path.Ext(u.Path), ".m2ts") {
				return "m2ts"
			}
			return "ts"
		case "avi", "mpeg", "flv", "asf", "ogg":
			return name
		}
	}
	return ""
}
