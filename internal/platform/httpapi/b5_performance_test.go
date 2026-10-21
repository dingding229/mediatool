package httpapi

import (
	"context"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

type b5SSEWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w b5SSEWriter) Flush() { w.ResponseRecorder.Flush(); w.cancel() }
func TestB5SSEAuthenticatedFlush(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_B5_PERF") != "1" {
		t.Skip("explicit representative workload")
	}
	ctx, s := pluginHTTPFixture(t)
	c := cfg()
	token, _ := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if _, err := (accounts.Repository{DB: s.DB}).Register(ctx, "b5_sse", "synthetic-password", token); err != nil {
		t.Fatal(err)
	}
	a := &API{Config: c, Deps: Dependencies{DB: s.DB}, Auth: auth.Auth{Key: c.SessionKey}}
	dir := os.Getenv("B5_PROFILE_DIR")
	var cpu *os.File
	if dir != "" {
		os.MkdirAll(dir, 0700)
		var err error
		cpu, err = os.Create(filepath.Join(dir, "sse.cpu.pprof"))
		if err != nil {
			t.Fatal(err)
		}
		if err = pprof.StartCPUProfile(cpu); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	for i := 0; i < 100; i++ {
		requestCtx, cancel := context.WithCancel(ctx)
		r := httptest.NewRequest("GET", "/api/v1/jobs/events", nil).WithContext(requestCtx)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := b5SSEWriter{httptest.NewRecorder(), cancel}
		a.events(w, r)
		cancel()
		if !strings.Contains(w.Body.String(), "event: tick") {
			t.Fatal("SSE event missing")
		}
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	if cpu != nil {
		pprof.StopCPUProfile()
		cpu.Close()
		for _, kind := range []string{"heap", "allocs"} {
			f, err := os.Create(filepath.Join(dir, "sse."+kind+".pprof"))
			if err != nil {
				t.Fatal(err)
			}
			pprof.Lookup(kind).WriteTo(f, 0)
			f.Close()
		}
	}
	t.Logf("B5_SSE ms=%.3f allocBytes=%d mallocs=%d streams=100", float64(elapsed)/1e6, after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs)
}
