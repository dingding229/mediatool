package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/emby"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/linkcache"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/modules/nativeplayback"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/modules/system"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/buildinfo"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpjson"
	"media-tool/internal/platform/logging"
	"media-tool/internal/platform/requestbudget"
	"media-tool/internal/platform/trace"
	"media-tool/internal/platform/webui"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type SessionReader interface {
	Session(context.Context, []byte) (accounts.Account, error)
}
type Dependencies struct {
	PluginInstances   PluginInstances
	NativeSessions    NativeSessionReader
	NativePlayback    nativeplayback.Service
	NativeProcessing  NativeProcessingService
	NativeStills      StillReader
	NativeLibraries   NativeLibraryReader
	NativeTMDBRefresh NativeTMDBRefresher
	NativeProbeRetry  NativeProbeRetrier
	Logs              *logging.Store
	AuthSessions      SessionReader
	PlayerAccess      PlayerAdmission
	Integrations      *integrations.Manager
	DB                *pgxpool.Pool
	Redis             *redis.Client
	Storage           storage.PlaybackResolver
	Logger            *slog.Logger
	Playback          *playback.Engine
}
type API struct {
	Config         config.Config
	Deps           Dependencies
	Auth           auth.Auth
	Builder        strm.Builder
	Engine         *playback.Engine
	Gateway        *emby.Gateway
	EmbyIdentities emby.IdentityStore
}
type Envelope struct {
	Data  any       `json:"data,omitempty"`
	Error *APIError `json:"error,omitempty"`
}
type APIError struct{ Code, Message string }

func (e APIError) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"code": e.Code, "message": e.Message})
}
func New(c config.Config, d Dependencies) http.Handler {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	a := &API{Config: c, Deps: d, Auth: auth.Auth{Key: c.SessionKey, Origin: c.PublicURL, AllowedOrigins: c.AuthAllowedOrigins, Secure: c.Environment == "production"}, Builder: strm.Builder{BaseURL: c.PublicURL, APIKey: c.STRMKey, AllowedRoots: c.STRMAllowedRoots}}
	a.EmbyIdentities = emby.NewMemoryIdentityStore()
	if d.Redis != nil {
		a.EmbyIdentities = &emby.RedisIdentityStore{Redis: d.Redis}
	}
	a.Engine = d.Playback
	if a.Engine == nil {
		blocked := []string{}
		for _, raw := range []string{c.PublicURL, c.EmbyUpstream, c.CloudDriveEndpoint} {
			parsed, err := url.Parse(raw)
			if err == nil && parsed.Hostname() != "" {
				blocked = append(blocked, parsed.Hostname())
			}
		}
		resolver := d.Storage
		if d.Redis != nil && d.Storage != nil {
			resolver = &linkcache.Cached{Source: d.Storage, Redis: d.Redis, Version: a.playbackCacheVersion, ResolveTimeout: c.PlaybackResolveTimeout}
		}
		a.Engine = playback.New(resolver, playback.Options{Policy: playback.Policy{BlockedHosts: blocked, AllowedHosts: c.PlaybackAllowedHosts, AllowHTTP: c.PlaybackAllowHTTP}, ProxyFallback: c.STRMProxyFallback, ForceProxy: c.STRMForceProxy, ResolveTimeout: c.PlaybackResolveTimeout})
	}
	if c.EmbyUpstream != "" {
		target, _ := url.Parse(c.EmbyUpstream)
		a.Gateway = emby.NewGateway(emby.GatewayOptions{Identities: a.EmbyIdentities, Upstream: target, PublicOrigin: embyPublicOrigin(c.EmbyPublicURL), Enable302: c.Emby302Enabled, DirectPlayOnly: c.EmbyDirectPlayOnly, LegacyEnabled: c.LegacyPlaybackEnabled, OwnedAsset: a.ownedAsset, Builder: a.Builder, STRMOutputDir: c.STRMOutputDir, STRMPathPrefix: c.EmbySTRMPathPrefix, ResolverOrigins: c.EmbyResolverOrigins, Playback: a.Engine})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media-server/native/sessions", a.protected(a.nativeSessions))
	mux.HandleFunc("GET /api/v1/media/emby/items/{id}", a.protected(a.catalogItem))
	mux.HandleFunc("GET /api/v1/media/tmdb/image", a.protected(a.tmdbPoster))
	mux.HandleFunc("GET /api/v1/media/library/emby/home", a.protected(a.catalogHome))
	mux.HandleFunc("/api/v1/plugin/{instance}/{path...}", a.pluginGateway)
	mux.HandleFunc("GET /api/v1/plugins/{id}/tools", a.protected(a.pluginTools))
	mux.HandleFunc("POST /api/v1/plugins/{id}/tools/call", a.protected(a.pluginToolCall))
	mux.HandleFunc("POST /api/v1/plugins/agent/chat", a.protected(a.pluginAgentUnsupported))
	mux.HandleFunc("POST /api/v1/plugins/agent/run", a.protected(a.pluginAgentUnsupported))
	mux.HandleFunc("GET /api/v1/plugins/{id}/auth/status", a.protected(a.pluginAuthStatus))
	mux.HandleFunc("GET /api/v1/plugins/workflows", a.protected(a.pluginWorkflowRead))
	mux.HandleFunc("POST /api/v1/plugins/workflows", a.protected(a.pluginWorkflow))
	mux.HandleFunc("POST /api/v1/plugins/auth/bindings", a.protected(a.pluginAuthBinding))
	mux.HandleFunc("POST /api/v1/plugins/auth/begin", a.protected(a.pluginAuthUnsupported))
	mux.HandleFunc("GET /api/v1/plugins/ui-surfaces", a.protected(a.pluginUISurfaces))
	mux.HandleFunc("GET /api/v1/plugins/{id}/ui/{kind}", a.protected(a.pluginUI))
	mux.HandleFunc("GET /api/v1/plugins/{id}/state-snapshots", a.protected(a.pluginStateList))
	mux.HandleFunc("POST /api/v1/plugins/{id}/state-snapshots", a.protected(a.pluginStateOperation))
	mux.HandleFunc("GET /api/v1/plugins/{id}/review", a.protected(a.pluginReviewPlan))
	mux.HandleFunc("GET /api/v1/plugins/{id}/extensions", a.protected(a.pluginExtensions))
	mux.HandleFunc("GET /api/v1/plugins/{id}/runs", a.protected(a.pluginRuns))
	mux.HandleFunc("POST /api/v1/plugins/{id}/run", a.protected(a.pluginRunExtension))
	mux.HandleFunc("POST /api/v1/plugins/{id}/api-keys", a.protected(a.pluginKeyCreate))
	mux.HandleFunc("POST /api/v1/plugins/{id}/api-keys/{key}/revoke", a.protected(a.pluginKeyRevoke))
	mux.HandleFunc("POST /api/v1/plugins/{id}/events/replay", a.protected(a.pluginReplay))
	mux.HandleFunc("GET /api/v1/plugins/resources", a.protected(a.pluginV2Resources))
	mux.HandleFunc("GET /api/v1/plugins/catalog", a.protected(a.pluginV2Catalog))
	mux.HandleFunc("POST /api/v1/plugins/operations", a.protected(a.pluginV2Operation))
	mux.HandleFunc("GET /api/v1/plugins/{id}", a.protected(a.pluginV2Inspect))
	mux.HandleFunc("GET /api/v1/plugins", a.protected(a.pluginV2List))
	mux.HandleFunc("GET /api/v1/media/organize-history", a.protected(a.organizeHistory))
	mux.HandleFunc("GET /api/v1/cache/{kind}", a.protected(a.viewCache))
	mux.HandleFunc("GET /api/v1/cache/{kind}/{id}", a.protected(a.cacheDetail))
	mux.HandleFunc("DELETE /api/v1/cache/{kind}/{id}", a.protected(a.deleteCache))
	mux.HandleFunc("POST /api/v1/cache/{kind}/clear", a.protected(a.clearCache))
	mux.HandleFunc("GET /api/v1/metadata/image/{size}/{file}", a.tmdbImage)
	mux.HandleFunc("GET /api/v1/metadata/details", a.protected(a.mediaDetails))
	mux.HandleFunc("GET /api/v1/search/media", a.protected(a.tmdbMediaSearch))
	mux.HandleFunc("GET /api/v1/settings/access", a.protected(a.accessSettings))
	mux.HandleFunc("PUT /api/v1/settings/access", a.protected(a.accessSettings))
	mux.HandleFunc("GET /api/v1/settings/application", a.protected(a.getApplicationSettings))
	mux.HandleFunc("PUT /api/v1/settings/application", a.protected(a.saveApplicationSettings))
	mux.HandleFunc("POST /api/v1/media/recognize-name", a.protected(a.parseReleaseName))
	mux.HandleFunc("GET /api/v1/logs/summary", a.protected(a.logsSummary))
	mux.HandleFunc("GET /api/v1/logs/query", a.protected(a.queryLogs))
	mux.HandleFunc("GET /api/v1/logs", a.protected(a.readLogs))
	mux.HandleFunc("POST /api/v1/logs/clear", a.protected(a.clearLogs))
	mux.HandleFunc("POST /api/v1/logs/level", a.protected(a.setLogLevel))
	mux.HandleFunc("GET /api/v1/data/postgres/tables", a.protected(a.dataTables))
	mux.HandleFunc("GET /api/v1/data/postgres/tables/{table}", a.protected(a.dataRows))
	mux.HandleFunc("GET /api/v1/data/redis/keys", a.protected(a.redisKeys))
	mux.HandleFunc("GET /api/v1/data/redis/value", a.protected(a.redisValue))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "alive"}) })
	mux.HandleFunc("GET /readyz", a.ready)
	mux.HandleFunc("GET /api/v1/auth/setup", a.authSetup)
	mux.HandleFunc("POST /api/v1/auth/register", a.register)
	mux.HandleFunc("POST /api/v1/auth/password", a.protected(a.changeAccountPassword))
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/auth/logout", a.protected(a.logout))
	mux.HandleFunc("GET /api/v1/auth/session", a.protected(a.accountSession))
	mux.HandleFunc("GET /api/v1/classification", a.protected(a.getClassification))
	mux.HandleFunc("PUT /api/v1/classification", a.protected(a.saveClassification))
	mux.HandleFunc("POST /api/v1/classification/preview", a.protected(a.previewClassification))
	mux.HandleFunc("GET /api/v1/system/status", a.protected(a.status))
	mux.HandleFunc("GET /api/v1/system/modules", a.protected(func(w http.ResponseWriter, r *http.Request) { write(w, 200, system.Modules()) }))
	mux.HandleFunc("GET /api/v1/integrations", a.protected(a.listIntegrations))
	mux.HandleFunc("POST /api/v1/integrations", a.protected(a.saveIntegration))
	mux.HandleFunc("PUT /api/v1/integrations/{id}", a.protected(a.saveIntegration))
	mux.HandleFunc("DELETE /api/v1/integrations/{id}", a.protected(a.deleteIntegration))
	mux.HandleFunc("POST /api/v1/integrations/{id}/secrets/reveal", a.protected(a.revealIntegrationSecrets))
	mux.HandleFunc("POST /api/v1/integrations/{id}/test", a.protected(a.testIntegration))
	mux.HandleFunc("POST /api/v1/integrations/{id}/115/auth/start", a.protected(a.start115Authorization))
	mux.HandleFunc("POST /api/v1/integrations/{id}/115/auth/poll", a.protected(a.poll115Authorization))
	mux.HandleFunc("POST /api/v1/integrations/{id}/115/auth/cancel", a.protected(a.cancel115Authorization))
	mux.HandleFunc("GET /api/v1/metadata/search", a.protected(a.metadataSearch))
	mux.HandleFunc("GET /api/v1/metadata/episodes", a.protected(a.metadataEpisodes))
	mux.HandleFunc("POST /api/v1/messaging/command-actions", a.protected(a.issueCommandAction))
	mux.HandleFunc("GET /api/v1/media/library/sources", a.protected(a.catalogSources))
	mux.HandleFunc("GET /api/v1/media/library", a.protected(a.listCatalog))
	mux.HandleFunc("GET /api/v1/media/library/emby/{id}/image", a.protected(a.catalogImage))
	mux.HandleFunc("POST /api/v1/files/batches/preview", a.protected(a.previewFileBatch))
	mux.HandleFunc("GET /api/v1/files/batches/{id}", a.protected(a.getFileBatch))
	mux.HandleFunc("POST /api/v1/files/batches/{id}/approve", a.protected(a.approveFileBatch))
	mux.HandleFunc("POST /api/v1/files/batches/{id}/reconcile", a.protected(a.reconcileFileBatch))
	mux.HandleFunc("POST /api/v1/settings/naming-preview", a.protected(a.namingPreview))
	mux.HandleFunc("GET /api/v1/cloud/accounts/info", a.protected(a.cloudAccountInfo))
	mux.HandleFunc("POST /api/v1/library/activity/remove", a.protected(a.removeLibraryHistory))
	mux.HandleFunc("GET /api/v1/library/activity", a.protected(a.libraryActivity))
	mux.HandleFunc("GET /api/v1/library/outputs/{id}/content", a.protected(a.managedOutputContent))
	mux.HandleFunc("GET /api/v1/library/outputs", a.protected(a.managedOutputs))
	mux.HandleFunc("POST /api/v1/library/outputs/{id}/emby", a.protected(a.checkManagedEmby))
	mux.HandleFunc("GET /api/v1/library/scans/{id}", a.protected(a.scanDetails))
	mux.HandleFunc("POST /api/v1/library/scans/{id}/{action}", a.protected(a.controlScan))
	mux.HandleFunc("GET /api/v1/files/roots", a.protected(a.listFileRoots))
	mux.HandleFunc("GET /api/v1/files", a.protected(a.listFiles))
	mux.HandleFunc("GET /api/v1/files/operations", a.protected(a.listFileOperations))
	mux.HandleFunc("POST /api/v1/files/preview", a.protected(a.previewFileOperation))
	mux.HandleFunc("POST /api/v1/files/operations/{id}/execute", a.protected(a.executeFileOperation))
	mux.HandleFunc("POST /api/v1/files/operations/{id}/reconcile", a.protected(a.reconcileFileOperation))
	mux.HandleFunc("GET /api/v1/jobs", a.protected(a.listJobs))
	mux.HandleFunc("POST /api/v1/jobs", a.protected(a.createJob))
	mux.HandleFunc("GET /api/v1/jobs/browse", a.protected(a.browseJobs))
	mux.HandleFunc("GET /api/v1/jobs/{id}", a.protected(a.jobDetail))
	mux.HandleFunc("GET /api/v1/system/operations", a.protected(a.operations))
	mux.HandleFunc("GET /api/v1/jobs/events", a.protected(a.events))
	mux.HandleFunc("POST /api/v1/strm/preview", a.protected(a.preview))
	mux.HandleFunc("GET /api/v1/diagnostics/playback/preflight", a.protected(a.playbackPreflight))
	mux.HandleFunc("GET /api/v1/media/assets/{id}/validation-ref", a.protected(a.assetValidationRef))
	mux.HandleFunc("GET /api/v1/media/assets/{id}/strm", a.protected(a.assetSTRM))
	mux.HandleFunc("POST /api/v1/cloudStorage/test", a.protected(a.testStorage))
	mux.HandleFunc("POST /api/v1/cloudStorage/diagnose", a.protected(a.diagnoseStorage))
	mux.HandleFunc("POST /api/v1/emby/test", a.protected(a.testEmby))
	mux.HandleFunc("POST /api/v1/emby/check", a.protected(a.checkEmbyItem))
	mux.HandleFunc("GET "+strm.Endpoint, a.resolve)
	mux.HandleFunc("GET /api/v1/sites", a.protected(a.listDefinitions))
	mux.HandleFunc("GET /api/v1/sites/metrics/schedule", a.protected(a.getSiteMetricSchedule))
	mux.HandleFunc("PUT /api/v1/sites/metrics/schedule", a.protected(a.saveSiteMetricSchedule))
	mux.HandleFunc("GET /api/v1/pt/templates/{id}/icon", a.ptTemplateIcon)
	mux.HandleFunc("GET /api/v1/sites/metrics", a.protected(a.siteMetrics))
	mux.HandleFunc("POST /api/v1/sites/metrics/sync", a.protected(a.syncSiteMetrics))
	mux.HandleFunc("GET /api/v1/pt/templates", a.protected(a.ptTemplates))
	mux.HandleFunc("POST /api/v1/pt/templates/load", a.protected(a.loadPTTemplates))
	mux.HandleFunc("GET /api/v1/pt/ua", a.protected(a.ptUA))
	mux.HandleFunc("GET /api/v1/pt/accounts", a.protected(a.ptAccounts))
	mux.HandleFunc("POST /api/v1/pt/accounts", a.protected(a.savePTAccount))
	mux.HandleFunc("PUT /api/v1/pt/accounts/{id}", a.protected(a.savePTAccount))
	mux.HandleFunc("GET /api/v1/sites/definitions", a.protected(a.listDefinitions))
	mux.HandleFunc("GET /api/v1/search", a.protected(a.listSearches))
	mux.HandleFunc("POST /api/v1/search", a.protected(a.startSearch))
	mux.HandleFunc("POST /api/v1/search/{id}/media", a.protected(a.saveSearchMedia))
	mux.HandleFunc("GET /api/v1/subscriptions/{id}/history", a.protected(a.subscriptionHistory))
	mux.HandleFunc("GET /api/v1/settings/network-proxy", a.protected(a.networkProxy))
	mux.HandleFunc("PUT /api/v1/settings/network-proxy", a.protected(a.networkProxy))
	mux.HandleFunc("GET /api/v1/system/runtime", a.protected(a.runtimeSummary))
	mux.HandleFunc("GET /api/v1/search/{id}", a.protected(a.getSearch))
	mux.HandleFunc("POST /api/v1/downloads/manual/{id}/retry", a.protected(a.retryManualDownload))
	mux.HandleFunc("POST /api/v1/downloads/manual/{id}/inspect", a.protected(a.reconcileManualDownload))
	mux.HandleFunc("GET /api/v1/downloads/manual", a.protected(a.listManualDownloads))
	mux.HandleFunc("POST /api/v1/downloads/manual", a.protected(a.createManualDownload))
	mux.HandleFunc("GET /api/v1/downloads", a.protected(a.listDownloads))
	mux.HandleFunc("POST /api/v1/downloads", a.protected(a.startDownload))
	mux.HandleFunc("POST /api/v1/downloads/{id}/cleanup/reconcile", a.protected(a.reconcileDownloadCleanup))
	mux.HandleFunc("POST /api/v1/downloads/{id}/cleanup", a.protected(a.approveDownloadCleanup))
	mux.HandleFunc("POST /api/v1/downloads/sync", a.protected(a.syncDownloads))
	mux.HandleFunc("GET /api/v1/downloads/{id}/source-preview", a.protected(a.downloadSourcePreview))
	mux.HandleFunc("POST /api/v1/downloads/{id}/reconfirm-source", a.protected(a.reconfirmDownloadSource))
	mux.HandleFunc("POST /api/v1/pipeline/{kind}/{id}/retry", a.protected(a.retryPipeline))
	mux.HandleFunc("POST /api/v1/transfers/{id}/control", a.protected(a.controlUpload))
	mux.HandleFunc("GET /api/v1/transfers/{id}/sessions", a.protected(a.transferSessions))
	mux.HandleFunc("GET /api/v1/transfers", a.protected(a.listTransfers))
	mux.HandleFunc("GET /api/v1/transfers/{id}/output-preview", a.protected(a.previewOutputResume))
	mux.HandleFunc("POST /api/v1/transfers/{id}/resume-import", a.protected(a.resumeOutputImport))
	mux.HandleFunc("POST /api/v1/media/assets/{id}/identity", a.protected(a.correctAssetIdentity))
	mux.HandleFunc("POST /api/v1/media/assets/{id}/identity-preview", a.protected(a.previewAssetIdentity))
	mux.HandleFunc("POST /api/v1/library/organize-selection", a.protected(a.libraryOrganizeSelection))
	mux.HandleFunc("GET /api/v1/media/assets", a.protected(a.listAssets))
	mux.HandleFunc("POST /api/v1/media/assets/{id}/grant/revoke", a.protected(a.revokeGrant))
	mux.HandleFunc("POST /api/v1/media/assets/{id}/grant/rotate", a.protected(a.rotateGrant))
	mux.HandleFunc("GET /api/v1/media", a.protected(a.listMedia))
	mux.HandleFunc("POST /api/v1/notification-deliveries/{id}/retry", a.protected(a.retryNotification))
	mux.HandleFunc("GET /api/v1/notifications", a.protected(a.listNotifications))
	mux.HandleFunc("GET /api/v1/subscriptions", a.protected(a.listSubscriptions))
	mux.HandleFunc("POST /api/v1/subscriptions", a.protected(a.saveSubscription))
	mux.HandleFunc("PUT /api/v1/subscriptions/{id}", a.protected(a.saveSubscription))
	mux.HandleFunc("DELETE /api/v1/subscriptions/{id}", a.protected(a.deleteSubscription))
	mux.HandleFunc("POST /api/v1/subscriptions/{id}/evaluate", a.protected(a.queueSubscription))
	mux.HandleFunc("GET /api/v1/subscription-runs/{id}", a.protected(a.getSubscriptionRun))
	mux.HandleFunc("POST /api/v1/library/scans", a.protected(a.scanLibrary))
	mux.HandleFunc("GET /api/v1/library/artifact-repairs", a.protected(a.listArtifactRepairs))
	mux.HandleFunc("GET /api/v1/library/archives", a.protected(a.listArchives))
	mux.HandleFunc("GET /api/v1/library/archive-policy", a.protected(a.archivePolicy))
	mux.HandleFunc("POST /api/v1/library/archive-policy", a.protected(a.saveArchivePolicy))
	mux.HandleFunc("POST /api/v1/library/archives/cleanup-preview", a.protected(a.previewArchiveCleanup))
	mux.HandleFunc("POST /api/v1/library/archives/cleanup-confirm", a.protected(a.confirmArchiveCleanup))
	mux.HandleFunc("GET /api/v1/library/archive-cleanups/{id}", a.protected(a.archiveCleanupResults))
	mux.HandleFunc("POST /api/v1/library/archive-cleanups/{id}/retry", a.protected(a.retryArchiveCleanup))
	mux.HandleFunc("POST /api/v1/library/artifact-repairs/{id}/retry", a.protected(a.retryArtifactRepair))
	mux.HandleFunc("GET /api/v1/library/scan-schedules", a.protected(a.listScanSchedules))
	mux.HandleFunc("POST /api/v1/library/scan-schedules", a.protected(a.saveScanSchedule))
	mux.HandleFunc("POST /api/v1/library/candidates/batch-confirm", a.protected(a.batchConfirmCandidates))
	mux.HandleFunc("GET /api/v1/library/scans", a.protected(a.listLibraryScans))
	mux.HandleFunc("GET /api/v1/library/candidates", a.protected(a.listLibraryCandidates))
	mux.HandleFunc("POST /api/v1/library/candidates/{id}/recognize", a.protected(a.recognizeCandidate))
	mux.HandleFunc("POST /api/v1/library/candidates/{id}/confirm", a.protected(a.confirmLibraryCandidate))
	for _, module := range []string{"cloudStorage"} {
		mux.HandleFunc("/api/v1/"+module, a.protected(notImplemented))
		mux.HandleFunc("/api/v1/"+module+"/", a.protected(notImplemented))
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "not_found", "接口不存在") })
	a.registerMaintenance(mux)
	a.registerMediaServer(mux)
	a.registerPlayerAccess(mux)
	a.registerSTRMTools(mux)
	a.registerExternal(mux)
	ui := webui.Handler()
	mux.HandleFunc("GET /watch", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("GET /watch/{rest...}", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	embyProxy := longStream(http.HandlerFunc(a.proxyEmby))
	mux.Handle("/emby/", embyProxy)
	mux.Handle("/emby", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !emby.IsRootOrigin(r.Context()) && (r.Method == "GET" || r.Method == "HEAD") {
			// Vue history-mode hard refresh belongs to the management SPA.
			// The dedicated Emby listener still proxies its actual server root.
			ui.ServeHTTP(w, r)
			return
		}
		embyProxy.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			fail(w, 405, "method_not_allowed", "不支持此方法")
			return
		}
		ui.ServeHTTP(w, r)
	})
	return a.middleware(mux)
}
func (a *API) protected(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account, err := a.sessionAccount(r)
		if err != nil {
			fail(w, 401, "unauthorized", "请使用管理员账号密码登录")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && !a.sameOrigin(r) {
			fail(w, 403, "csrf_rejected", "请求来源或防跨站请求标识无效")
			return
		}
		if account.MustChangePassword && r.URL.Path != "/api/v1/auth/password" && r.URL.Path != "/api/v1/auth/logout" && r.URL.Path != "/api/v1/auth/session" {
			fail(w, 403, "password_change_required", "请先重置临时密码，再访问工作台")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && !strings.HasPrefix(r.URL.Path, "/api/v1/ops/") && r.URL.Path != "/api/v1/auth/logout" {
			if paused, err := maintenance.Paused(r.Context(), a.Deps.DB); err != nil {
				fail(w, 503, "maintenance_unavailable", "维护状态不可读取")
				return
			} else if paused {
				fail(w, 423, "maintenance_paused", "当前处于恢复/回滚维护窗口，业务写入暂停")
				return
			}
		}
		next(w, r)
	}
}
func (a *API) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := make([]byte, 12)
		if _, checkedErr := rand.Read(nonce); checkedErr != nil {
			deferutil.Report(checkedErr, "internal/platform/httpapi/api.go:")
			return
		}
		id := hex.EncodeToString(nonce)
		w.Header().Set("X-Request-ID", id)
		r = r.WithContext(context.WithValue(r.Context(), playerVisitKey{}, &playerVisit{}))
		r = r.WithContext(trace.With(r.Context(), a.Deps.Logger.With("requestId", id)))
		r = r.WithContext(storage.WithResolveObserver(r.Context(), func(event storage.ResolveObservation) {
			fields := []any{"module", "playback", "requestId", id, "stage", string(event.Stage), "outcome", event.Outcome, "durationMs", event.DurationMillis}
			if event.ProviderCode != 0 {
				fields = append(fields, "providerCode", event.ProviderCode)
			}
			if event.HTTPStatus != 0 {
				fields = append(fields, "providerHTTPStatus", event.HTTPStatus)
			}
			if trace := event.HTTPTrace; trace != nil {
				fields = append(fields, "dnsMs", trace.DNSMillis, "tcpMs", trace.TCPMillis, "tlsMs", trace.TLSMillis, "firstByteMs", trace.FirstByteMillis, "httpTotalMs", trace.TotalMillis, "httpAttempt", trace.Attempt, "connectionReused", trace.ConnectionReused, "httpResponseReceived", trace.ResponseReceived)
			}
			a.Deps.Logger.Info("Playback resolution stage", fields...)
		}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if !strings.HasPrefix(r.URL.Path, "/emby") {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		}
		// Preserve Flusher/Hijacker/ReaderFrom and their exact availability;
		// a naive status-only wrapper can break SSE, media and WebSockets.
		guarded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					if recovered == http.ErrAbortHandler {
						// ReverseProxy uses this sentinel after a partial body fails.
						// Do not append a JSON error to an MPEG-TS/SSE/media stream.
						fields := []any{"method", r.Method, "path", r.URL.Path, "requestId", id}
						if reason := w.Header().Get("X-Media-Gateway-Reason"); trustedDeliveryLabel(reason) {
							fields = append(fields, "X-Media-Gateway-Reason", reason)
						}
						a.Deps.Logger.Info("HTTP stream aborted", fields...)
						panic(http.ErrAbortHandler)
					}
					a.Deps.Logger.Error("HTTP panic recovered", "requestId", id)
					fail(w, 500, "internal_error", "服务内部错误")
				}
			}()
			if a.admitPlayer(w, r) {
				next.ServeHTTP(w, r)
			}
		})
		metrics := httpsnoop.CaptureMetrics(guarded, w, r)
		if visit, ok := r.Context().Value(playerVisitKey{}).(*playerVisit); ok && visit.ID != "" {
			finish, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
			if err := a.playerAccessRepo().FinishEvent(finish, visit.ID, visit.UserID, visit.UserName, visit.Verified, metrics.Code); err != nil {
				a.Deps.Logger.Warn("playback event finalization unavailable", "eventId", visit.ID)
			}
			cancel()
		}

		// Owner explicitly requested raw request and redirect URLs in private logs.
		// Login bodies, Cookie and Authorization headers are still not collected.
		module := "httpapi"
		if strings.Contains(r.URL.Path, "strm302") || strings.Contains(r.URL.Path, "PlaybackInfo") || strings.Contains(strings.ToLower(r.URL.Path), "/videos/") {
			module = "playback"
		}
		requestURL := *r.URL
		if requestURL.Host == "" {
			requestURL.Host = r.Host
			requestURL.Scheme = "http"
			if r.TLS != nil {
				requestURL.Scheme = "https"
			}
		}

		fields := []any{"module", module, "url", requestURL.String(), "userAgent", r.UserAgent(), "range", r.Header.Get("Range"), "location", w.Header().Get("Location"), "method", r.Method, "path", r.URL.Path, "status", metrics.Code, "responseBytes", metrics.Written, "durationMs", metrics.Duration.Milliseconds(), "requestId", id}
		// These headers are fixed server enums. No provider URLs, raw queries,
		// client credentials or client-supplied UA enter access logs.
		for _, key := range []string{"X-Media-Gateway", "X-Media-Gateway-Reason", "X-Media-Delivery", "X-Media-Delivery-Reason"} {
			if value := w.Header().Get(key); trustedDeliveryLabel(value) {
				fields = append(fields, key, value)
			}
		}
		a.Deps.Logger.Info("HTTP request", fields...)
	})
}
func write(w http.ResponseWriter, status int, data any) {
	if checkedErr := httpjson.Write(w, status, Envelope{Data: data}); checkedErr != nil {
		deferutil.Report(checkedErr, "internal/platform/httpapi/api.go:write")
		return
	}
}
func fail(w http.ResponseWriter, status int, code, message string) {
	if checkedErr := httpjson.Write(w, status, Envelope{Error: &APIError{Code: code, Message: message}}); checkedErr != nil {
		deferutil.Report(checkedErr, "internal/platform/httpapi/api.go:fail")
		return
	}
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	return decodeLimit(w, r, target, 16<<10)
}
func decodeLimit(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		fail(w, 400, "invalid_json", "JSON 请求格式无效或包含不支持的字段")
		return false
	}
	var rest any
	if err := decoder.Decode(&rest); !errors.Is(err, io.EOF) {
		fail(w, 400, "invalid_json", "只允许一个 JSON 对象")
		return false
	}
	return true
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.revokeSession(r); err != nil {
		fail(w, 503, "session_revoke_failed", "会话撤销未提交，请稍后重试")
		return
	}
	a.Auth.ClearCookie(w)
	write(w, 200, map[string]bool{"authenticated": false})
}

type DependencyStatus struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Message string `json:"message"`
}

func (a *API) dependencies(ctx context.Context) []DependencyStatus {
	statuses := []DependencyStatus{
		{"postgres", "PostgreSQL", "not_configured", "未配置 DATABASE_URL；持久化任务暂不可用"},
		{"redis", "Redis", "not_configured", "未配置 REDIS_URL；后续可重建缓存使用此连接"},
		{"clouddrive2", "CloudDrive2 / 115", "not_configured", "未配置 CloudDrive2；请填写地址和接入凭据"},
		{"emby", "Emby", "disabled", "未配置或启用 Emby 连接"},
	}
	var wg sync.WaitGroup
	if a.Deps.DB != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			check, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if a.Deps.DB.Ping(check) != nil {
				statuses[0].State = "unavailable"
				statuses[0].Message = "数据库不可达，请检查连接与网络"
			} else {
				statuses[0].State = "ready"
				statuses[0].Message = "数据库连接可用"
			}
		}()
	}
	if a.Deps.Redis != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			check, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if a.Deps.Redis.Ping(check).Err() != nil {
				statuses[1].State = "unavailable"
				statuses[1].Message = "Redis 不可达，请检查连接与网络"
			} else {
				statuses[1].State = "ready"
				statuses[1].Message = "缓存连接可用；直链按文件、配置版本及请求上下文隔离"
			}
		}()
	}
	if inspector, ok := a.Deps.Storage.(storage.Inspector); ok && a.storageConfigured(ctx) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			_, err := inspector.Probe(bounded, a.builder(ctx).AllowedRoots[0])
			if err != nil {
				statuses[2].State = "unavailable"
				statuses[2].Message = cloudMessage(err)
			} else {
				statuses[2].State = "ready"
				statuses[2].Message = "CloudDrive2 已就绪，令牌可读取配置根目录；不代表真实播放器验收已通过"
			}
		}()
	} else if a.Config.CloudDriveEndpoint != "" {
		statuses[2].State = "unavailable"
		statuses[2].Message = "存储客户端未初始化"
	}
	if gateway := a.gateway(ctx); gateway != nil {
		statuses[3].State = "configured"
		statuses[3].Message = "Emby反代已配置，真实客户端播放待验收"
		if gateway.RedirectEnabled() {
			statuses[3].Message = "静态流302网关已配置，真实客户端播放待验收"
		}
	}
	wg.Wait()
	return statuses
}
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	deps := a.dependencies(r.Context())
	ready := deps[0].State == "ready" || (a.Config.Environment == "development" && a.Deps.DB == nil)
	// Redis is cache, not the durable queue: report degradation but do not block.
	if !ready {
		fail(w, 503, "not_ready", "主数据库未就绪")
		return
	}
	write(w, 200, map[string]string{"status": "ready"})
}
func (a *API) status(w http.ResponseWriter, r *http.Request) {
	write(w, 200, map[string]any{
		"name": "Media Tool", "version": buildinfo.Version, "environment": a.Config.Environment, "dependencies": a.dependencies(r.Context()), "publicUrl": a.access(r.Context()).ManagementURL, "memberUrl": a.access(r.Context()).MemberURL, "playbackUrl": a.access(r.Context()).PlaybackURL, "strmEndpoint": strm.Endpoint, "allowedRoots": a.builder(r.Context()).AllowedRoots, "phase": "业务接入开发", "databaseConfigured": a.Deps.DB != nil,
		"worker": a.workerStatus(r.Context()), "integrationsAvailable": a.Deps.Integrations != nil, "storageConfigured": a.storageConfigured(r.Context()), "proxyFallback": a.Config.STRMProxyFallback, "playbackResolveTimeoutMs": a.Engine.ResolveBudget().Milliseconds(), "forceProxy": a.Config.STRMForceProxy, "legacyPlaybackEnabled": a.Config.LegacyPlaybackEnabled, "emby302Enabled": a.Config.Emby302Enabled, "embyDirectPlayOnly": a.directPlayOnly(r.Context()),
	})
}
func (a *API) preview(w http.ResponseWriter, r *http.Request) {
	var ref storage.FileRef
	if !decode(w, r, &ref) {
		return
	}
	link, err := a.builder(r.Context()).Build(ref)
	if err != nil {
		fail(w, 400, "invalid_file_ref", err.Error())
		return
	}
	write(w, 200, map[string]string{"url": link, "content": link + "\n", "warning": "链接包含播放专用密钥；实际播放需 CloudDrive2 连接与文件诊断通过"})
}
func (a *API) resolve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		fail(w, 400, "invalid_query", "查询参数无效")
		return
	}
	if len(q["apiKey"]) != 1 {
		fail(w, 401, "invalid_playback_key", "播放凭据无效")
		return
	}
	for k, v := range q {
		if (k != "apiKey" && k != "path" && k != "pickCode" && k != "assetId") || len(v) != 1 {
			fail(w, 400, "invalid_query", "参数未知或重复")
			return
		}
	}
	ref := storage.FileRef{Path: q.Get("path"), PickCode: q.Get("pickCode"), AssetID: q.Get("assetId")}
	if ref.AssetID != "" {
		if a.Deps.Integrations == nil {
			fail(w, 401, "invalid_playback_key", "播放凭据无效")
			return
		}
		grant, err := a.grants().Authorize(r.Context(), ref.AssetID, q.Get("apiKey"))
		if err != nil {
			fail(w, 401, "invalid_playback_key", "文件播放凭据无效、撤销或文件缺失")
			return
		}
		ref.Path = grant.CloudPath
	} else if !a.Config.LegacyPlaybackEnabled || !auth.Match(a.Config.STRMKey, q.Get("apiKey")) {
		fail(w, 401, "invalid_playback_key", "播放凭据无效")
		return
	}
	builder := a.builder(r.Context())
	if ref.AssetID != "" {
		builder.AllowedRoots = []string{"/"}
	}
	if err = builder.Validate(ref); err != nil {
		fail(w, 400, "invalid_file_ref", err.Error())
		return
	}
	if !a.storageConfigured(r.Context()) {
		fail(w, 503, "storage_not_configured", "请配置并启用 CloudDrive2 连接")
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(requestbudget.FixedSTRMWrite(a.Engine.ResolveBudget())))
	if a.directPlayOnly(r.Context()) {
		a.Engine.ServeRedirect(w, r, ref)
	} else {
		a.Engine.Serve(w, r, ref)
	}
}
func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	if a.Deps.DB == nil {
		fail(w, 503, "database_not_configured", "配置 PostgreSQL 并执行迁移后可查询任务")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	list, err := (jobs.Repository{DB: a.Deps.DB}).List(ctx)
	if err != nil {
		fail(w, 503, "jobs_unavailable", "任务表不可用，请检查数据库和迁移状态")
		return
	}
	write(w, 200, list)
}
func (a *API) createJob(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind string `json:"kind"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Kind != "system.check" {
		fail(w, 400, "unsupported_job", "运维入口仅支持只读系统检查；下载、上传等任务请在对应业务模块创建")
		return
	}
	if a.Deps.DB == nil {
		fail(w, 503, "database_not_configured", "请先配置 PostgreSQL 并执行迁移")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	j, err := (jobs.Repository{DB: a.Deps.DB}).Create(ctx, body.Kind, json.RawMessage(`{}`))
	if err != nil {
		fail(w, 503, "jobs_unavailable", "任务创建失败，请检查数据库和迁移状态")
		return
	}
	write(w, 201, j)
}
func (a *API) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	if checkedErr := controller.SetWriteDeadline(time.Time{}); checkedErr != nil && !errors.Is(checkedErr, http.ErrNotSupported) {
		deferutil.Report(checkedErr, "internal/platform/httpapi/api.go:events")
		return
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		// The SSE stream is an invalidation hint, not a durable event bus. Reconnect
		// then GET /jobs to reconcile current PostgreSQL state.
		if !a.authorized(r) {
			return
		}
		if _, err := io.WriteString(w, "event: tick\ndata: {}\n\n"); err != nil {
			return
		}
		if err := controller.Flush(); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
func notImplemented(w http.ResponseWriter, r *http.Request) {
	fail(w, 501, "not_implemented", "模块接口已预留，业务适配将在后续里程碑实现")
}

func longStream(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if checkedErr := http.NewResponseController(w).SetWriteDeadline(time.Time{}); checkedErr != nil && !errors.Is(checkedErr, http.ErrNotSupported) {
			deferutil.Report(checkedErr, "internal/platform/httpapi/api.go:")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func trustedDeliveryLabel(value string) bool {
	switch value {
	case "emby-static", "emby-upstream", "authorized_static", "identity_or_permission_unconfirmed", "transcoding_parameters", "non_static", "source_selection", "unmanaged_source", "container_mismatch", "redirect", "proxy", "forced_proxy", "provider_headers_required", "provider_user_agent_required", "non_https_source", "direct_conditions_met", "unavailable", "direct_validation_failed", "request_cancelled", "upstream_timeout", "upstream_closed", "upstream_unavailable":
		return true
	}
	return false
}
