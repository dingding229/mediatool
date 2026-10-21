package filemanager

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"
)

func TestB5OrganizeTwenty(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_B5_PERF") != "1" {
		t.Skip("explicit representative workload")
	}
	ctx, e, root, _ := fixture(t)
	for i := 0; i < 20; i++ {
		if err := os.WriteFile(filepath.Join(root.Path, fmt.Sprintf("Synthetic%02d.mkv", i)), []byte("synthetic fixture, not media"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir := os.Getenv("B5_PROFILE_DIR")
	var cpu *os.File
	if dir != "" {
		os.MkdirAll(dir, 0700)
		var err error
		cpu, err = os.Create(filepath.Join(dir, "organize.cpu.pprof"))
		if err != nil {
			t.Fatal(err)
		}
		if err = pprof.StartCPUProfile(cpu); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	start := time.Now()
	for i := 0; i < 20; i++ {
		p, err := e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "organize", Source: fmt.Sprintf("Synthetic%02d.mkv", i), MediaType: "movie", Title: fmt.Sprintf("Organized%02d", i), Year: "2026", Template: "{title} ({year}){ext}"})
		if err != nil {
			t.Fatal(err)
		}
		r, err := e.Execute(ctx, p.ID, p.Digest, true)
		if err != nil || r.State != "succeeded" {
			t.Fatal(r, err)
		}
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&b)
	if cpu != nil {
		pprof.StopCPUProfile()
		cpu.Close()
		for _, kind := range []string{"heap", "allocs"} {
			f, err := os.Create(filepath.Join(dir, "organize."+kind+".pprof"))
			if err != nil {
				t.Fatal(err)
			}
			pprof.Lookup(kind).WriteTo(f, 0)
			f.Close()
		}
	}
	t.Logf("B5_ORGANIZE ms=%.3f allocBytes=%d mallocs=%d files=20", float64(elapsed)/1e6, b.TotalAlloc-a.TotalAlloc, b.Mallocs-a.Mallocs)
}
