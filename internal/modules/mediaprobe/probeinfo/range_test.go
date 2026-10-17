package probeinfo

import (
	"fmt"
	"testing"
)

func TestDOVICompatibilityAndRangePairs(t *testing.T) {
	for _, tt := range []struct{ field, want string }{
		{"", DOVI}, {`,"dv_bl_signal_compatibility_id":0`, DOVI},
		{`,"dv_bl_signal_compatibility_id":1`, DOVIWithHDR10},
		{`,"dv_bl_signal_compatibility_id":4`, DOVIWithHLG},
		{`,"dv_bl_signal_compatibility_id":2`, DOVIWithSDR},
		{`,"dv_bl_signal_compatibility_id":9`, DOVI},
	} {
		raw := fmt.Sprintf(`{"streams":[{"index":0,"codec_type":"video","codec_name":"hevc","color_transfer":"arib-std-b67","side_data_list":[{"side_data_type":"DOVI configuration record","dv_profile":8%s}]}]}`, tt.field)
		r, err := Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		s := r.Streams[0]
		if s.VideoRange != HDR || s.VideoRangeType != tt.want || s.DisplayTitle != "HEVC Dolby Vision" {
			t.Fatal(tt, s)
		}
	}
	for _, pair := range [][2]string{{SDR, SDR}, {HDR, HDR10}, {HDR, HLG}, {HDR, DOVI}, {HDR, DOVIWithHDR10}, {HDR, DOVIWithHLG}, {HDR, DOVIWithSDR}} {
		if err := (Result{SchemaVersion: 1, Streams: []Stream{{Type: Video, VideoRange: pair[0], VideoRangeType: pair[1]}}}).Validate(); err != nil {
			t.Fatal(pair, err)
		}
	}
	for _, pair := range [][2]string{{HDR10, HDR10}, {"DolbyVision", "DolbyVision"}, {SDR, HDR10}, {HDR, SDR}, {HDR, ""}, {HDR, "unknown"}} {
		if err := (Result{SchemaVersion: 1, Streams: []Stream{{Type: Video, VideoRange: pair[0], VideoRangeType: pair[1]}}}).Validate(); err == nil {
			t.Fatal("invalid range pair", pair)
		}
	}
}
