package clouddrive2

import (
	"context"
	"media-tool/internal/modules/storage"
)

type operationKey struct{}

func (a *Adapter) acquire(ctx context.Context) (context.Context, func(), error) {
	if active, _ := ctx.Value(operationKey{}).(*Adapter); active == a {
		return ctx, func() {}, nil
	}
	a.lifecycle.Lock()
	if a.retired {
		a.lifecycle.Unlock()
		return ctx, nil, storage.ErrUnavailable
	}
	a.active++
	a.lifecycle.Unlock()
	return context.WithValue(ctx, operationKey{}, a), func() {
		a.lifecycle.Lock()
		defer a.lifecycle.Unlock()
		a.active--
		if a.retired && a.active == 0 {
			a.conn.Close()
		}
	}, nil
}

// Retire prevents new work but retains the connection until streams/RPCs drain.
func (a *Adapter) Retire() {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.retired = true
	if a.active == 0 {
		a.conn.Close()
	}
}
