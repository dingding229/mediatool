package stillrunner

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"media-tool/internal/modules/mediaprobe/ffx"
	"media-tool/internal/modules/mediaprobe/stillqueue"
)

func jpegFrame(t *testing.T, c color.Color) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 32, 18))
	for y := 0; y < 18; y++ {
		for x := 0; x < 32; x++ {
			im.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, im, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type ticket struct {
	url, ua string
	used    bool
}

func (t *ticket) Claim() (string, string, error) {
	if t.used {
		return "", "", errors.New("used")
	}
	t.used = true
	return t.url, t.ua, nil
}

type fakeLinks struct {
	calls, reserves int
	url             string
}

func (l *fakeLinks) Reserve(ctx context.Context) (context.Context, func(), error) {
	l.reserves++
	return ctx, func() {}, nil
}
func (l *fakeLinks) Fetch(context.Context, string) (Ticket, error) {
	l.calls++
	return &ticket{url: l.url, ua: "SyntheticProbe/1.0"}, nil
}

type fakeFrames struct {
	raw     [][]byte
	offsets []time.Duration
}

func (f *fakeFrames) Reserve(ctx context.Context) (context.Context, func(), error) {
	return ctx, func() {}, nil
}
func (f *fakeFrames) Still(_ context.Context, url string, at time.Duration, o FrameOptions) ([]byte, error) {
	if url != "synthetic" || o.UserAgent != "SyntheticProbe/1.0" || o.Width != 1280 {
		return nil, ErrImage
	}
	f.offsets = append(f.offsets, at)
	return f.raw[len(f.offsets)-1], nil
}
func TestAllBlankChoosesHighestVariance(t *testing.T) {
	im := image.NewGray(image.Rect(0, 0, 32, 18))
	for y := 0; y < 18; y++ {
		for x := 0; x < 32; x++ {
			if (x+y)%2 == 0 {
				im.SetGray(x, y, color.Gray{Y: 10})
			}
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, im, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	varied := b.Bytes()
	stats, err := Analyze(varied)
	if err != nil || !stats.Blank() || stats.Variance == 0 {
		t.Fatal(stats, err)
	}
	c, err := (stillqueue.Config{}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	frames := &fakeFrames{raw: [][]byte{jpegFrame(t, color.Black), varied, jpegFrame(t, color.White)}}
	raw, err := Capture(t.Context(), stillqueue.Lease{DurationTicks: 30_000_000}, &fakeLinks{url: "synthetic"}, frames, c)
	if err != nil || !bytes.Equal(raw, varied) {
		t.Fatal("variance fallback", err)
	}
}

func TestCaptureOrderBoundsAndAnalysis(t *testing.T) {
	c, err := (stillqueue.Config{}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	black, white, gray := jpegFrame(t, color.Black), jpegFrame(t, color.White), jpegFrame(t, color.Gray{Y: 100})
	for _, tt := range []struct {
		frames   [][]byte
		want     []time.Duration
		expected []byte
	}{{[][]byte{gray}, []time.Duration{450 * time.Millisecond}, gray}, {[][]byte{black, white, gray}, []time.Duration{450 * time.Millisecond, 360 * time.Millisecond, 540 * time.Millisecond}, gray}, {[][]byte{black, black, black}, []time.Duration{450 * time.Millisecond, 360 * time.Millisecond, 540 * time.Millisecond}, black}} {
		links := &fakeLinks{url: "synthetic"}
		frames := &fakeFrames{raw: tt.frames}
		raw, err := Capture(t.Context(), stillqueue.Lease{AssetID: "synthetic", DurationTicks: 30_000_000}, links, frames, c)
		if err != nil || !bytes.Equal(raw, tt.expected) || links.calls != len(tt.want) || links.reserves != len(tt.want)-1 {
			t.Fatal(err, links, frames.offsets)
		}
		for i, at := range frames.offsets {
			if at != tt.want[i] || at <= 0 {
				t.Fatal(frames.offsets)
			}
		}
	}
	for _, raw := range [][]byte{nil, []byte("invalid"), bytes.Repeat([]byte{'x'}, int(stillqueue.MaxImageBytes)+1)} {
		if _, err := Analyze(raw); err == nil {
			t.Fatal("bad image accepted")
		}
	}
	for _, ticks := range []int64{0, -1, 1, 1<<63 - 1} {
		if _, err := offset(ticks, 15); err == nil {
			t.Fatal("unsafe duration")
		}
	}
}

type realFrames struct{ runner *ffx.Runner }

func (f realFrames) Reserve(ctx context.Context) (context.Context, func(), error) {
	return ffx.ReserveStill(ctx)
}
func (f realFrames) Still(ctx context.Context, url string, at time.Duration, o FrameOptions) ([]byte, error) {
	return f.runner.Still(ctx, url, at, ffx.Options{UserAgent: o.UserAgent, Timeout: o.Timeout, Width: o.Width, Height: o.Height, Quality: o.Quality})
}
func TestRealFFmpegBlackIntroFallback(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		if os.Getenv("MEDIA_TOOL_REQUIRE_FFMPEG") == "1" {
			t.Fatal(err)
		}
		t.Skip("ffmpeg absent")
	}
	file := filepath.Join(t.TempDir(), "synthetic-black-intro.mp4")
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=black:s=160x90:r=100:d=0.5", "-f", "lavfi", "-i", "testsrc2=s=160x90:r=100:d=2.5", "-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0", "-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", file)
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("synthetic ffmpeg failed: %v %.1000s", err, raw)
	}
	runner, err := ffx.New(ffx.Config{AllowLocalFiles: true, MaxOutputBytes: int(stillqueue.MaxImageBytes)})
	if err != nil {
		t.Fatal(err)
	}
	c, err := (stillqueue.Config{}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	reserved, release, err := ffx.ReserveStill(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	links := &fakeLinks{url: file}
	raw, err := Capture(reserved, stillqueue.Lease{AssetID: "synthetic", DurationTicks: 30_000_000}, links, realFrames{runner}, c)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := Analyze(raw)
	if err != nil || stats.Blank() || links.calls != 3 {
		t.Fatal("black intro fallback did not execute", stats, links.calls, err)
	}
	t.Logf("REAL_FFMPEG black-intro frames=%d mean=%.2f variance=%.2f", links.calls, stats.Mean, stats.Variance)
}
