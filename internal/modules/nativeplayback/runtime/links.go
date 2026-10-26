package runtime

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"media-tool/internal/adapters/open115"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/linkcache"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/modules/nativeplayback"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/settings"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/modules/subtitles"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/requestbudget"
	"net/url"
	"os"
	"strings"
	"time"
)

type Resolver struct {
	DB      *pgxpool.Pool
	Config  config.Config
	Manager *integrations.Manager
	Source  storage.PlaybackResolver
	Logger  *slog.Logger
	Policy  playback.Policy
}

func New(db *pgxpool.Pool, c config.Config, m *integrations.Manager, cache *redis.Client, source storage.PlaybackResolver, logger *slog.Logger) *Service {
	if m != nil {
		source = integrations.DynamicStorage{Manager: m}
	}
	if source != nil {
		source = &limitedSource{source: source, lanes: make(chan struct{}, 4)}
	}
	if cache != nil && source != nil {
		version := func(context.Context, storage.FileRef) (string, error) { return "env", nil }
		if m != nil {
			version = m.PlaybackVersion
		}
		source = &linkcache.Cached{Source: source, Redis: cache, Version: version, ResolveTimeout: c.PlaybackResolveTimeout}
	}
	blocked := []string{}
	for _, raw := range []string{c.PublicURL, c.MemberPublicURL, c.CloudDriveEndpoint} {
		if u, e := url.Parse(raw); e == nil && u.Hostname() != "" {
			blocked = append(blocked, u.Hostname())
		}
	}
	return &Service{Subtitles: subtitles.Repository{DB: db, Root: subtitles.Root(c)}, DB: db, Browse: catalog.NewBrowseRepository(db, logger), Links: &Resolver{Logger: logger, DB: db, Config: c, Manager: m, Source: source, Policy: playback.Policy{BlockedHosts: blocked, AllowedHosts: c.PlaybackAllowedHosts, AllowHTTP: c.PlaybackAllowHTTP}}}
}

type evidence struct {
	Path, Pick, Output string
	Size, Revision     int64
	Digest             []byte
}

func (r *Resolver) load(ctx context.Context, id string) (e evidence, err error) {
	err = r.DB.QueryRow(ctx, `WITH eligible AS (`+catalog.ExposableAssetsSQL+`) SELECT f.full_path,coalesce(f.pick_code,''),s.output_path,f.size_bytes,e.revision,a.content_digest FROM eligible e JOIN cloud_files f ON f.id=e.cloud_file_id JOIN strm_entries s ON s.asset_id=e.asset_id AND s.output_path<>'' AND lower(s.output_path) LIKE '%.strm' LEFT JOIN managed_media_artifacts a ON a.asset_id=e.asset_id AND a.relative_path=s.output_path AND a.artifact_kind='strm' AND a.owner_scope='asset:'||e.asset_id::text WHERE e.asset_id=$1::uuid ORDER BY s.output_path LIMIT 1`, id).Scan(&e.Path, &e.Pick, &e.Output, &e.Size, &e.Revision, &e.Digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, nativeplayback.ErrNotFound
	}
	if err != nil {
		return e, nativeplayback.ErrUpstream
	}
	if len(e.Digest) != sha256.Size {
		return e, nativeplayback.ErrNotFound
	}
	return e, nil
}
func (r *Resolver) Resolve(parent context.Context, id, ua, ip string) (nativeplayback.RedirectTarget, error) {
	ctx, cancel := context.WithTimeout(parent, requestbudget.EffectiveResolve(r.Config.PlaybackResolveTimeout))
	defer cancel()
	if r.Logger != nil {
		ctx = storage.WithResolveObserver(ctx, func(o storage.ResolveObservation) {
			fields := []any{"module", "playback", "assetId", id, "stage", string(o.Stage), "result", o.Outcome, "durationMs", o.DurationMillis}
			if h := o.HTTPTrace; h != nil {
				fields = append(fields, "dnsMs", h.DNSMillis, "tcpMs", h.TCPMillis, "tlsMs", h.TLSMillis, "firstByteMs", h.FirstByteMillis, "httpTotalMs", h.TotalMillis, "attempt", h.Attempt, "connectionReused", h.ConnectionReused)
			}
			r.Logger.Info("Native playback resolve stage", fields...)
		})
	}
	e, err := r.load(ctx, id)
	if err != nil {
		return nativeplayback.RedirectTarget{}, err
	}
	body, err := strm.ReadOwned(ctx, r.Config.STRMOutputDir, e.Output)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, strm.ErrUnsafeBody) {
			return nativeplayback.RedirectTarget{}, nativeplayback.ErrNotFound
		}
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	digest := sha256.Sum256(body)
	if subtle.ConstantTimeCompare(digest[:], e.Digest) != 1 {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrNotFound
	}
	origin, err := settings.PlaybackOrigin(ctx, r.DB, r.Config.PublicURL)
	if err != nil {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	builder := strm.Builder{BaseURL: origin, APIKey: r.Config.STRMKey, AllowedRoots: r.Config.STRMAllowedRoots}
	opts := strm.ParseOptions{Builder: builder, Origins: []string{r.Config.PublicURL}, LegacyEnabled: r.Config.LegacyPlaybackEnabled}
	if r.Manager != nil {
		profile, e := r.Manager.Repository.Default(ctx, "clouddrive2")
		if e == nil {
			if root := strings.TrimRight(profile.String("cloudRoot"), "/"); root != "" {
				opts.Builder.AllowedRoots = []string{root}
			}
		} else if !errors.Is(e, integrations.ErrNotFound) {
			return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
		}
		opts.ManagedURL = (playbackgrants.Service{DB: r.DB, Vault: r.Manager.Repository.Vault, PublicURL: r.Config.PublicURL}).VerifyManagedURL
	}
	raw := strings.TrimSpace(string(body))
	ref, err := strm.ParseStable(ctx, raw, opts)
	if err != nil {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	stable, err := url.Parse(raw)
	if err != nil || ref.Path != e.Path || stable.Query().Get("path") != e.Path || ref.AssetID != "" && ref.AssetID != id || stable.Query().Get("assetId") != "" && stable.Query().Get("assetId") != id || stable.Query().Get("pickCode") != "" && stable.Query().Get("pickCode") != e.Pick {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrNotFound
	}
	ref.AssetID, ref.Path, ref.PickCode, ref.ExpectedSize, ref.UserAgent, ref.ClientIP = id, e.Path, e.Pick, e.Size, ua, ip
	if r.Source == nil {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	// DynamicStorage -> official115 retains R1-F's one bounded, no-response path
	// retry and existing process limiter/circuit policy. No outer retry is added.
	link, err := r.Source.Resolve(ctx, ref)
	if err != nil {
		var limited *nativeplayback.RetryError
		if errors.As(err, &limited) {
			return nativeplayback.RedirectTarget{}, limited
		}
		if errors.Is(err, open115.ErrRateLimited) {
			return nativeplayback.RedirectTarget{}, &nativeplayback.RetryError{After: time.Minute}
		}
		var api *open115.APIError
		if errors.As(err, &api) && api.RateLimited() {
			return nativeplayback.RedirectTarget{}, &nativeplayback.RetryError{After: time.Minute}
		}
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	if len(link.URL) > 8192 || len(link.Headers) > 0 || link.UserAgent != "" && link.UserAgent != ua || !link.ExpiresAt.IsZero() && time.Until(link.ExpiresAt) < 5*time.Second || !link.CacheUntil.IsZero() && time.Until(link.CacheUntil) < 5*time.Second {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	target, err := url.Parse(link.URL)
	if err != nil || strings.EqualFold(target.Hostname(), stable.Hostname()) {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	if err = r.Policy.Check(ctx, link.URL); err != nil {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrUpstream
	}
	// Fence exposure/identity again after provider I/O; never redirect stale STRM.
	latest, err := r.load(ctx, id)
	if err != nil {
		return nativeplayback.RedirectTarget{}, err
	}
	if latest.Path != e.Path || latest.Pick != e.Pick || latest.Output != e.Output || latest.Revision != e.Revision || latest.Size != e.Size || subtle.ConstantTimeCompare(latest.Digest, e.Digest) != 1 {
		return nativeplayback.RedirectTarget{}, nativeplayback.ErrNotFound
	}
	return nativeplayback.RedirectTarget{URL: link.URL, ExpiresAt: link.ExpiresAt}, nil
}
