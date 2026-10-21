package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/transport"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"
)

func TestB5FencedHostRPC(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_B5_PERF") != "1" {
		t.Skip("explicit representative workload")
	}
	s, l := activeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	client, err := transport.New(&transport.Stream{Reader: a, Writer: a}, transport.Options{Prefix: "p", MaxPending: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := transport.New(&transport.Stream{Reader: b, Writer: b}, transport.Options{Prefix: "h", MaxPending: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if err = server.Handle("host.call", s.Host(l)); err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err = server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("B5_PROFILE_DIR")
	var cpu *os.File
	if dir != "" {
		os.MkdirAll(dir, 0700)
		cpu, err = os.Create(filepath.Join(dir, "rpc.cpu.pprof"))
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
	for i := 0; i < 200; i++ {
		var out json.RawMessage
		if err = client.Call(ctx, "host.call", json.RawMessage(`{"method":"plugin.state.get","args":{}}`), &out); err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	if cpu != nil {
		pprof.StopCPUProfile()
		cpu.Close()
		for _, kind := range []string{"heap", "allocs"} {
			f, err := os.Create(filepath.Join(dir, "rpc."+kind+".pprof"))
			if err != nil {
				t.Fatal(err)
			}
			pprof.Lookup(kind).WriteTo(f, 0)
			f.Close()
		}
	}
	t.Logf("B5_RPC ms=%.3f allocBytes=%d mallocs=%d calls=200", float64(elapsed)/1e6, after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs)
}
