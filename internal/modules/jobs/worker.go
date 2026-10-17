package jobs

import (
	"context"
	"errors"
	"log/slog"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/trace"
	"time"
)

type Handler func(context.Context, Job) error
type Worker struct {
	Repository          Repository
	Handlers            map[string]Handler
	Owner               string
	Queue               string
	PollInterval, Lease time.Duration
	Logger              *slog.Logger
}

func (w Worker) Run(ctx context.Context) error {
	if w.Repository.DB == nil {
		return errors.New("worker requires DATABASE_URL")
	}
	if w.Lease < 3*time.Second || w.PollInterval < 100*time.Millisecond || w.Logger == nil {
		return errors.New("invalid worker configuration")
	}
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			check, cancel := context.WithTimeout(ctx, 2*time.Second)
			_, err := w.Repository.DB.Exec(check, `INSERT INTO worker_heartbeats(owner,queue,last_seen_at)VALUES($1,$2,now()) ON CONFLICT(owner) DO UPDATE SET last_seen_at=now()`, w.Owner, queueName(w.Queue))
			cancel()
			if err != nil {
				w.Logger.Warn("worker heartbeat unavailable")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	defer func() {
		<-heartbeatDone
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, checkedErr := w.Repository.DB.Exec(cleanup, `DELETE FROM worker_heartbeats WHERE owner=$1`, w.Owner); checkedErr != nil {
			deferutil.Report(checkedErr, "internal/modules/jobs/worker.go:")
			return
		}
	}()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			j, err := w.Repository.ClaimQueue(ctx, w.Owner, w.Lease, w.Queue)
			if err != nil {
				w.Logger.Error("job claim failed")
				timer.Reset(w.PollInterval)
				continue
			}
			if j == nil {
				timer.Reset(w.PollInterval)
				continue
			}
			w.Logger.Info("job claimed", "jobId", j.ID, "kind", j.Kind, "queue", w.Queue)
			w.execute(ctx, *j)
			timer.Reset(0)
		}
	}
}
func (w Worker) execute(parent context.Context, j Job) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(w.Lease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				heartbeatCtx, stop := context.WithTimeout(ctx, w.Lease/4)
				err := w.Repository.Heartbeat(heartbeatCtx, j, w.Lease)
				stop()
				if err != nil {
					w.Logger.Warn("job heartbeat lost", "jobId", j.ID)
					cancel()
					return
				}
			}
		}
	}()
	workErr := func() (err error) {
		defer func() {
			if recover() != nil {
				err = errors.New("handler panicked")
			}
		}()
		handler, ok := w.Handlers[j.Kind]
		if !ok {
			return errors.New("job handler not registered")
		}
		return handler(trace.With(ctx, w.Logger.With("jobId", j.ID, "queue", w.Queue, "kind", j.Kind)), j)
	}()
	canceled := ctx.Err() != nil
	cancel()
	<-heartbeatDone
	if canceled {
		return
	} // leave lease for recovery, never complete an unowned job
	finishCtx, stop := context.WithTimeout(parent, 5*time.Second)
	defer stop()
	if err := w.Repository.Finish(finishCtx, j, workErr); err != nil {
		w.Logger.Warn("job completion not committed", "jobId", j.ID)
		return
	}
	w.Logger.Info("job finished", "jobId", j.ID, "kind", j.Kind, "success", workErr == nil, "error", workErr)
}

func queueName(q string) string {
	if q == "" {
		return "default"
	}
	return q
}
