//go:build linux && !cgo

package sandbox

import (
	"io"
	"runtime/pprof"
	"time"
)

func profileCPU(w io.Writer) error {
	if e := pprof.StartCPUProfile(w); e != nil {
		return e
	}
	deadline := time.Now().Add(120 * time.Millisecond)
	var value uint64
	for time.Now().Before(deadline) {
		value++
	}
	_ = value
	pprof.StopCPUProfile()
	return nil
}
