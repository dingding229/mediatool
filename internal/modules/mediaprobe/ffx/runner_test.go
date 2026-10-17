package ffx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("MEDIA_TOOL_FFX_HELPER") == "1" {
		helperProcess()
		return
	}
	os.Exit(m.Run())
}
func helperWrite(w interface{ Write([]byte) (int, error) }, value string) {
	if _, err := w.Write([]byte(value)); err != nil {
		os.Exit(3)
	}
}
func helperProcess() {
	mode := os.Getenv("MEDIA_TOOL_FFX_MODE")
	switch mode {
	case "argv":
		raw, err := json.Marshal(os.Args[1:])
		if err != nil {
			os.Exit(2)
		}
		helperWrite(os.Stdout, string(raw))
	case "block", "ignore":
		helperBlock(mode)
	case "overflow":
		helperWrite(os.Stdout, strings.Repeat("x", 1<<20))
		time.Sleep(20 * time.Second)
	case "stderr":
		helperWrite(os.Stderr, strings.Repeat("old-noise\n", 3000))
		helperWrite(os.Stderr, "request https://example.invalid/video.mkv?sig=synthetic-secret&token=synthetic-token\nsynthetic-secret\nlast diagnostic line\n")
		os.Exit(7)
	default:
		helperWrite(os.Stdout, `{"format":{"duration":"3.000"},"streams":[]}`)
	}
}
func fakeRunner(t *testing.T, c Config) *Runner {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c.FFprobePath, c.FFmpegPath = exe, exe
	r, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIA_TOOL_FFX_HELPER", "1")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	return r
}

const syntheticURL = "https://example.invalid/video.mkv?sig=synthetic-secret&token=synthetic-token"

func TestConfigurationAndInputWhitelist(t *testing.T) {
	r, err := New(Config{})
	if err != nil || r.config.ProbeTimeout != 30*time.Second || r.config.StillTimeout != 45*time.Second || r.config.MaxOutputBytes != 8<<20 || r.config.AllowLocalFiles {
		t.Fatal("defaults", err)
	}
	for _, c := range []Config{{ProbeTimeout: -1}, {StillTimeout: 3 * time.Minute}, {MaxOutputBytes: -1}, {MaxOutputBytes: 17 << 20}, {FFmpegPath: "sh"}, {FFprobePath: "/path\x00"}} {
		if _, err := New(c); !errors.Is(err, ErrInvalid) {
			t.Fatal(c, err)
		}
	}
	for _, link := range []string{"", "/tmp/file.mp4", "file:/tmp/file.mp4", "file:///tmp/file.mp4", "concat:https://example.invalid/a|https://example.invalid/b", "pipe:0", "ftp://example.invalid/x", "data:text/plain,abc", "https://user:password@example.invalid/file", "https://example.invalid/x\n?token=x", "https://example.invalid\\evil", "http://"} {
		if _, err := r.Probe(context.Background(), link, Options{}); !errors.Is(err, ErrInvalid) {
			t.Fatal("accepted illegal source", link, err)
		}
	}
	local := filepath.Join(t.TempDir(), "synthetic.mp4")
	if err := os.WriteFile(local, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	localRunner := fakeRunner(t, Config{AllowLocalFiles: true})
	t.Setenv("MEDIA_TOOL_FFX_MODE", "argv")
	for _, tc := range []struct {
		url  string
		opts Options
	}{{syntheticURL, Options{UserAgent: "bad\r\nHeader: x"}}, {syntheticURL, Options{ProbeSize: 1}}, {syntheticURL, Options{AnalyzeDuration: -1}}, {syntheticURL, Options{Timeout: 3 * time.Minute}}} {
		if _, err := localRunner.Probe(context.Background(), tc.url, tc.opts); !errors.Is(err, ErrInvalid) {
			t.Fatal(tc, err)
		}
	}
	for _, o := range []Options{{Width: 2000}, {Height: 2000}, {Quality: 32}} {
		if _, err := localRunner.Still(context.Background(), syntheticURL, 0, o); !errors.Is(err, ErrInvalid) {
			t.Fatal(o, err)
		}
	}
	if _, err := localRunner.Still(context.Background(), syntheticURL, -1, Options{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	raw, err := localRunner.Probe(context.Background(), local, Options{})
	if err != nil || !strings.Contains(string(raw), "http,https,tcp,tls,file") {
		t.Fatal(string(raw), err)
	}
	for _, link := range []string{"file:" + local, "pipe:0", "concat:" + local} {
		if _, err := localRunner.Probe(context.Background(), link, Options{}); !errors.Is(err, ErrInvalid) {
			t.Fatal("local switch allowed protocol", link, err)
		}
	}
	if _, err := (*Runner)(nil).Probe(context.Background(), syntheticURL, Options{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := r.Probe(nil, syntheticURL, Options{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
func TestFixedArgvAndNoCallerArguments(t *testing.T) {
	r := fakeRunner(t, Config{})
	t.Setenv("MEDIA_TOOL_FFX_MODE", "argv")
	raw, err := r.Probe(context.Background(), syntheticURL, Options{UserAgent: "Synthetic Player/1.0"})
	if err != nil {
		t.Fatal(err)
	}
	var argv []string
	if err := json.Unmarshal(raw, &argv); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(argv, " ")
	for _, marker := range []string{"-protocol_whitelist http,https,tcp,tls", "-show_format", "-show_streams", "-show_chapters", "-print_format json", "-rw_timeout 10000000", "-user_agent Synthetic Player/1.0"} {
		if !strings.Contains(joined, marker) {
			t.Fatal("missing fixed argument", marker, joined)
		}
	}
	if argv[len(argv)-1] != syntheticURL || strings.Contains(joined, "file,") {
		t.Fatal("source/whitelist", joined)
	}
	raw, err = r.Still(context.Background(), syntheticURL, 1500*time.Millisecond, Options{Width: 320, Height: 180, Quality: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &argv); err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(argv, " ")
	for _, marker := range []string{"-nostdin", "-ss 1.500000", "-frames:v 1", "-an -sn -dn", "scale=320:180:force_original_aspect_ratio=decrease", "-threads 1", "-c:v mjpeg", "-f image2pipe pipe:1"} {
		if !strings.Contains(joined, marker) {
			t.Fatal("missing still argument", marker, joined)
		}
	}
}
func TestUnavailableExecutablesAreLazyAndAllowDegradation(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-ffmpeg")
	r, err := New(Config{FFprobePath: missing, FFmpegPath: missing})
	if err != nil {
		t.Fatal("startup should not fail", err)
	}
	for _, still := range []bool{false, true} {
		_, err := r.run(context.Background(), syntheticURL, 0, Options{}, still)
		if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := Probe(context.Background(), syntheticURL, Options{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := Still(context.Background(), syntheticURL, 0, Options{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestOutputCapsAndSanitizedStderrTail(t *testing.T) {
	r := fakeRunner(t, Config{MaxOutputBytes: 1024})
	t.Setenv("MEDIA_TOOL_FFX_MODE", "overflow")
	start := time.Now()
	raw, err := r.Probe(context.Background(), syntheticURL, Options{})
	if !errors.Is(err, ErrOutputLimit) || len(raw) != 0 || time.Since(start) > 2*time.Second {
		t.Fatal("output cap/kill", len(raw), err)
	}
	t.Setenv("MEDIA_TOOL_FFX_MODE", "stderr")
	_, err = r.Probe(context.Background(), syntheticURL, Options{})
	var failure *ProcessError
	if !errors.As(err, &failure) || !errors.Is(err, ErrProcess) {
		t.Fatal(err)
	}
	if len(failure.Stderr) > 4096 || !strings.HasSuffix(failure.Stderr, "last diagnostic line\n") || !strings.Contains(failure.Stderr, "request https://example.invalid/video.mkv") {
		t.Fatal("stderr tail", failure.Stderr)
	}
	for _, secret := range []string{"synthetic-secret", "synthetic-token", "?sig=", "token=", "https://user:"} {
		if strings.Contains(failure.Error()+failure.Stderr, secret) {
			t.Fatal("error leaked signed URL", secret)
		}
	}
}
func TestGlobalConcurrencySharedByDifferentRunnersAndWaitingCancellation(t *testing.T) {
	if err := ConfigureConcurrency(1, 1); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := ConfigureConcurrency(2, 1); err != nil {
			t.Error(err)
		}
	}()
	r1, r2 := fakeRunner(t, Config{}), fakeRunner(t, Config{})
	t.Setenv("MEDIA_TOOL_FFX_MODE", "block")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := r1.Probe(ctx, syntheticURL, Options{}); done <- err }()
	waitForSlots(t, 1)
	if err := ConfigureConcurrency(2, 2); !errors.Is(err, ErrInvalid) {
		t.Fatal("reconfigured active semaphore", err)
	}
	waitCtx, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer stop()
	if _, err := r2.Probe(waitCtx, syntheticURL, Options{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("waiting deadline", err)
	}
	t.Setenv("MEDIA_TOOL_FFX_MODE", "argv")
	// The independent Still lane remains available while Probe is occupied.
	if _, err := r2.Still(context.Background(), syntheticURL, 0, Options{Timeout: time.Second}); err != nil {
		t.Fatal("probe blocked still lane", err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := r2.Probe(context.Background(), syntheticURL, Options{Timeout: time.Second}); err != nil {
		t.Fatal("permit leaked", err)
	}
	for _, limits := range [][2]int{{0, 1}, {3, 1}, {1, 3}} {
		if err := ConfigureConcurrency(limits[0], limits[1]); !errors.Is(err, ErrInvalid) {
			t.Fatal(limits, err)
		}
	}
}
func waitForSlots(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		processSlots.mu.Lock()
		users := processSlots.users
		processSlots.mu.Unlock()
		if users >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("call did not enter semaphore")
}
func TestParallelCallsRespectGlobalDefaultLimits(t *testing.T) {
	if err := ConfigureConcurrency(2, 1); err != nil {
		t.Fatal(err)
	}
	r := fakeRunner(t, Config{})
	t.Setenv("MEDIA_TOOL_FFX_MODE", "argv")
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Go(func() {
			if _, err := r.Probe(context.Background(), syntheticURL, Options{}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	processSlots.mu.Lock()
	users := processSlots.users
	processSlots.mu.Unlock()
	if users != 0 {
		t.Fatal("users leaked", users)
	}
}
func TestRealFFmpegProbeAndStillFromGeneratedH264AAC(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		if os.Getenv("MEDIA_TOOL_REQUIRE_FFMPEG") == "1" {
			t.Fatal("ffmpeg absent")
		}
		t.Skip("ffmpeg absent")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		if os.Getenv("MEDIA_TOOL_REQUIRE_FFMPEG") == "1" {
			t.Fatal("ffprobe absent")
		}
		t.Skip("ffprobe absent")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "synthetic.srt")
	file := filepath.Join(dir, "synthetic.mp4")
	if err := os.WriteFile(sub, []byte("1\n00:00:00,100 --> 00:00:02,800\nSynthetic subtitle\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=3", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=3", "-i", sub, "-t", "3", "-map", "0:v", "-map", "1:a", "-map", "2:s", "-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", "-c:a", "aac", "-c:s", "mov_text", file)
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate synthetic h264+aac+subtitle: %v %s", err, raw)
	}
	r, err := New(Config{AllowLocalFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := r.Probe(ctx, file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		Format  struct{ Duration string }
		Streams []struct {
			CodecName string `json:"codec_name"`
			CodecType string `json:"codec_type"`
		}
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatal("probe JSON", err)
	}
	duration, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil || duration < 2.9 || duration > 3.2 {
		t.Fatal("duration", duration, err)
	}
	codecs := map[string]string{}
	for _, stream := range probe.Streams {
		codecs[stream.CodecType] = stream.CodecName
	}
	if codecs["video"] != "h264" || codecs["audio"] != "aac" || codecs["subtitle"] != "mov_text" {
		t.Fatal("synthetic codecs", codecs)
	}
	verifyJPEG := func(raw []byte, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		image, err := jpeg.Decode(bytes.NewReader(raw))
		if err != nil || image.Bounds().Dx() != 320 || image.Bounds().Dy() != 180 {
			t.Fatal("JPEG output", err)
		}
	}
	verifyJPEG(r.Still(ctx, file, time.Second, Options{Width: 320, Height: 180}))
	if raw, err := r.Still(ctx, file, 10*time.Second, Options{}); !errors.Is(err, ErrProcess) || len(raw) != 0 {
		t.Fatal("empty past-EOF screenshot reported success", len(raw), err)
	}
	var observed atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		observed.Store(req.UserAgent())
		http.ServeFile(w, req, file)
	}))
	defer server.Close()
	remote, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	url := server.URL + "/synthetic.mp4?signature=synthetic-only"
	if raw, err := remote.Probe(ctx, url, Options{UserAgent: "Synthetic Probe/1"}); err != nil || !json.Valid(raw) {
		t.Fatal("HTTP probe", err)
	}
	if observed.Load() != "Synthetic Probe/1" {
		t.Fatal("HTTP UA not propagated", observed.Load())
	}
	verifyJPEG(remote.Still(ctx, url, time.Second, Options{Width: 320, Height: 180, UserAgent: "Synthetic Probe/1"}))
	t.Logf("real ffprobe JSON + JPEG verified with lavfi 3s H264/AAC/mov_text fixture (%s)", filepath.Base(ffmpeg))
}

func TestDefaultLaneCapacityAndCancelledWaiters(t *testing.T) {
	if err := ConfigureConcurrency(2, 1); err != nil {
		t.Fatal(err)
	}
	a, err := processSlots.acquire(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer a()
	b, err := processSlots.acquire(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer b()
	still, err := processSlots.acquire(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer still()
	for _, lane := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		release, err := processSlots.acquire(ctx, lane)
		cancel()
		if release != nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("global default capacity", lane, err)
		}
	}
}
func TestStderrTailRedactsOtherURLsAndPartialLines(t *testing.T) {
	input, err := inputSource(syntheticURL, false)
	if err != nil {
		t.Fatal(err)
	}
	tail := &tailOutput{}
	if _, err := tail.Write([]byte("https://other.invalid/path?different-signature=other-secret\n")); err != nil {
		t.Fatal(err)
	}
	if value := tail.safe(input); strings.Contains(value, "other-secret") || strings.Contains(value, "?") {
		t.Fatal("unrelated URL query leaked", value)
	}
	tail = &tailOutput{}
	if _, err := tail.Write([]byte(strings.Repeat("partial-sensitive-signature", 400))); err != nil {
		t.Fatal(err)
	}
	if value := tail.safe(input); value != "[stderr tail truncated]" {
		t.Fatal("partial first line leaked", value)
	}
}
func TestDockerFFmpegDependencyIsRuntimeOnly(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "deploy", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	stages := strings.Split(string(raw), " AS runtime\n")
	if len(stages) != 2 || strings.Contains(stages[0], "apk add --no-cache ffmpeg") || !strings.Contains(stages[1], "apk add --no-cache ca-certificates postgresql18-client ffmpeg") {
		t.Fatal("ffmpeg must install only in runtime stage")
	}
}
