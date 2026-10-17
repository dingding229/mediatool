package nativeplayback

import (
	"context"
	"encoding/json"
	"fmt"
	"media-tool/internal/modules/embyapi/dto"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlaybackInfoGoldensAndURLInvariant(t *testing.T) {
	for _, name := range []string{"single", "multiple", "no-snapshot"} {
		t.Run(name, func(t *testing.T) {
			out := dto.PlaybackInfoResponse{PlaySessionId: "aaaaaaaa-0000-4000-8000-000000000031"}
			count := 1
			if name == "multiple" {
				count = 2
			}
			for i := 0; i < count; i++ {
				source := "aaaaaaaa-0000-4000-8000-000000000032"
				if i == 1 {
					source = "bbbbbbbb-0000-4000-8000-000000000032"
				}
				container := "mkv"
				if name == "no-snapshot" {
					container = ""
				}
				absolute, relative, err := StreamURLs("https://native.synthetic.invalid", "100031", source, container, "synthetic-user-token")
				if err != nil {
					t.Fatal(err)
				}
				media, err := dto.NewMediaSourceInfo(absolute)
				if err != nil {
					t.Fatal(err)
				}
				media.Id = source
				media.Name = fmt.Sprintf("版本 %d", i+1)
				media.Container = container
				media.DirectStreamUrl = relative
				if name != "no-snapshot" {
					ticks, size, bitrate := int64(600000000), int64(1000000), int64(123456)
					media.RunTimeTicks = &ticks
					media.Size = &size
					media.Bitrate = &bitrate
					index := 0
					media.MediaStreams = dto.Array[dto.MediaStream]{{Type: "Video", Index: &index, Codec: "h264", DisplayTitle: "1080p H264 SDR"}}
				}
				out.MediaSources = append(out.MediaSources, media)
			}
			raw, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, '\n')
			file := filepath.Join("testdata", name+".json")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err = os.MkdirAll("testdata", 0755); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(file, raw, 0644); err != nil {
					t.Fatal(err)
				}
			}
			expected, err := os.ReadFile(file)
			if err != nil || string(expected) != string(raw) {
				t.Fatal("golden mismatch", err)
			}
			var decoded dto.PlaybackInfoResponse
			if err = json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, origin := range []string{"https://115.com", "https://cdn.115cdn.net", "https://native.invalid/path", "https://user:pass@native.invalid", "javascript:bad"} {
		if _, _, err := StreamURLs(origin, "100031", "asset", "mkv", "token"); err == nil {
			t.Fatal("unsafe origin", origin)
		}
	}
	for _, bad := range []string{"../1", "1/2", "0", "1?admin"} {
		if _, _, err := StreamURLs("https://native.invalid", bad, "asset", "mkv", "token"); err == nil {
			t.Fatal("bad item", bad)
		}
	}
	a, b, err := StreamURLs("https://native.invalid", "31", "asset", "unknown", "token")
	if err != nil || !strings.Contains(a, "stream.mkv?") || !strings.HasPrefix(b, "/Videos/") {
		t.Fatal(a, b, err)
	}
	media, err := dto.NewMediaSourceInfo(a)
	if err != nil {
		t.Fatal(err)
	}
	media.DirectStreamUrl = b + "&evil=1"
	if _, err = json.Marshal(media); err == nil {
		t.Fatal("mismatched direct URL")
	}
}
func TestPlaybackExecutionDependencyBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "go", "list", "-deps", "./runtime").CombinedOutput()
	if err != nil {
		t.Fatal(err, string(raw))
	}
	for _, p := range strings.Fields(string(raw)) {
		if p == "os/exec" || strings.HasPrefix(p, "media-tool/internal/modules/mediaprobe/") && p != "media-tool/internal/modules/mediaprobe/probeinfo" {
			t.Fatal("playback imports execution package", p)
		}
	}
}

func TestActorDoesNotSerializeToken(t *testing.T) {
	raw, err := json.Marshal(Actor{Token: "private-user-token"})
	if err != nil || strings.Contains(string(raw), "private-user-token") {
		t.Fatal(string(raw), err)
	}
}
