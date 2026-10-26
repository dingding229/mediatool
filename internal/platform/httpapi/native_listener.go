package httpapi

import (
	"context"
	"media-tool/internal/modules/embyapi"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativeauth"
	"media-tool/internal/modules/mediausers/nativepermissions"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/modules/nativeplayback/runtime"
	"media-tool/internal/modules/playeraccess/clientinfo"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/config"
	"net/http"
	"time"
)

// NativeMediaHandler never wraps or forwards to the management handler.
func NativeMediaHandler(c config.Config, d Dependencies) (http.Handler, error) {
	if d.IPBlock == nil && d.DB != nil {
		d.IPBlock = newIPBlock(d.DB, d.Logger)
	}
	images := NewNativeImageResolver(d.DB, d.Integrations, d.Redis)
	images.Stills = d.NativeStills
	images.ImageRoot = d.NativeImageRoot
	browse := catalog.NewCachedBrowse(catalog.NewBrowseRepository(d.DB, d.Logger), d.Redis)
	playback := d.NativePlayback
	if playback == nil {
		svc := runtime.New(d.DB, c, d.Integrations, d.Redis, d.Storage, d.Logger)
		svc.Stats = d.PlaybackStats
		playback = svc
	}
	var observer nativeauth.DeviceObserver
	if o, ok := d.PlaybackStats.(nativeauth.DeviceObserver); ok {
		observer = o
	}
	handler, err := embyapi.NewServer(embyapi.ServerOptions{IPAdmission: func(r *http.Request) (bool, error) { return checkMediaIP(r.Context(), d.IPBlock, d.DB, r) }, Playback: playback, RequestOrigin: func(r *http.Request) (string, error) {
		if evidence, ok := r.Context().Value(ipEvidenceKey{}).(ipEvidence); ok {
			return clientinfo.RequestOrigin(r, evidence.proxies), evidence.err
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		prefs, err := (settings.Repository{DB: d.DB}).Get(ctx)
		if err != nil {
			return "", err
		}
		return clientinfo.RequestOrigin(r, prefs.Value.TrustedProxyCIDRs), nil
	}, Settings: settings.NewNativeServerRepository(d.DB), PublicURL: c.MemberPublicURL, Logger: d.Logger, Images: images, Browse: browse, Discovery: browse, Auth: nativeAccounts{AccountService: &nativeauth.Store{DB: d.DB, Secret: c.SessionKey, Observer: observer}, global: c.STRMKey}, Permissions: func() contracts.PermissionChecker { return nativepermissions.New(d.DB) }, ClientAddress: func(r *http.Request) (clientinfo.Address, error) {
		if evidence, ok := r.Context().Value(ipEvidenceKey{}).(ipEvidence); ok {
			return publicEvidence(evidence.address), evidence.err
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		prefs, err := (settings.Repository{DB: d.DB}).Get(ctx)
		if err != nil {
			return clientinfo.Address{}, err
		}
		return clientinfo.ClientAddress(r, prefs.Value.TrustedProxyCIDRs), nil
	}})
	if err != nil {
		return nil, err
	}
	return idleMedia(handler, d.DB, func(*http.Request) bool { return true }), nil
}

// An operator global STRM key is never an Emby user credential, even if a
// manually modified token table accidentally contains the same digest.
type nativeAccounts struct {
	contracts.AccountService
	global string
}

func (a nativeAccounts) Authenticate(ctx context.Context, raw string, d contracts.DeviceInfo) (contracts.User, contracts.Device, contracts.Token, error) {
	if a.global != "" && auth.Match(a.global, raw) {
		return contracts.User{}, contracts.Device{}, contracts.Token{}, contracts.ErrUnauthenticated
	}
	return a.AccountService.Authenticate(ctx, raw, d)
}
