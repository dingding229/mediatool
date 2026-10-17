package stillrunner

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/ffx"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/mediaprobe/stillqueue"
	"media-tool/internal/modules/mediaprobe/stillstore"
	"media-tool/internal/modules/nativelibrary"
)

type Service struct {
	Repo        *stillqueue.Repository
	manager     *integrations.Manager
	logger      *slog.Logger
	root        string
	mu          sync.Mutex
	cache       *stillstore.Cache
	maintenance sync.Mutex
	cursor      int64
}

// New performs the mandatory startup budget check but no file/network I/O.
func New(m *integrations.Manager, logger *slog.Logger, root string, maxBytes int64) (*Service, error) {
	if m == nil || m.Repository.DB == nil || logger == nil {
		return nil, stillqueue.ErrInvalid
	}
	r, err := stillqueue.New(m.Repository.DB, stillqueue.Config{ResolveTimeout: m.Config.PlaybackResolveTimeout, FrameTimeout: m.Config.NativeStillsFrameTimeout, MaxBytes: maxBytes})
	if err != nil {
		return nil, err
	}
	if root == "" {
		root = stillstore.DefaultRoot
	}
	return &Service{Repo: r, manager: m, logger: logger, root: root}, nil
}
func (s *Service) files() (*stillstore.Cache, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil {
		store, err := stillstore.Open(s.root)
		if err != nil {
			return nil, err
		}
		cache, err := stillstore.NewCache(store, s.Repo)
		if err != nil {
			return nil, errors.Join(err, store.Close())
		}
		s.cache = cache
	}
	return s.cache, nil
}
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache != nil {
		return s.cache.Close()
	}
	return nil
}
func (s *Service) Publish(ctx context.Context, l stillqueue.Lease, raw []byte) (stillqueue.Image, error) {
	c, err := s.files()
	if err != nil {
		return stillqueue.Image{}, &stillstore.RetryError{Reason: stillqueue.StorageUnavailable, After: time.Minute}
	}
	return c.Publish(ctx, l, raw)
}

// Called only after the image resolver has checked current public exposure.
func (s *Service) ReadStill(ctx context.Context, id string) ([]byte, string, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n < 100000 {
		return nil, "", stillstore.ErrNotFound
	}
	snapshot, err := s.Repo.Current(ctx, n)
	if err != nil {
		return nil, "", err
	}
	if snapshot == nil {
		if _, err = s.Repo.RequeueEvicted(ctx, n); err != nil && !errors.Is(err, stillqueue.ErrDisabled) {
			return nil, "", err
		}
		return nil, "", stillstore.ErrNotFound
	}
	c, err := s.files()
	if err != nil {
		return nil, "", stillstore.ErrNotFound
	}
	raw, err := c.Read(ctx, *snapshot)
	if err != nil {
		if _, e := s.Repo.Calibrate(ctx, *snapshot, 0); e != nil {
			return nil, "", e
		}
		return nil, "", stillstore.ErrNotFound
	}
	tag := nativelibrary.StillImageTag(snapshot.Fingerprint, snapshot.Digest)
	if tag == "" {
		return nil, "", stillstore.ErrNotFound
	}
	return raw, tag, nil
}
func (s *Service) RetireStill(ctx context.Context, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return stillqueue.ErrInvalid
	}
	snapshot, err := s.Repo.Current(ctx, n)
	if err != nil || snapshot == nil {
		return err
	}
	_, err = s.Repo.Calibrate(ctx, *snapshot, 0)
	return err
}
func (s *Service) Maintain(ctx context.Context) error {
	if !s.maintenance.TryLock() {
		return nil
	}
	defer s.maintenance.Unlock()
	next, count, err := s.Repo.ScheduleBatch(ctx, s.cursor)
	s.cursor = next
	if err != nil {
		return err
	}
	if count < 200 {
		s.cursor = 0
	}
	if _, err = s.Repo.RetireSnapshots(ctx); err != nil {
		return err
	}
	on, err := s.Repo.Enabled(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	c := s.cache
	s.mu.Unlock()
	if c == nil && on {
		c, err = s.files()
		if err != nil {
			return err
		}
	}
	if c != nil {
		if err = c.Flush(ctx); err != nil {
			return err
		}
		_, err = c.GC(ctx, 200)
	}
	return err
}

type liveLinks struct{ source *linksource.Source }

func (l liveLinks) Reserve(ctx context.Context) (context.Context, func(), error) {
	return linksource.Reserve(ctx)
}
func (l liveLinks) Fetch(ctx context.Context, id string) (Ticket, error) {
	return l.source.Fetch(ctx, id)
}

type liveFrames struct{ runner *ffx.Runner }

func (l liveFrames) Reserve(ctx context.Context) (context.Context, func(), error) {
	return ffx.ReserveStill(ctx)
}
func (l liveFrames) Still(ctx context.Context, url string, at time.Duration, o FrameOptions) ([]byte, error) {
	return l.runner.Still(ctx, url, at, ffx.Options{UserAgent: o.UserAgent, Timeout: o.Timeout, Width: o.Width, Height: o.Height, Quality: o.Quality})
}
func (s *Service) Run(ctx context.Context) error {
	source, err := linksource.NewForManager(s.manager, linksource.Config{}, s.logger)
	if err != nil {
		return err
	}
	runner, err := ffx.New(ffx.Config{MaxOutputBytes: int(stillqueue.MaxImageBytes), StillTimeout: s.Repo.Limits().FrameTimeout})
	if err != nil {
		return err
	}
	engine := Engine{Store: s.Repo, Links: liveLinks{source}, Frames: liveFrames{runner}, Files: s, Config: s.Repo.Limits(), Logger: s.logger}
	group, ctx := errgroup.WithContext(ctx)
	worker := jobs.Worker{Repository: jobs.Repository{DB: s.manager.Repository.DB}, Queue: "still", Owner: "native-still-" + jobs.UUID(), PollInterval: s.manager.Config.WorkerPollInterval, Lease: max(s.manager.Config.WorkerLease, s.Repo.Limits().Lease+time.Minute), Logger: s.logger, Handlers: map[string]jobs.Handler{"native.still": engine.Process}}
	group.Go(func() error { return worker.Run(ctx) })
	group.Go(func() error { return s.MaintainLoop(ctx) })
	return group.Wait()
}

func (s *Service) MaintainLoop(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		check, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.Maintain(check)
		cancel()
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("Native still maintenance deferred")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Collect runs one bounded sweep using the same cursor/lock as maintenance.
func (s *Service) Collect(ctx context.Context) (stillstore.GCReport, error) {
	c, err := s.files()
	if err != nil {
		return stillstore.GCReport{}, stillstore.ErrUnavailable
	}
	if err = c.Flush(ctx); err != nil {
		return stillstore.GCReport{}, err
	}
	return c.GC(ctx, 200)
}
