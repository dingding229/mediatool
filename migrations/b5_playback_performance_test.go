package migrations_test

import (
	"encoding/json"
	"io"
	"media-tool/internal/modules/embyapi/dto"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

func TestB5PlaybackPerformance(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_B5_PERF") != "1" {
		t.Skip("explicit representative workload")
	}
	f := newPlaybackFixture(t)
	if _, err := f.provider.Up(f.ctx); err != nil {
		t.Fatal(err)
	}
	b5SeedMovies(t, f.browseFixture, 10000)
	previous := f.manager.OutboundHTTP.Transport
	f.manager.OutboundHTTP.Transport = playbackTransport(func(r *http.Request) (*http.Response, error) {
		response, err := previous.RoundTrip(r)
		if err != nil {
			return response, err
		}
		raw, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		response.Body = io.NopCloser(strings.NewReader(strings.ReplaceAll(string(raw), "cdn.synthetic.invalid", "edge.115cdn.net")))
		return response, nil
	})
	info := func() {
		w := f.call("GET", "/emby/Items/"+f.item+"/PlaybackInfo", f.token, "SenPlayer/6.2.1")
		var data dto.PlaybackInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil || w.Code != 200 || len(data.MediaSources) != 2 {
			t.Fatal(w.Code, err)
		}
	}
	resolve := func() {
		w := f.call("HEAD", "/Videos/"+f.item+"/stream.mkv?Static=true&MediaSourceId="+f.a.asset, f.token, "SenPlayer/6.2.1")
		if w.Code != 302 || w.Body.Len() != 0 || !strings.Contains(w.Header().Get("Location"), ".115cdn.net/") {
			t.Fatal(w.Code, w.Body.String(), w.Header().Get("Location"))
		}
	}
	for _, load := range []struct {
		name string
		run  func()
	}{{"PlaybackInfo", info}, {"resolver302", resolve}} {
		runtime.GC()
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		start := time.Now()
		load.run()
		elapsed := time.Since(start)
		runtime.ReadMemStats(&b)
		t.Logf("B5_PLAYBACK name=%s ms=%.3f allocBytes=%d mallocs=%d", load.name, float64(elapsed)/1e6, b.TotalAlloc-a.TotalAlloc, b.Mallocs-a.Mallocs)
	}
	if dir := os.Getenv("B5_PROFILE_DIR"); dir != "" {
		os.MkdirAll(dir, 0700)
		cpu, err := os.Create(filepath.Join(dir, "playback.cpu.pprof"))
		if err != nil {
			t.Fatal(err)
		}
		if err = pprof.StartCPUProfile(cpu); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 6; i++ {
			info()
			resolve()
		}
		pprof.StopCPUProfile()
		cpu.Close()
		for _, kind := range []string{"heap", "allocs"} {
			file, err := os.Create(filepath.Join(dir, "playback."+kind+".pprof"))
			if err != nil {
				t.Fatal(err)
			}
			pprof.Lookup(kind).WriteTo(file, 0)
			file.Close()
		}
	}
}
