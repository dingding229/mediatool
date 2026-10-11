package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"log/slog"
	"media-tool/internal/adapters/clouddrive2"
	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/messaging"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/modules/subscriptions"
	"media-tool/internal/platform/cache"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/httpapi"
	"media-tool/internal/platform/logging"
	"media-tool/internal/platform/schema"
	"media-tool/internal/platform/secrets"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
func run() error {
	queue := flag.String("queue", "default", "worker queue: default, transfer or library")
	role := flag.String("role", "all", "all (default), server, worker, healthcheck, or strm")
	cloudPath := flag.String("cloud-path", "", "CloudDrive2 full path for one STRM")
	relative := flag.String("strm-relative", "", "relative .strm output under STRM_OUTPUT_DIR")
	flag.Parse()
	if *role == "healthcheck" {
		return healthcheck()
	}
	c, err := config.Load()
	if err != nil {
		return err
	}
	if *role == "strm" {
		builder := strm.Builder{BaseURL: c.PublicURL, APIKey: c.STRMKey, AllowedRoots: c.STRMAllowedRoots}
		link, err := builder.Build(storage.FileRef{Path: *cloudPath})
		if err != nil {
			return err
		}
		if *relative == "" {
			return errors.New("-strm-relative is required")
		}
		if err = os.MkdirAll(c.STRMOutputDir, 0750); err != nil {
			return errors.New("STRM output directory unavailable")
		}
		if err = strm.WriteAtomic(c.STRMOutputDir, *relative, link); err != nil {
			return err
		}
		slog.Info("single STRM written; URL and playback credentials are not printed")
		return nil
	}
	logger, err := logging.New(os.Stdout, c.LogLevel, c.SessionKey, c.STRMKey, c.SecretKey, c.CloudDrivePassword, c.CloudDriveToken, c.DatabaseURL, c.RedisURL)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := database.Open(ctx, c.DatabaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	if db != nil {
		defer db.Close()
	}
	rc, err := cache.Open(c.RedisURL)
	if err != nil {
		return errors.New("invalid Redis configuration")
	}
	if rc != nil {
		defer rc.Close()
	}
	var manager *integrations.Manager
	if db != nil && c.SecretKey != "" {
		vault, vaultErr := secrets.New(c.SecretKey)
		if vaultErr != nil {
			return vaultErr
		}
		manager = integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, c)
		defer manager.Close()
	}
	switch *role {
	case "worker":
		return runWorker(ctx, c, db, manager, logger, *queue)
	case "server":
		return runServer(ctx, c, db, rc, manager, logger)
	case "all":
		if db == nil || rc == nil || manager == nil {
			return errors.New("unified runtime requires PostgreSQL, Redis and APP_SECRET_KEY")
		}
		migrateCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err = schema.Up(migrateCtx, c.DatabaseURL)
		cancel()
		if err != nil {
			return err
		}
		logger.Info("startup migrations complete")
		return supervise(ctx,
			func(ctx context.Context) error { return runServer(ctx, c, db, rc, manager, logger) },
			func(ctx context.Context) error { return runWorker(ctx, c, db, manager, logger, "default") },
			func(ctx context.Context) error { return runWorker(ctx, c, db, manager, logger, "transfer") },
			func(ctx context.Context) error { return runWorker(ctx, c, db, manager, logger, "library") },
		)

	default:
		return fmt.Errorf("unknown role %q", *role)
	}
}
func healthcheck() error {
	address := os.Getenv("APP_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("invalid APP_ADDR")
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/readyz")
	if err != nil {
		return errors.New("readiness check failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("service not ready")
	}
	return nil
}

// supervise cancels every component on any exit and joins them before shared
// database, cache and integration resources are closed.
func supervise(ctx context.Context, components ...func(context.Context) error) error {
	group, ctx := errgroup.WithContext(ctx)
	for _, component := range components {
		group.Go(func() error {
			err := component(ctx)
			if err == nil && ctx.Err() == nil {
				return errors.New("runtime component exited unexpectedly")
			}
			return err
		})
	}
	return group.Wait()
}

func runWorker(ctx context.Context, c config.Config, db *pgxpool.Pool, manager *integrations.Manager, logger *slog.Logger, queue string) error {
	w := jobs.Worker{Repository: jobs.Repository{DB: db}, Queue: queue, Owner: "worker-" + jobs.UUID(), PollInterval: c.WorkerPollInterval, Lease: c.WorkerLease, Logger: logger, Handlers: map[string]jobs.Handler{
		"system.check": func(ctx context.Context, j jobs.Job) error {
			if db == nil || db.Ping(ctx) != nil {
				return errors.New("PostgreSQL unavailable")
			}
			return nil
		},
	}}
	if manager != nil {
		engine := search.Engine{DB: db, Vault: manager.Repository.Vault, Connections: manager}
		w.Handlers["search.run"] = engine.Process
		subscriptionEngine := subscriptions.Engine{DB: db, Connections: manager}
		w.Handlers["subscription.evaluate"] = subscriptionEngine.Process
		libraryEngine := library.Engine{DB: db, Connections: manager}
		w.Handlers["library.scan"] = libraryEngine.Process
		w.Handlers["library.refresh"] = libraryEngine.Refresh
		downloader := acquisition.Engine{DB: db, Connections: manager, Search: engine}
		w.Handlers["download.add"] = downloader.Add
		w.Handlers["transfer.upload"] = downloader.Transfer
		w.Handlers["transfer.import.resume"] = downloader.RunOutputImport
		w.Handlers["emby.refresh"] = downloader.RefreshEmby
		w.Handlers["emby.confirm"] = downloader.ConfirmEmby
		if queue == "default" {
			watchCtx, watchCancel := context.WithCancel(ctx)
			watchDone := make(chan struct{})
			go func() { defer close(watchDone); libraryEngine.RunChangeWatcher(watchCtx, logger) }()
			defer func() { watchCancel(); <-watchDone }()
			schedulerDone := make(chan struct{})
			defer func() { watchCancel(); <-schedulerDone }()
			go func() {
				defer close(schedulerDone)
				ticker := time.NewTicker(15 * time.Second)
				defer ticker.Stop()
				for {
					check, cancel := context.WithTimeout(watchCtx, 2*time.Minute)
					if err := downloader.Reconcile(check); err != nil && ctx.Err() == nil {
						logger.Warn("download reconciliation unavailable")
					}
					if queue == "default" {
						subscriptionEngine.Schedule(check)
						libraryEngine.Schedule(check)
						libraryEngine.RecoverArtifacts(check)
						downloader.PrepareTransfers(check)
						dispatcher := messaging.Dispatcher{DB: db, Connections: manager}
						dispatcher.Prepare(check)
						dispatcher.Deliver(check)
					}
					cancel()
					select {
					case <-watchCtx.Done():
						return
					case <-ticker.C:
					}
				}
			}()
		}
	}
	logger.Info("worker starting", "queue", queue)
	return w.Run(ctx)

}

func runServer(ctx context.Context, c config.Config, db *pgxpool.Pool, rc *redis.Client, manager *integrations.Manager, logger *slog.Logger) error {
	var resolver storage.PlaybackResolver
	if manager != nil {
		resolver = integrations.DynamicStorage{Manager: manager}
	} else if c.CloudDriveEndpoint != "" {
		adapter, openErr := clouddrive2.New(clouddrive2.Options{Endpoint: c.CloudDriveEndpoint, Token: c.CloudDriveToken, Username: c.CloudDriveUsername, Password: c.CloudDrivePassword, CAFile: c.CloudDriveCAFile, RequestTimeout: c.CloudDriveRequestTimeout, MaxListFiles: c.CloudDriveMaxListFiles})
		if openErr != nil {
			return openErr
		}
		defer adapter.Close()
		resolver = adapter
	}
	handler := httpapi.New(c, httpapi.Dependencies{Integrations: manager, DB: db, Redis: rc, Storage: resolver, Logger: logger})
	server := &http.Server{Addr: c.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	listener, err := net.Listen("tcp", c.Address)
	if err != nil {
		return errors.New("cannot bind APP_ADDR")
	}
	done := make(chan error, 2)
	var gatewayServer *http.Server
	if c.EmbyGatewayAddress != "" {
		gatewayListener, err := net.Listen("tcp", c.EmbyGatewayAddress)
		if err != nil {
			listener.Close()
			return errors.New("cannot bind EMBY_GATEWAY_ADDR")
		}
		gatewayServer = &http.Server{Handler: httpapi.EmbyHandler(handler), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
		go func() {
			logger.Info("Emby gateway listener started", "address", c.EmbyGatewayAddress)
			done <- gatewayServer.Serve(gatewayListener)
		}()
	}

	go func() {
		logger.Info("HTTP server started", "address", c.Address, "environment", c.Environment)
		done <- server.Serve(listener)
	}()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if gatewayServer != nil {
			if gatewayServer.Shutdown(shutdown) != nil {
				gatewayServer.Close()
			}
		}
		if server.Shutdown(shutdown) != nil {
			server.Close()
		}
	}()
	select {
	case err = <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP server stopped unexpectedly")
		}
		return errors.New("HTTP server closed unexpectedly")
	case <-ctx.Done():
	}
	return nil

}
