package linksource

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/settings"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
)

// NewForManager deliberately uses DynamicStorage, NOT linkcache.Cached: probe
// URLs must never enter Redis. No listener, player admission or HTTP self-call.
func NewForManager(m *integrations.Manager, c Config, logger *slog.Logger) (*Source, error) {
	if m == nil || m.Repository.DB == nil {
		return nil, fail(Retryable, Unavailable, 0)
	}
	if c.ResolveTimeout == 0 {
		c.ResolveTimeout = m.Config.PlaybackResolveTimeout
	}
	return New(Dependencies{Repository: SQLRepository{DB: m.Repository.DB}, Reader: RootReader{Root: m.Config.STRMOutputDir}, Parser: managedParser{m}, Resolver: trustedResolver{integrations.DynamicStorage{Manager: m}}, Policy: playback.Policy{AllowedHosts: m.Config.PlaybackAllowedHosts, AllowHTTP: m.Config.PlaybackAllowHTTP}}, c, logger)
}

type managedParser struct{ m *integrations.Manager }

func (p managedParser) Parse(ctx context.Context, raw string) (storage.FileRef, error) {
	c := p.m.Config
	origin, err := settings.PlaybackOrigin(ctx, p.m.Repository.DB, c.PublicURL)
	if err != nil {
		return storage.FileRef{}, classify(ctx, err)
	}
	builder := strm.Builder{BaseURL: origin, APIKey: c.STRMKey, AllowedRoots: c.STRMAllowedRoots}
	profile, err := p.m.Repository.Default(ctx, "clouddrive2")
	if err == nil {
		if root := strings.TrimRight(profile.String("cloudRoot"), "/"); root != "" {
			builder.AllowedRoots = []string{root}
		}
	} else if !errors.Is(err, integrations.ErrNotFound) {
		return storage.FileRef{}, classify(ctx, err)
	}
	grants := playbackgrants.Service{DB: p.m.Repository.DB, Vault: p.m.Repository.Vault, PublicURL: c.PublicURL}
	ref, err := strm.ParseStable(ctx, raw, strm.ParseOptions{Builder: builder, Origins: []string{c.PublicURL}, LegacyEnabled: c.LegacyPlaybackEnabled, ManagedURL: grants.VerifyManagedURL})
	if err != nil {
		if errors.Is(err, playbackgrants.ErrDenied) || errors.Is(err, storage.ErrUnavailable) || ctx.Err() != nil {
			return ref, classify(ctx, err)
		}
		return ref, fail(Manual, Mismatch, 0)
	}
	return ref, nil
}

type trustedResolver struct{ inner storage.PlaybackResolver }

func (r trustedResolver) Resolve(ctx context.Context, ref storage.FileRef) (storage.DownloadLink, error) {
	// This unexported capability is set ONLY by Fetch, never from a request/header.
	// External UA allowlists remain unchanged; grant and target checks still apply.
	if !trustedProbe(ctx) {
		return storage.DownloadLink{}, fail(Manual, Authorization, 0)
	}
	return r.inner.Resolve(ctx, ref)
}
