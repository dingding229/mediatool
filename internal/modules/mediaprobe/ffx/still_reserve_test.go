package ffx

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStillReservationSerialFramesAndClose(t *testing.T) {
	ctx, close, err := ReserveStill(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		release, err := acquireSlot(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := acquireSlot(ctx, true); !errors.Is(err, ErrInvalid) {
			t.Fatal("concurrent reuse")
		}
		release()
	}
	wait, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, _, err := ReserveStill(wait); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("permit released between frames", err)
	}
	release, err := acquireSlot(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	close()
	close()
	release()
	if _, err := acquireSlot(ctx, true); !errors.Is(err, ErrInvalid) {
		t.Fatal("closed permit reused")
	}
	_, done, err := ReserveStill(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	done()
}
