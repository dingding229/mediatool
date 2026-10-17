package probequeue

import (
	"context"
	"errors"
	"golang.org/x/sync/errgroup"
	"log/slog"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/ffx"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/mediaprobe/stillqueue"
)

type liveLinks struct{ source *linksource.Source }

func (l liveLinks) Reserve(ctx context.Context) (context.Context, func(), error) {
	return linksource.Reserve(ctx)
}
func (l liveLinks) Fetch(ctx context.Context, id string) (Ticket, error) {
	return l.source.Fetch(ctx, id)
}

type liveProbe struct{ runner *ffx.Runner }

func (l liveProbe) Reserve(ctx context.Context) (context.Context, func(), error) {
	return ffx.ReserveProbe(ctx)
}
func (l liveProbe) Probe(ctx context.Context, url string, o ProbeOptions) ([]byte, error) {
	return l.runner.Probe(ctx, url, ffx.Options{UserAgent: o.UserAgent, Timeout: o.Timeout})
}

// Run starts exactly two wake consumers in the existing app, no new service.
// Constructors do no network/process I/O; disabled startup is safe without ffx.
func Run(ctx context.Context, m *integrations.Manager, logger *slog.Logger) error {
	if m == nil || m.Repository.DB == nil || logger == nil {
		return ErrInvalid
	}
	source, err := linksource.NewForManager(m, linksource.Config{}, logger)
	if err != nil {
		return err
	}
	runner, err := ffx.New(ffx.Config{})
	if err != nil {
		return err
	}
	engine := Engine{Store: Repository{DB: m.Repository.DB}, Links: liveLinks{source}, Probe: liveProbe{runner}, ResolveTimeout: m.Config.PlaybackResolveTimeout, Logger: logger}
	stills, err := stillqueue.New(m.Repository.DB, stillqueue.Config{ResolveTimeout: m.Config.PlaybackResolveTimeout, FrameTimeout: m.Config.NativeStillsFrameTimeout})
	if err != nil {
		return err
	}
	engine.AfterReady = func(ctx context.Context, asset string) error {
		_, err := stills.EnqueueAsset(ctx, asset)
		if errors.Is(err, stillqueue.ErrDisabled) {
			return nil
		}
		return err
	}
	group, ctx := errgroup.WithContext(ctx)
	for range 2 {
		worker := jobs.Worker{Repository: jobs.Repository{DB: m.Repository.DB}, Queue: "probe", Owner: "native-probe-" + jobs.UUID(), PollInterval: m.Config.WorkerPollInterval, Lease: m.Config.WorkerLease, Logger: logger, Handlers: map[string]jobs.Handler{"native.probe": engine.Process}}
		group.Go(func() error { return worker.Run(ctx) })
	}
	return group.Wait()
}
