package probeinfo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/mediaprobe/ffx"
)

func ffmpeg(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-loglevel", "error", "-y", "-nostdin"}, args...)...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("synthetic ffmpeg: %v %.2000s", err, raw)
	}
}
func requireEncoder(t *testing.T, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-encoders").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), " "+name+" ") {
		if os.Getenv("MEDIA_TOOL_REQUIRE_FFMPEG") == "1" {
			t.Fatal("ffmpeg encoder unavailable: " + name)
		}
		t.Skip("ffmpeg encoder unavailable: " + name)
	}
}
func probeFile(t *testing.T, file string) Result {
	t.Helper()
	runner, err := ffx.New(ffx.Config{AllowLocalFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := runner.Probe(t.Context(), file, ffx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.DurationTicks < 29_000_000 || r.DurationTicks > 32_000_000 {
		t.Fatal("3s duration", r.DurationTicks)
	}
	if r.Size == nil || *r.Size <= 0 {
		t.Fatal("file size missing")
	}
	return r
}
func mediaGolden(t *testing.T, name string, r Result) {
	t.Helper()
	// Encoders/muxers differ in packet sizes, duration rounding, and advertised
	// bitrates across versions. Check those above, lock semantic track data here.
	r.DurationTicks = 0
	r.Size = nil
	r.BitRate = nil
	for i := range r.Streams {
		r.Streams[i].BitRate = nil
		r.Streams[i].Level = nil
		r.Streams[i].Profile = ""
	}
	golden(t, name, r)
}
func TestGeneratedMediaGoldens(t *testing.T) {
	for _, binary := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(binary); err != nil {
			if os.Getenv("MEDIA_TOOL_REQUIRE_FFMPEG") == "1" {
				t.Fatal(binary + " unavailable")
			}
			t.Skip(binary + " unavailable")
		}
	}
	requireEncoder(t, "libx264")
	requireEncoder(t, "aac")
	dir := t.TempDir()
	subtitle := filepath.Join(dir, "captions.srt")
	font := filepath.Join(dir, "synthetic.ttf")
	for file, body := range map[string]string{subtitle: "1\n00:00:00,000 --> 00:00:02,000\nSynthetic captions\n", font: "Synthetic font attachment bytes"} {
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	mkv := filepath.Join(dir, "synthetic.mkv")
	ffmpeg(t, "-f", "lavfi", "-i", "color=c=blue:s=160x90:r=24:d=3", "-f", "lavfi", "-i", "sine=frequency=440:duration=3", "-i", subtitle,
		"-map", "0:v", "-map", "1:a", "-map", "1:a", "-map", "2:s", "-map", "2:s", "-t", "3", "-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", "-c:a", "aac", "-c:s", "srt",
		"-metadata:s:a:0", "language=zho", "-metadata:s:a:1", "language=eng", "-metadata:s:s:0", "language=eng", "-metadata:s:s:1", "language=zho",
		"-disposition:a:0", "default", "-disposition:a:1", "0", "-disposition:s:0", "forced", "-disposition:s:1", "hearing_impaired",
		"-attach", font, "-metadata:s:t:0", "mimetype=application/x-truetype-font", "-metadata:s:t:0", "filename=Synthetic.ttf", mkv)
	t.Run("multitrack", func(t *testing.T) {
		r := probeFile(t, mkv)
		if len(r.Streams) != 5 || r.Streams[0].Index != 0 || !r.Streams[3].IsForced || !r.Streams[4].IsHearingImpaired {
			t.Fatal("tracks/dispositions", r.Streams)
		}
		mediaGolden(t, "generated_multitrack.golden", r)
	})
	t.Run("attached_cover", func(t *testing.T) {
		cover := filepath.Join(dir, "cover.jpg")
		ffmpeg(t, "-f", "lavfi", "-i", "color=c=red:s=32x32", "-frames:v", "1", "-threads", "1", cover)
		mp4 := filepath.Join(dir, "cover.mp4")
		ffmpeg(t, "-i", mkv, "-i", cover, "-map", "0:v:0", "-map", "0:a:0", "-map", "1:v:0", "-c", "copy", "-disposition:v:1", "attached_pic", "-t", "3", mp4)
		r := probeFile(t, mp4)
		if len(r.Streams) != 2 {
			t.Fatal("cover not filtered", r.Streams)
		}
		mediaGolden(t, "generated_cover.golden", r)
	})
	t.Run("hdr10", func(t *testing.T) {
		requireEncoder(t, "libx265")
		hdr := filepath.Join(dir, "hdr.mkv")
		ffmpeg(t, "-f", "lavfi", "-i", "color=c=green:s=160x90:r=24:d=3", "-t", "3", "-c:v", "libx265", "-threads", "1", "-pix_fmt", "yuv420p10le",
			"-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc", hdr)
		r := probeFile(t, hdr)
		s := r.Streams[0]
		if s.BitDepth == nil || *s.BitDepth != 10 || s.VideoRange != HDR || s.VideoRangeType != HDR10 {
			t.Fatal("HDR10 parsing", s)
		}
		mediaGolden(t, "generated_hdr10.golden", r)
	})
}
