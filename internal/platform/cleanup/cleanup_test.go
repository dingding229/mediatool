package cleanup

import (
	"context"
	"errors"
	"testing"
	"time"
)

type badCloser struct{ closed bool }

func (c *badCloser) Close() error { c.closed = true; return errors.New("synthetic close failure") }

type rollbackProbe struct {
	called    bool
	cancelled bool
	bounded   bool
}

func (q *rollbackProbe) Rollback(ctx context.Context) error {
	q.called = true
	q.cancelled = ctx.Err() != nil
	d, ok := ctx.Deadline()
	q.bounded = ok && time.Until(d) <= 2*time.Second
	return errors.New("already completed")
}
func TestExplicitCleanupDoesNotLosePrimaryResults(t *testing.T) {
	c := &badCloser{}
	Close(c, "Read-only resource after primary read result is known")
	if !c.closed {
		t.Fatal("close omitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := &rollbackProbe{}
	Rollback(ctx, q)
	if !q.called || q.cancelled || !q.bounded {
		t.Fatal("rollback must use detached bounded cleanup", q)
	}
	defer func() {
		if recover() == nil {
			t.Error("missing rationale accepted")
		}
	}()
	Ignore(nil, "")
}
