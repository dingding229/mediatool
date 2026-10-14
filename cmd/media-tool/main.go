package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
	"io"
	"log/slog"
	"media-tool/internal/adapters/clouddrive2"
	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/filemanager"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/messaging"
	"media-tool/internal/modules/plugins"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/sitemetrics"
	"media-tool/internal/modules/sites"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/modules/subscriptions"
	"media-tool/internal/platform/cache"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/httpapi"
	"media-tool/internal/platform/logging"
	"media-tool/internal/platform/ownerrecovery"
	"media-tool/internal/platform/pluginhost"
	"media-tool/internal/platform/scheduling"
	"media-tool/internal/platform/schema"
	"media-tool/internal/platform/secrets"
	"media-tool/internal/platform/trace"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger, _ := logging.New(os.Stdout, "info")
	slog.SetDefault(logger)
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
func run() error {
	queue := flag.String("queue", "default", "worker queue: default, transfer or library")
	role := flag.String("role", "all", "all (default), server, worker, healthcheck, strm, or account-info")
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
	// Owner-only local CLI query uses the same configured official authorization,
	// without opening a second web session or mutating administrator credentials.
	if *role == "account-info" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		db, err := database.Open(ctx, c.DatabaseURL)
		if err != nil || db == nil {
			return errors.New("official account query requires configured PostgreSQL")
		}
		defer db.Close()
		vault, err := secrets.New(c.SecretKey)
		if err != nil {
			return err
		}
		manager := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, c)
		defer manager.Close()
		out, _, err := manager.OfficialAccountInfo(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	logStore, err := logging.Open(c.LogDir, c.LogLevel)
	if err != nil {
		return err
	}
	defer logStore.Close()
	logger, err := logging.NewRawControlled(io.MultiWriter(os.Stdout, logStore), c.LogLevel, &logStore.Level)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)
	logger.Debug("runtime configuration validated", "role", *role)
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
		return runWorker(ctx, c, db, manager, logger, *queue, rc)
	case "server":
		return runServer(ctx, c, db, rc, manager, logger, logStore)
	case "all":
		if db == nil || rc == nil || manager == nil {
			return errors.New("unified runtime requires PostgreSQL, Redis and APP_SECRET_KEY")
		}
		migrateCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err = schema.Up(migrateCtx, c.DatabaseURL)
		if err == nil {
			err = (sites.Repository{DB: db}).RegisterGoTemplates(migrateCtx)
		}
		cancel()
		if err != nil {
			return err
		}
		if err = filemanager.RegisterLocal(ctx, manager, c.LocalBrowseRoots); err != nil {
			return errors.New("automatic local roots unavailable")
		}
		logger.Info("startup migrations complete")
		recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 30*time.Second)
		_, err = ownerrecovery.Apply(recoveryCtx, db, c.PasswordResetFile, os.Stdout)
		recoveryCancel()
		if err != nil {
			return err
		}
		return supervise(ctx,
			func(ctx context.Context) error { return runServer(ctx, c, db, rc, manager, logger, logStore) },
			func(ctx context.Context) error { return runWorker(ctx, c, db, manager, logger, "default", rc) },
			func(ctx context.Context) error { return runWorker(ctx, c, db, manager, logger, "transfer", rc) },
			func(ctx context.Context) error { return runWorker(ctx, c, db, manager, logger, "library", rc) },
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

func runWorker(ctx context.Context, c config.Config, db *pgxpool.Pool, manager *integrations.Manager, logger *slog.Logger, queue string, rc *redis.Client) error {
	logger = logger.With("component", "worker", "queue", queue)
	w := jobs.Worker{Repository: jobs.Repository{DB: db}, Queue: queue, Owner: "worker-" + jobs.UUID(), PollInterval: c.WorkerPollInterval, Lease: c.WorkerLease, Logger: logger, Handlers: map[string]jobs.Handler{
		"system.check": func(ctx context.Context, j jobs.Job) error {
			if db == nil || db.Ping(ctx) != nil {
				return errors.New("PostgreSQL unavailable")
			}
			return nil
		},
	}}
	if manager != nil {
		engine := search.Engine{DB: db, Vault: manager.Repository.Vault, Connections: manager, Redis: rc}
		w.Handlers["search.run"] = engine.Process
		pluginRegistry := plugins.Registry{DB: db, Vault: manager.Repository.Vault, Host: pluginhost.New(db, manager)}
		w.Handlers["plugin.service"] = pluginRegistry.ProcessService
		w.Handlers["plugin.dispatch"] = pluginRegistry.Process
		metricEngine := sitemetrics.Engine{DB: db, Connections: manager}
		w.Handlers["site.metrics.refresh"] = metricEngine.Process
		subscriptionEngine := subscriptions.Engine{DB: db, Connections: manager}
		w.Handlers["subscription.evaluate"] = subscriptionEngine.Process
		libraryEngine := library.Engine{DB: db, Connections: manager}
		w.Handlers["library.scan"] = libraryEngine.Process
		w.Handlers["library.strm.repair"] = libraryEngine.ProcessSTRMRepair
		fileEngine := filemanager.Engine{DB: db, Connections: manager}
		w.Handlers["files.batch"] = fileEngine.ProcessBatch
		w.Handlers["library.refresh"] = libraryEngine.Refresh
		w.Handlers["library.emby.confirm"] = libraryEngine.CheckEmby
		downloader := acquisition.Engine{DB: db, Connections: manager, Search: engine}
		w.Handlers["download.add"] = downloader.Add
		w.Handlers["download.cleanup"] = downloader.Cleanup
		w.Handlers["transfer.upload"] = downloader.Transfer
		w.Handlers["transfer.import.resume"] = downloader.RunOutputImport
		w.Handlers["emby.refresh"] = downloader.RefreshEmby
		w.Handlers["emby.confirm"] = downloader.ConfirmEmby
		if queue == "default" {
			watchCtx, watchCancel := context.WithCancel(ctx)
			watchDone := make(chan struct{})
			go func() { defer close(watchDone); libraryEngine.RunChangeWatcher(watchCtx, logger) }()
			defer func() { watchCancel(); <-watchDone }()
			cronLogger := scheduling.Logger{Log: logger.With("module", "scheduler")}
			scheduler := cron.New(cron.WithSeconds(), cron.WithLogger(cronLogger), cron.WithChain(cron.Recover(cronLogger), cron.SkipIfStillRunning(cronLogger)))
			_, scheduleErr := scheduler.AddFunc("*/15 * * * * *", func() {
				logger.Debug("scheduler reconciliation cycle")
				check, cancel := context.WithTimeout(watchCtx, 2*time.Minute)
				defer cancel()
				check = trace.With(check, logger)
				if err := downloader.Reconcile(check); err != nil && ctx.Err() == nil {
					logger.Warn("download reconciliation unavailable")
				}
				if queue == "default" {
					if err := downloader.ScheduleCleanup(check); err != nil {
						logger.Warn("cleanup scheduling unavailable", "error", err)
					}
					if err := metricEngine.Schedule(check); err != nil {
						logger.Warn("site metrics schedule unavailable", "error", err)
					}
					if err := pluginRegistry.Schedule(check); err != nil {
						logger.Warn("plugin schedule unavailable", "error", err)
					}
					if err := pluginRegistry.DispatchEvents(check); err != nil {
						logger.Warn("plugin events unavailable", "error", err)
					}
					if err := subscriptionEngine.Schedule(check); err != nil {
						logger.Warn("subscription schedule unavailable", "error", err)
					}
					if err := libraryEngine.ScheduleEmby(check); err != nil {
						logger.Warn("automatic Emby refresh unavailable", "error", err)
					}
					if err := libraryEngine.Schedule(check); err != nil {
						logger.Warn("library schedule unavailable", "error", err)
					}
					if err := libraryEngine.RecoverArtifacts(check); err != nil {
						logger.Warn("artifact recovery unavailable", "error", err)
					}
					if err := downloader.PrepareTransfers(check); err != nil {
						logger.Warn("transfer preparation unavailable", "error", err)
					}
					dispatcher := messaging.Dispatcher{DB: db, Connections: manager}
					if err := dispatcher.Prepare(check); err != nil {
						logger.Warn("notification preparation unavailable", "error", err)
					}
					if err := dispatcher.Deliver(check); err != nil {
						logger.Warn("notification delivery unavailable", "error", err)
					}
				}
				cancel()
			})
			if scheduleErr != nil {
				return scheduleErr
			}
			scheduler.Start()
			defer func() { <-scheduler.Stop().Done() }()

		}
	}
	logger.Info("worker starting", "queue", queue)
	return w.Run(ctx)

}

func runServer(ctx context.Context, c config.Config, db *pgxpool.Pool, rc *redis.Client, manager *integrations.Manager, logger *slog.Logger, logStore *logging.Store) error {
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
	handler := httpapi.New(c, httpapi.Dependencies{Integrations: manager, DB: db, Redis: rc, Storage: resolver, Logger: logger, Logs: logStore})
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
