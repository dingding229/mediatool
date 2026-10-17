package linksource

import (
	"context"
	"sync/atomic"
	"time"
)

type reservationKey struct{}
type reservation struct {
	used    atomic.Bool
	release func()
}

func admit() (func(), error) {
	admission.Lock()
	defer admission.Unlock()
	delay := time.Until(admission.after)
	if delay > 0 || admission.active >= max(1, admission.limit) {
		return nil, fail(Retryable, Limited, max(delay, 5*time.Second))
	}
	admission.active++
	return func() { admission.Lock(); admission.active--; admission.Unlock() }, nil
}

// Reserve waits BEFORE the probe lease starts. Its private one-use capability
// cannot be constructed from HTTP input and does not alter the external UA gate.
func Reserve(ctx context.Context) (context.Context, func(), error) {
	for {
		if ctx.Err() != nil {
			return ctx, nil, ctx.Err()
		}
		release, err := admit()
		if err == nil {
			p := &reservation{release: release}
			close := func() {
				if p.used.CompareAndSwap(false, true) {
					p.release()
				}
			}
			return context.WithValue(ctx, reservationKey{}, p), close, nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx, nil, ctx.Err()
		case <-timer.C:
		}
	}
}
func takeReservation(ctx context.Context) (func(), error) {
	if p, ok := ctx.Value(reservationKey{}).(*reservation); ok {
		if !p.used.CompareAndSwap(false, true) {
			return nil, fail(Permanent, Used, 0)
		}
		return p.release, nil
	}
	return admit()
}
