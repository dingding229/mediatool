package plugins

import (
	"context"
	"errors"
	"testing"
	"time"
)

func leb(n uint32) []byte {
	out := []byte{}
	for {
		b := byte(n & 127)
		n >>= 7
		if n != 0 {
			b |= 128
		}
		out = append(out, b)
		if n == 0 {
			return out
		}
	}
}
func section(id byte, raw []byte) []byte {
	return append(append([]byte{id}, leb(uint32(len(raw)))...), raw...)
}

// Tiny real WASM modules, not mocked interpreter results.
func sandboxModule(body []byte, pages uint32) []byte {
	out := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	out = append(out, section(1, []byte{1, 0x60, 0, 0})...)
	out = append(out, section(3, []byte{1, 0})...)
	out = append(out, section(5, append([]byte{1, 0}, leb(pages)...))...)
	out = append(out, section(7, []byte{1, 6, '_', 's', 't', 'a', 'r', 't', 0, 0})...)
	code := append([]byte{0}, body...)
	code = append(code, 0x0b)
	out = append(out, section(10, append(append([]byte{1}, leb(uint32(len(code)))...), code...))...)
	return out
}
func TestSandboxRequiresEntrypointAndBoundsMemoryAndTime(t *testing.T) {
	ctx := context.Background()
	if ValidateModule(ctx, []byte{0, 97, 115, 109, 1, 0, 0, 0}) == nil {
		t.Fatal("manifest with empty code accepted")
	}
	if ValidateModule(ctx, sandboxModule(nil, 1025)) == nil {
		t.Fatal("more than 64MiB accepted")
	}
	loop := sandboxModule([]byte{0x03, 0x40, 0x0c, 0, 0x0b}, 1)
	if ValidateModule(ctx, loop) != nil {
		t.Fatal("valid loop module rejected at compile")
	}
	bounded, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, e := executeWASM(bounded, loop, Manifest{ID: "owned-loop", Runtime: "wasm"}, Event{Kind: "plugin.test"}, nil, nil)
	if e == nil || time.Since(start) > time.Second || !errors.Is(bounded.Err(), context.DeadlineExceeded) {
		t.Fatal("loop not interrupted", e, time.Since(start))
	}
}
func TestSandboxBoundedOutputDoesNotGrowAfterRejection(t *testing.T) {
	out := boundedBuffer{limit: 4}
	if _, e := out.Write([]byte("1234")); e != nil {
		t.Fatal(e)
	}
	if _, e := out.Write([]byte("5")); e == nil || string(out.data) != "1234" {
		t.Fatal("unbounded output", out.data, e)
	}
}
func TestSandboxRejectsUnknownHostFunctionsAndImportedMemory(t *testing.T) {
	for _, ns := range []string{"env", "mediatool", "mediatool_host"} {
		out := []byte{0, 97, 115, 109, 1, 0, 0, 0}
		out = append(out, section(1, []byte{1, 0x60, 0, 0})...)
		imp := append([]byte{1, byte(len(ns))}, []byte(ns)...)
		imp = append(imp, 5, 's', 'h', 'e', 'l', 'l', 0, 0)
		out = append(out, section(2, imp)...)
		out = append(out, section(3, []byte{1, 0})...)
		out = append(out, section(7, []byte{1, 6, '_', 's', 't', 'a', 'r', 't', 0, 1})...)
		out = append(out, section(10, []byte{1, 2, 0, 0x0b})...)
		if ValidateModule(context.Background(), out) == nil {
			t.Fatal("arbitrary host function", ns)
		}
	}
	out := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	out = append(out, section(1, []byte{1, 0x60, 0, 0})...)
	out = append(out, section(2, []byte{1, 3, 'e', 'n', 'v', 6, 'm', 'e', 'm', 'o', 'r', 'y', 2, 0, 1})...)
	out = append(out, section(3, []byte{1, 0})...)
	out = append(out, section(7, []byte{1, 6, '_', 's', 't', 'a', 'r', 't', 0, 0})...)
	out = append(out, section(10, []byte{1, 2, 0, 0x0b})...)
	if ValidateModule(context.Background(), out) == nil {
		t.Fatal("host memory import")
	}
}
