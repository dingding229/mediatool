package ffx

import (
	"context"
	"sync/atomic"
)

type stillReservationKey struct{}
type stillReservation struct {
	state   atomic.Int32
	release func()
}

// ReserveStill holds the actual lane across a serial frame sequence. States:
// 0 idle, 1 executing, 2 closed, 3 closing after the active process is reaped.
func ReserveStill(ctx context.Context) (context.Context, func(), error) {
	release, err := processSlots.acquire(ctx, true)
	if err != nil {
		return ctx, nil, err
	}
	p := &stillReservation{release: release}
	close := func() {
		for {
			state := p.state.Load()
			switch state {
			case 0:
				if p.state.CompareAndSwap(0, 2) {
					p.release()
					return
				}
			case 1:
				if p.state.CompareAndSwap(1, 3) {
					return
				}
			default:
				return
			}
		}
	}
	return context.WithValue(ctx, stillReservationKey{}, p), close, nil
}
func reservedStill(ctx context.Context) (func(), bool, error) {
	p, ok := ctx.Value(stillReservationKey{}).(*stillReservation)
	if !ok {
		return nil, false, nil
	}
	if !p.state.CompareAndSwap(0, 1) {
		return nil, true, ErrInvalid
	}
	return func() {
		if !p.state.CompareAndSwap(1, 0) && p.state.CompareAndSwap(3, 2) {
			p.release()
		}
	}, true, nil
}
