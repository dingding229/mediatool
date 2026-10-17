package ffx

import (
	"context"
	"golang.org/x/sync/semaphore"
	"sync"
)

var processSlots = globalSlots{probe: semaphore.NewWeighted(2), still: semaphore.NewWeighted(1)}

type globalSlots struct {
	mu           sync.Mutex
	probe, still *semaphore.Weighted
	users        int
}

// ConfigureConcurrency is process-wide, not per Runner. Changes are allowed
// only when no invocation is running OR waiting, so creating Runners cannot
// manufacture extra permits. Both lane limits are bounded to 1..2.
func ConfigureConcurrency(probe, still int) error {
	if probe < 1 || probe > 2 || still < 1 || still > 2 {
		return ErrInvalid
	}
	processSlots.mu.Lock()
	defer processSlots.mu.Unlock()
	if processSlots.users != 0 {
		return ErrInvalid
	}
	processSlots.probe = semaphore.NewWeighted(int64(probe))
	processSlots.still = semaphore.NewWeighted(int64(still))
	return nil
}
func (g *globalSlots) acquire(ctx context.Context, still bool) (func(), error) {
	g.mu.Lock()
	lane := g.probe
	if still {
		lane = g.still
	}
	g.users++
	g.mu.Unlock()
	done := func() { g.mu.Lock(); g.users--; g.mu.Unlock() }
	if err := lane.Acquire(ctx, 1); err != nil {
		done()
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		lane.Release(1)
		done()
		return nil, err
	}
	return func() { lane.Release(1); done() }, nil
}
