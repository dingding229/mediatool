package ffx

import (
	"context"
	"sync/atomic"
)

type probeReservationKey struct{}
type probeReservation struct {
	used    atomic.Bool
	release func()
}

// ReserveProbe acquires the actual process-wide lane before a durable lease.
// Close releases an unused permit; once consumed, Probe owns release until exit.
func ReserveProbe(ctx context.Context) (context.Context, func(), error) {
	release, err := processSlots.acquire(ctx, false)
	if err != nil {
		return ctx, nil, err
	}
	p := &probeReservation{release: release}
	close := func() {
		if p.used.CompareAndSwap(false, true) {
			p.release()
		}
	}
	return context.WithValue(ctx, probeReservationKey{}, p), close, nil
}
func acquireSlot(ctx context.Context, still bool) (func(), error) {
	if still {
		if release, ok, err := reservedStill(ctx); ok {
			return release, err
		}
	}
	if !still {
		if p, ok := ctx.Value(probeReservationKey{}).(*probeReservation); ok {
			if !p.used.CompareAndSwap(false, true) {
				return nil, ErrInvalid
			}
			return p.release, nil
		}
	}
	return processSlots.acquire(ctx, still)
}
