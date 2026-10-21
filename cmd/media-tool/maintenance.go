package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/maintenance"
)

// Pause cancels/drains plugin subprocesses without changing administrator intent.
// Resume starts a fresh supervisor against the verified/restored catalog.
func runPluginSupervisor(ctx context.Context, s interface{ Run(context.Context) error }, db *pgxpool.Pool) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var cancel context.CancelFunc
	var done chan error
	defer func() {
		if cancel != nil {
			cancel()
			<-done
		}
	}()
	for {
		check, stop := context.WithTimeout(ctx, 2*time.Second)
		paused, err := maintenance.Paused(check, db)
		stop()
		if paused || err != nil {
			if cancel != nil {
				cancel()
				<-done
				cancel = nil
				done = nil
			}
		} else if cancel == nil {
			run, stop := context.WithCancel(ctx)
			cancel = stop
			done = make(chan error, 1)
			go func(ch chan error) { ch <- s.Run(run) }(done)
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-done:
			cancel()
			cancel = nil
			done = nil
			return err
		case <-ticker.C:
		}
	}
}
