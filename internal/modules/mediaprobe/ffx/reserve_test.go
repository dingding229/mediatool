package ffx

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProbeReservationsUseRealGlobalLanesAndCannotDoubleConsume(t *testing.T) {
	if err := ConfigureConcurrency(2, 1); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, closeFirst, err := ReserveProbe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFirst()
	_, closeSecond, err := ReserveProbe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSecond()
	activeEnd, err := acquireSlot(first, false)
	if err != nil {
		t.Fatal(err)
	}
	closeFirst() // A consumer owns this lane until it actually finishes.
	blocked, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, _, err := ReserveProbe(blocked); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("early close freed active permit", err)
	}
	if _, err := acquireSlot(first, false); !errors.Is(err, ErrInvalid) {
		t.Fatal("permit reused", err)
	}
	activeEnd()
	closeFirst()
	closeSecond()
	closeSecond()
	next, closeNext, err := ReserveProbe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	closeNext()
	if _, err := acquireSlot(next, false); !errors.Is(err, ErrInvalid) {
		t.Fatal("closed permit accepted")
	}
	if err := ConfigureConcurrency(2, 1); err != nil {
		t.Fatal("permit leaked", err)
	}
}
