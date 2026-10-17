package nativelibrary

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediaprobe/probeinfo"
)

var updateProbeGolden = flag.Bool("update-native-probe", false, "update synthetic probed movie/episode goldens")

func TestProbedProjectionGoldens(t *testing.T) {
	p, err := probeinfo.Parse([]byte(`{"format":{"format_name":"matroska","duration":"61.5","size":"123456","bit_rate":"800000"},"streams":[{"index":0,"codec_type":"video","codec_name":"hevc","width":1920,"height":1080,"pix_fmt":"yuv420p10le","color_transfer":"smpte2084","side_data_list":[{"side_data_type":"DOVI configuration record","dv_profile":8,"dv_bl_signal_compatibility_id":1}]},{"index":2,"codec_type":"audio","codec_name":"aac","channels":6,"tags":{"language":"chi"},"disposition":{"default":1}},{"index":4,"codec_type":"subtitle","codec_name":"subrip","tags":{"language":"eng"},"disposition":{"forced":1}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	legacy := probeinfo.Result{DurationTicks: 770000000}
	fallback := int64(14400000000)
	sources := []MovieSource{{ID: "synthetic-v1", Container: "mkv", Probe: &p}, {ID: "synthetic-v2", Container: "mp4", Probe: &legacy}, {ID: "synthetic-v3", Container: "mp4", RunTimeTicks: &fallback}}
	created := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	movie, err := MovieDTO(Movie{ID: "100101", ParentID: "100001", Name: "Synthetic Probed Film", TMDBID: 900101, CreatedAt: created, Sources: sources, Details: &TMDBDetails{RuntimeMinutes: 90}})
	if err != nil {
		t.Fatal(err)
	}
	episode, err := EpisodeDTO(EpisodeProjection{ID: "100203", ParentID: "100202", SeriesID: "100201", SeasonID: "100202", SeriesName: "Synthetic Series", SeasonName: "特别篇", Name: "Synthetic Probed Episode", TMDBID: 90201, Number: 1, Season: 0, CreatedAt: created, RuntimeTicks: &fallback, Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	for name, item := range map[string]dto.BaseItemDto{"movie-probed": movie, "episode-probed": episode} {
		t.Run(name, func(t *testing.T) {
			if item.RunTimeTicks == nil || *item.RunTimeTicks != 615000000 || item.Container != "mkv" || len(item.MediaStreams) != 3 || item.Size == nil || *item.Size != 123456 {
				t.Fatal("primary technical fields")
			}
			video := item.MediaStreams[0]
			if video.Index == nil || *video.Index != 0 || video.VideoRange != "HDR" || video.VideoRangeType != "DOVIWithHDR10" || video.DisplayTitle != "1080p HEVC Dolby Vision" {
				t.Fatal(video)
			}
			for i, s := range item.MediaSources {
				if s.Name != "版本 "+string(rune('1'+i)) || s.Path != "" || s.SupportsTranscoding() {
					t.Fatal("version or URL capabilities changed")
				}
			}
			if len(item.MediaSources[1].MediaStreams) != 0 || *item.MediaSources[1].RunTimeTicks != 770000000 || len(item.MediaSources[2].MediaStreams) != 0 || *item.MediaSources[2].RunTimeTicks != fallback {
				t.Fatal("per-version runtime fallback")
			}
			raw, err := json.MarshalIndent(item, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, '\n')
			file := filepath.Join("testdata", "projection", name+".json")
			if *updateProbeGolden {
				if err := os.WriteFile(file, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != string(want) {
				t.Fatalf("probe golden mismatch %s", raw)
			}
			var back dto.BaseItemDto
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatal(err)
			}
			again, err := json.MarshalIndent(back, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if string(again)+"\n" != string(raw) || strings.Contains(string(raw), "null") {
				t.Fatal("probe golden roundtrip/null")
			}
		})
	}
}
func TestInvalidProbeDoesNotChangeFallback(t *testing.T) {
	bad := probeinfo.Result{DurationTicks: -1}
	source := MovieSource{ID: "synthetic", Container: "mp4", Probe: &bad}
	item, err := MovieDTO(Movie{ID: "100101", ParentID: "100001", Name: "Synthetic", TMDBID: 900101, CreatedAt: time.Now(), Sources: []MovieSource{source}})
	if err != nil {
		t.Fatal(err)
	}
	if item.Container != "" || len(item.MediaStreams) != 0 || len(item.MediaSources[0].MediaStreams) != 0 {
		t.Fatal("invalid probe should be absent")
	}
}
