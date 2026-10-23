package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"media-tool/internal/adapters/clouddrive2"
	"media-tool/internal/appcli"
	"media-tool/internal/modules/acquisition"
	sourcecatalog "media-tool/internal/modules/catalog"
	"media-tool/internal/modules/filemanager"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/ipgeo"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/modules/manualdownloads"
	probeadmin "media-tool/internal/modules/mediaprobe/admin"
	"media-tool/internal/modules/mediaprobe/probequeue"
	"media-tool/internal/modules/mediaprobe/stillrunner"
	"media-tool/internal/modules/messaging"
	"media-tool/internal/modules/nativelibrary/catalog"
	playbackruntime "media-tool/internal/modules/nativeplayback/runtime"
	"media-tool/internal/modules/nativetmdb"
	"media-tool/internal/modules/networkproxy"
	"media-tool/internal/modules/onlineupdate"
	"media-tool/internal/modules/playbackstats"
	pluginbusiness "media-tool/internal/modules/plugins/business"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/modules/plugins/v2/sandbox"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/settings"
	"media-tool/internal/modules/sitemetrics"
	"media-tool/internal/modules/sites"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/modules/subscriptions"
	"media-tool/internal/modules/updatecheck"
	"media-tool/internal/platform/buildinfo"
	"media-tool/internal/platform/cache"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/httpapi"
	"media-tool/internal/platform/logging"
	"media-tool/internal/platform/ownerrecovery"
	"media-tool/internal/platform/safeerrors"
	"media-tool/internal/platform/scheduling"
	"media-tool/internal/platform/schema"
	"media-tool/internal/platform/secrets"
	"media-tool/internal/platform/trace"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
)

func main() {
	if handled, e := appcli.Dispatch(os.Args[1:]); handled {
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(2)
		}
		return
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "server", "worker", "healthcheck", "strm", "account-info":
			os.Args = append([]string{os.Args[0], "-role", os.Args[1]}, os.Args[2:]...)
		case "plugin-worker":
		default:
			if !strings.HasPrefix(os.Args[1], "-") {
				fmt.Fprintln(os.Stderr, "unknown command; see docs/CLI_REFERENCE.md")
				os.Exit(2)
			}
		}
	}

	if err := sandbox.Parent(); err != nil {
		fmt.Fprintln(os.Stderr, "process protection unavailable")
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "plugin-worker" {
		if err := pluginv2.Worker(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "plugin worker unavailable")
			os.Exit(1)
		}
		return
	}
	logger, _ := logging.New(os.Stdout, "info")
	slog.SetDefault(logger)
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
func run() error {
	queue := flag.String("queue", "default", "worker queue: default, transfer, library, metadata, maintenance or probe")
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
	defer deferutil.Close(logStore, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	logger, err := logging.NewRawControlled(io.MultiWriter(os.Stdout, logStore), c.LogLevel, &logStore.Level)
	if err != nil {
		return err
	}
	logStore.ApplyPolicy(logging.DefaultPolicy(), time.Now())
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
	// Join the log policy service before closing its DB/store dependencies.
	policyCtx, policyCancel := context.WithCancel(ctx)
	policyDone := make(chan struct{})
	go func() {
		defer close(policyDone)
		logStore.Maintain(policyCtx, func(ctx context.Context) (logging.Policy, error) {
			v, e := (settings.Repository{DB: db}).Get(ctx)
			return v.Value.Logging, e
		}, logger)
	}()
	defer func() { policyCancel(); <-policyDone }()
	rc, err := cache.Open(c.RedisURL)
	if err != nil {
		return errors.New("invalid Redis configuration")
	}
	if rc != nil {
		defer deferutil.Close(rc, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
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
	var stills *stillrunner.Service
	if manager != nil {
		stills, err = stillrunner.New(manager, logger, c.NativeStillsRoot, c.NativeStillsMaxBytes)
		if err != nil {
			return err
		}
		defer func() {
			if err := stills.Close(); err != nil {
				logger.Warn("Native still cache close deferred")
			}
		}()
	}
	switch *role {
	case "worker":
		if *queue == "still" {
			if stills == nil {
				return errors.New("native still worker requires manager")
			}
			return stills.Run(ctx)
		}
		if *queue == "probe" {
			return probequeue.Run(ctx, manager, logger)
		}
		return runWorker(ctx, c, db, manager, logger, *queue, rc)
	case "server":
		if stills != nil {
			return supervise(ctx, func(ctx context.Context) error { return runServer(ctx, c, db, rc, manager, logger, logStore, stills) }, stills.MaintainLoop)
		}
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
			func(ctx context.Context) error { return runServer(ctx, c, db, rc, manager, logger, logStore, stills) },
			func(ctx context.Context) error { return stills.Run(ctx) },
			func(ctx context.Context) error { return runWorker(ctx, c, db, manager, logger, "default", rc) },
			func(ctx context.Context) error { return runWorker(ctx, c, db, manager, logger, "transfer", rc) },
			func(ctx context.Context) error { return runWorker(ctx, c, db, manager, logger, "library", rc) },
			func(ctx context.Context) error { return runWorker(ctx, c, db, manager, logger, "maintenance", rc) },
			func(ctx context.Context) error { return runWorker(ctx, c, db, manager, logger, "metadata", rc) },
			func(ctx context.Context) error { return probequeue.Run(ctx, manager, logger) },
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
	defer deferutil.Close(resp.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
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
	if queue == "default" {
		pollCtx, cancelPoll := context.WithCancel(trace.With(ctx, logger))
		defer cancelPoll()
		pollDone := make(chan struct{})
		go func() {
			defer close(pollDone)
			_ = (messaging.CommandIngress{DB: db, Connections: manager, Plugins: pluginv2.Store{DB: db, Vault: manager.Repository.Vault}, Owner: jobs.UUID()}).Run(pollCtx)
		}()
		defer func() { cancelPoll(); <-pollDone }()
	}

	logger = logger.With("component", "worker", "queue", queue)
	w := jobs.Worker{Repository: jobs.Repository{DB: db}, Queue: queue, Owner: "worker-" + jobs.UUID(), PollInterval: c.WorkerPollInterval, Lease: c.WorkerLease, Logger: logger, Handlers: map[string]jobs.Handler{
		"system.check": func(ctx context.Context, j jobs.Job) error {
			if db == nil || db.Ping(ctx) != nil {
				return errors.New("PostgreSQL unavailable")
			}
			return nil
		},
	}}
	ops := maintenance.Engine{DB: db, Redis: rc, DSN: c.DatabaseURL, Root: c.MaintenanceDir, OutputRoot: c.STRMOutputDir, Key: c.SecretKey, HostProc: c.HostProcDir, Revision: buildinfo.Revision}
	if queue == "maintenance" {
		restoreCtx, restoreCancel := context.WithCancel(ctx)
		restoreDone := make(chan struct{})
		go func() { defer close(restoreDone); ops.ServeWebRestores(restoreCtx) }()
		defer func() { restoreCancel(); <-restoreDone }()
	}
	w.Handlers["ops.backup"] = func(ctx context.Context, j jobs.Job) error {
		var in struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(j.Payload, &in) != nil {
			return maintenance.ErrUnsafe
		}
		bounded, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		return ops.Backup(bounded, in.ID)
	}
	w.Handlers["ops.drill"] = func(ctx context.Context, j jobs.Job) error {
		var in struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(j.Payload, &in) != nil {
			return maintenance.ErrUnsafe
		}
		bounded, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		return ops.Drill(bounded, in.ID)
	}
	if manager != nil {
		engine := search.Engine{DB: db, Vault: manager.Repository.Vault, Connections: manager, Redis: rc}
		w.Handlers["search.run"] = engine.Process
		// Retired jobs are acknowledged, never retried/executed against archived WASM tables.
		w.Handlers["plugin.v2.workflow"] = (pluginv2.WorkflowRunner{Store: pluginv2.Store{DB: db, Vault: manager.Repository.Vault, Root: c.PluginDataRoot}}).Process
		w.Handlers["plugin.v2.service"] = (pluginv2.Store{DB: db, Vault: manager.Repository.Vault, Root: c.PluginDataRoot}).ProcessService
		w.Handlers["plugin.service"] = func(context.Context, jobs.Job) error { return nil }
		w.Handlers["plugin.dispatch"] = func(context.Context, jobs.Job) error { return nil }
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
		probeRepo := probequeue.Repository{DB: db}
		assignmentEngine := catalog.Engine{DB: db}
		detailsEngine := nativetmdb.Engine{DB: db, Connections: manager}
		w.Handlers["native.tmdb.fetch"] = detailsEngine.Process
		w.Handlers["native.assign"] = func(ctx context.Context, j jobs.Job) error {
			if err := assignmentEngine.Process(ctx, j); err != nil {
				return err
			}
			var in struct {
				ID string `json:"assetId"`
			}
			if err := json.Unmarshal(j.Payload, &in); err != nil {
				return err
			}
			if _, err := probeRepo.Enqueue(ctx, in.ID); err != nil && !errors.Is(err, probequeue.ErrDisabled) && !errors.Is(err, probequeue.ErrNotExposable) {
				logger.Warn("Native probe enqueue deferred", "assetId", in.ID)
			}
			language, err := detailsEngine.Language(ctx)
			if err == nil {
				err = (nativetmdb.Repository{DB: db}).EnqueueAsset(ctx, in.ID, language)
			}
			if err != nil {
				logger.Warn("TMDB详情稍后重试排队", "module", "native", "error", err)
			}
			return nil // Provider unavailability must not fail asset assignment.
		}
		w.Handlers["library.emby.confirm"] = libraryEngine.CheckEmby
		downloader := acquisition.Engine{DB: db, Connections: manager, Search: engine}
		w.Handlers["download.manual.import"] = (manualdownloads.Engine{DB: db, Connections: manager}).ProcessImport
		w.Handlers["download.manual.add"] = (manualdownloads.Engine{DB: db, Connections: manager}).Process
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
			var nativeIDCursor int64 // Only this non-overlapping cron callback owns the cursor.
			var nativeTMDBCursor string
			var nativeProbeCursor string
			_, scheduleErr := scheduler.AddFunc(scheduling.CoreReconcileCron, func() {
				logger.Debug("scheduler reconciliation cycle")
				check, cancel := context.WithTimeout(watchCtx, 2*time.Minute)
				defer cancel()
				check = trace.With(check, logger)
				release, gateErr := maintenance.BeginWork(check, db)
				if gateErr != nil {
					return
				}
				defer release()
				if err := ops.Schedule(check); err != nil {
					logger.Warn("maintenance scheduling failed", "module", "maintenance", "error", err)
				}
				paused, pauseErr := maintenance.Paused(check, db)
				if pauseErr != nil || paused {
					return
				}

				nextProbe, countProbe, probeErr := probeRepo.ScheduleBatch(check, nativeProbeCursor)
				nativeProbeCursor = nextProbe
				if probeErr != nil {
					logger.Warn("Native probe sweep deferred")
				} else if countProbe < 200 {
					nativeProbeCursor = ""
				}
				if language, err := detailsEngine.Language(check); err == nil {
					next, count, scheduleErr := (nativetmdb.Repository{DB: db}).ScheduleBatch(check, language, nativeTMDBCursor)
					nativeTMDBCursor = next
					if scheduleErr != nil {
						logger.Warn("TMDB详情调度暂不可用", "module", "native", "error", scheduleErr)
					} else if count < 200 {
						nativeTMDBCursor = ""
					}
				}
				if _, err := catalog.PreallocateIDBatch(check, db); err != nil {
					logger.Warn("native ID preallocation unavailable", "module", "native", "error", err)
				}
				if next, count, err := catalog.RefreshIDLifecycle(check, db, nativeIDCursor); err != nil {
					logger.Warn("native ID lifecycle unavailable", "module", "native", "error", err)
				} else if count < 500 {
					nativeIDCursor = 0
				} else {
					nativeIDCursor = next
				}
				if err := manager.TickOfficialHealth(check); err != nil {
					logger.Warn("115 health scheduling failed", "module", "integrations", "error", err)
				}
				if err := downloader.Reconcile(check); err != nil && ctx.Err() == nil {
					logger.Warn("download reconciliation unavailable")
				}
				if queue == "default" {
					if err := (manualdownloads.Engine{DB: db, Connections: manager}).Sync(check); err != nil {
						logger.Warn("manual download sync failed", "module", "downloads", "error", err)
					}
					if err := downloader.ScheduleCleanup(check); err != nil {
						logger.Warn("cleanup scheduling unavailable", "error", err)
					}
					if err := metricEngine.Schedule(check); err != nil {
						logger.Warn("site metrics schedule unavailable", "error", err)
					}
					if err := subscriptionEngine.Schedule(check); err != nil {
						logger.Warn("subscription schedule unavailable", "error", err)
					}
					if err := (subscriptions.CoverageObserver{Engine: subscriptionEngine, Cache: rc}).Reconcile(check); err != nil {
						logger.Warn("subscription coverage observation deferred", "module", "plugins")
					}
					if err := (sourcecatalog.SourceRegistry{Store: pluginv2.Store{DB: db, Vault: manager.Repository.Vault}, Cache: rc}).Refresh(check); err != nil {
						logger.Warn("plugin source cache refresh deferred", "module", "plugins")
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
					if err := (safeerrors.Bus{DB: db}).Flush(check); err != nil {
						logger.Warn("safe error aggregation deferred", "module", "system")
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

func runServer(ctx context.Context, c config.Config, db *pgxpool.Pool, rc *redis.Client, manager *integrations.Manager, logger *slog.Logger, logStore *logging.Store, native ...*stillrunner.Service) error {
	stats := playbackstats.New(db, logger)
	statsCtx, statsCancel := context.WithCancel(ctx)
	statsDone := make(chan struct{})
	go func() {
		defer close(statsDone)
		stats.Run(statsCtx, func(ctx context.Context) (int, error) {
			v, e := (settings.Repository{DB: db}).Get(ctx)
			return v.Value.PlaybackRetentionDays, e
		})
	}()
	defer func() { statsCancel(); <-statsDone }()

	if db != nil {
		if err := (catalog.Repository{DB: db}).SyncCurrent(ctx); err != nil {
			return fmt.Errorf("native library startup sync failed: %w", err)
		}
	}
	sessionCtx, stopSessions := context.WithCancel(ctx)
	defer stopSessions()
	if db != nil {
		go playbackruntime.MaintainSessions(sessionCtx, db, logger)
	}
	var resolver storage.PlaybackResolver
	if manager != nil {
		resolver = integrations.DynamicStorage{Manager: manager}
	} else if c.CloudDriveEndpoint != "" {
		adapter, openErr := clouddrive2.New(clouddrive2.Options{Endpoint: c.CloudDriveEndpoint, Token: c.CloudDriveToken, Username: c.CloudDriveUsername, Password: c.CloudDrivePassword, CAFile: c.CloudDriveCAFile, RequestTimeout: c.CloudDriveRequestTimeout, MaxListFiles: c.CloudDriveMaxListFiles})
		if openErr != nil {
			return openErr
		}
		defer deferutil.Close(adapter, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
		resolver = adapter
	}
	var geoVault *secrets.Vault
	if manager != nil {
		geoVault = manager.Repository.Vault
	}
	geo := ipgeo.New(db, geoVault, filepath.Join(filepath.Dir(c.PluginDataRoot), "ipgeo"))
	geoCtx, geoCancel := context.WithCancel(ctx)
	geoDone := make(chan struct{})
	go func() { defer close(geoDone); geo.Run(geoCtx, logger) }()
	defer func() { geoCancel(); <-geoDone }()
	updates := updatecheck.New(db, logger)
	updateCtx, stopUpdates := context.WithCancel(ctx)
	updatesDone := make(chan struct{})
	go func() { defer close(updatesDone); updates.Run(updateCtx) }()
	defer func() { stopUpdates(); <-updatesDone }()
	updater := onlineupdate.New(db, updates)
	go updater.Run(ctx)
	managementDeps := httpapi.Dependencies{OnlineUpdater: updater, UpdateChecker: updates, IPGeo: geo, Integrations: manager, DB: db, Redis: rc, Storage: resolver, Logger: logger, Logs: logStore}
	if db != nil && manager != nil {
		store := pluginv2.Store{DB: db, Vault: manager.Repository.Vault, Root: c.PluginDataRoot, Logger: logger}
		businessHost := pluginbusiness.AcquisitionHost{DB: db, Vault: manager.Repository.Vault, Connections: manager, Redis: rc, Network: networkproxy.NewHostClient(networkproxy.Repository{DB: db, Vault: manager.Repository.Vault})}
		if len(native) > 0 && native[0] != nil {
			businessHost.Processing = &probeadmin.Repository{DB: db, Stills: native[0].Repo, MaxBytes: c.NativeStillsMaxBytes}
			businessHost.Collector = native[0]
		}
		store.Business = businessHost
		if err := pluginv2.SelfCheck(); err != nil {
			return err
		}
		if err := store.Refresh(ctx); err != nil {
			return fmt.Errorf("plugin definition refresh: %w", err)
		}
		managementDeps.PluginInstances = store
		executable, e := os.Executable()
		if e != nil {
			return errors.New("plugin executable unavailable")
		}
		supervisor := pluginv2.Supervisor{Store: store, Executable: executable, Logger: logger, MaxTasks: c.PluginMaxTasks}
		finished := make(chan error, 1)
		go func() { finished <- runPluginSupervisor(sessionCtx, supervisor, db) }()
		defer func() {
			stopSessions()
			select {
			case e := <-finished:
				if e != nil {
					logger.Warn("Plugin supervisor stopped", "module", "plugins")
				}
			case <-time.After(35 * time.Second):
				logger.Warn("Plugin supervisor shutdown pending", "module", "plugins")
			}
		}()
	}
	if len(native) > 0 && native[0] != nil {
		managementDeps.NativeStills = native[0]
	}
	handler := httpapi.New(c, managementDeps)
	server := &http.Server{Addr: c.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	listener, err := net.Listen("tcp", c.Address)
	if err != nil {
		return errors.New("cannot bind APP_ADDR")
	}
	done := make(chan error, 3)
	var memberServer *http.Server
	if c.MemberAddress != "" {
		deps := httpapi.Dependencies{DB: db, Logger: logger, Integrations: manager, Redis: rc, PlaybackStats: stats}
		if len(native) > 0 {
			deps.NativeStills = native[0]
		}
		nativeHandler, err := httpapi.NativeMediaHandler(c, deps)
		if err != nil {
			if closeErr := listener.Close(); closeErr != nil {
				logger.Debug("Management listener close interrupted")
			}
			return errors.New("cannot initialize native media handler")
		}
		memberListener, err := net.Listen("tcp", c.MemberAddress)
		if err != nil {
			deferutil.Close(listener, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
			return errors.New("cannot bind MEMBER_ADDR")
		}
		memberServer = &http.Server{Handler: nativeHandler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
		go func() {
			logger.Info("Native Emby listener started", "address", c.MemberAddress)
			done <- memberServer.Serve(memberListener)
		}()
	}
	var gatewayServer *http.Server
	if c.EmbyGatewayAddress != "" {
		gatewayListener, err := net.Listen("tcp", c.EmbyGatewayAddress)
		if err != nil {
			deferutil.Close(listener, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
			if memberServer != nil {
				deferutil.Close(memberServer, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
			}
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
		if memberServer != nil {
			if memberServer.Shutdown(shutdown) != nil {
				deferutil.Close(memberServer, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
			}
		}
		if gatewayServer != nil {
			if gatewayServer.Shutdown(shutdown) != nil {
				deferutil.Close(gatewayServer, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
			}
		}
		if server.Shutdown(shutdown) != nil {
			deferutil.Close(server, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
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
