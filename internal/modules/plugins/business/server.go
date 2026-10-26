package business

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"

	"media-tool/internal/modules/mediaserver"
	"media-tool/internal/modules/mediausers"
	nativecatalog "media-tool/internal/modules/nativelibrary/catalog"
	playbackruntime "media-tool/internal/modules/nativeplayback/runtime"
	"media-tool/internal/modules/nativetmdb"
	"media-tool/internal/modules/playeraccess"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/pluginactor"
	"regexp"
	"strings"
)

var serverID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

// Client-controlled names/plots must not smuggle addresses or credentials into
// an otherwise safe summary DTO. The native display policy is a first layer.
var privateServerText = regexp.MustCompile(`(?i)(?:client.?ip|user.?agent|password|cookie|authorization|api.?key|token|pick.?code)\s*[:=]|\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)

func serverLabel(value string) string {
	value = mediausers.ManagementLabel(value)
	if privateServerText.MatchString(value) {
		return "[内容已隐藏]"
	}
	return value
}
func serverItem(v mediaserver.Item) b.ServerItem {
	return b.ServerItem{ID: v.ID, Title: serverLabel(v.Title), Year: serverLabel(v.Year), MediaType: safeState(v.MediaType), Overview: serverLabel(v.Overview), Season: v.Season, Episode: v.Episode}
}
func safeUser(v mediausers.User) (b.ServerUser, error) {
	out := b.ServerUser{ID: v.ID, Username: serverLabel(v.Username), DisplayName: serverLabel(v.DisplayName), Enabled: v.Enabled, Revision: v.Revision, MaxStreams: v.MaxStreams, MaxParentalRating: v.MaxParentalRating, ExpiresAt: v.ExpiresAt, Grants: []b.ServerGrant{}}
	if len(v.Grants) > 200 {
		return out, contracts.NewError(contracts.PayloadTooLarge)
	}
	for _, g := range v.Grants {
		out.Grants = append(out.Grants, b.ServerGrant{LibraryID: g.LibraryID, CanPlay: g.CanPlay, CanDownload: g.CanDownload})
	}
	return out, nil
}
func (h AcquisitionHost) serverCatalog() mediaserver.PluginCatalog {
	registry, _ := mediaserver.NewRegistry(mediaserver.Builtin{DB: h.DB})
	return mediaserver.PluginCatalog{DB: h.DB, Registry: registry}
}
func (h AcquisitionHost) callServer(ctx context.Context, call contracts.HostCall, p contracts.Permission, admit func() error) (out any, err error) {
	// No wire field can establish actor authority, even if it names a real user.
	spec, ok := b.Lookup(call.Method)
	if !ok || !spec.Supported {
		return nil, unsupportedBusiness()
	}
	if spec.Scope != p.Scope {
		return nil, contracts.NewError(contracts.PermissionDenied)
	}
	if _, err = pluginactor.Require(ctx, p.Scope); err != nil {
		return nil, contracts.NewError(contracts.PermissionDenied)
	}
	if !mediaBounds(call.Args) {
		return nil, invalidBusiness()
	}
	if h.DB == nil {
		return nil, unsupportedBusiness()
	}
	defer func() {
		if errors.Is(err, pluginactor.ErrDenied) || errors.Is(err, mediaserver.ErrDenied) {
			err = contracts.NewError(contracts.PermissionDenied)
		}
		if errors.Is(err, mediaserver.ErrUnsupported) {
			err = unsupportedBusiness()
		}
	}()
	switch call.Method {
	case "mediaserver.providers":
		var in b.Empty
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		out := []b.ServerProvider{}
		for _, d := range h.serverCatalog().Registry.Descriptions() {
			out = append(out, b.ServerProvider{ID: d.ID, Name: serverLabel(d.Name), Catalog: d.Capabilities.Catalog, Refresh: true})
		}
		return out, nil
	case "mediaserver.libraries", "mediaserver.items", "catalog.list", "catalog.item":
		var in b.ServerQuery
		if decodeBusiness(call.Args, &in) != nil || pageOK(in.PageRequest) != nil || in.Provider != "builtin" || in.MediaType != "" && in.MediaType != "movie" && in.MediaType != "series" {
			return nil, invalidBusiness()
		}
		if in.LibraryID != "" && strings.ContainsAny(in.LibraryID, "/\\.") {
			return nil, invalidBusiness()
		}
		service := h.serverCatalog()

		if call.Method == "mediaserver.libraries" {
			v, n, e := service.Libraries(ctx, in.Provider, (in.Page-1)*in.Size, in.Size)
			if e != nil {
				return nil, e
			}
			out := b.Page[b.ServerLibrary]{Items: []b.ServerLibrary{}, HasMore: n > in.Page*in.Size}
			for _, l := range v {
				out.Items = append(out.Items, b.ServerLibrary{ID: l.ID, Name: serverLabel(l.Name), MediaType: l.MediaType, ItemCount: l.Count})
			}
			return out, nil
		}
		if call.Method == "catalog.item" && in.ID == "" || in.ID != "" && !validUUID(in.ID) {
			return nil, invalidBusiness()
		}
		v, n, e := service.Items(ctx, in.Provider, in.LibraryID, in.ID, in.Query, in.MediaType, in.Page, in.Size)
		if e != nil {
			return nil, e
		}
		if call.Method == "catalog.item" {
			if len(v) != 1 {
				return nil, contracts.NewError(contracts.NotFound)
			}
			return serverItem(v[0]), nil
		}
		result := b.Page[b.ServerItem]{Items: []b.ServerItem{}, HasMore: n > in.Page*in.Size}
		for _, i := range v {
			result.Items = append(result.Items, serverItem(i))
		}
		return result, nil
	case "mediaserver.refresh":
		var in b.ServerRefresh
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.AssetID) || in.Provider != "builtin" {
			return nil, invalidBusiness()
		}
		if h.Connections == nil {
			return nil, unsupportedBusiness()
		}
		guarded, e := (nativecatalog.Repository{DB: h.DB}).PluginRefreshContext(ctx, in.AssetID)
		if e != nil {
			return nil, e
		}
		if e = admit(); e != nil {
			return nil, e
		}
		// Both branches queue existing native workflows; never invoke a job handler
		// or synchronously refresh through an HTTP/Playback client.
		if in.Provider == "builtin" {
			lang, err := (nativetmdb.Engine{DB: h.DB, Connections: h.Connections}).Language(guarded)
			if err != nil {
				return nil, err
			}
			e = (nativetmdb.Repository{DB: h.DB}).EnqueueAsset(guarded, in.AssetID, lang)
		}
		return b.Receipt{ID: in.AssetID, State: "requested"}, e
	case "mediaserver.settings.get":
		var in b.Empty
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		v, e := settings.NewNativeServerRepository(h.DB).Get(ctx)
		return b.ServerConfiguration{ServerName: serverLabel(v.ServerName), Enabled: v.Enabled, Revision: v.Revision}, e
	case "mediaserver.settings.update":
		var in b.ServerConfiguration
		if decodeBusiness(call.Args, &in) != nil || call.ExpectedRevision == nil || *call.ExpectedRevision != in.Revision {
			return nil, invalidBusiness()
		}
		if e := admit(); e != nil {
			return nil, e
		}
		v, e := settings.SaveNativeManagement(ctx, h.DB, settings.NativeManagement{ServerName: in.ServerName, Enabled: in.Enabled, Revision: in.Revision})
		return b.ServerConfiguration{ServerName: serverLabel(v.ServerName), Enabled: v.Enabled, Revision: v.Revision}, e
	case "playback.sessions", "mediaserver.sessions":
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		if in.Query != "" {
			return nil, unsupportedBusiness()
		}
		// Scoped session membership cannot be proven from List's current safe DTO.
		a, _ := pluginactor.Require(ctx, p.Scope)
		if a.Restricted() {
			return nil, unsupportedBusiness()
		}
		v, e := (playbackruntime.SessionRepository{DB: h.DB}).List(ctx, (in.Page-1)*in.Size, in.Size)
		if e != nil {
			return nil, e
		}
		out := b.Page[b.PlaybackSession]{Items: []b.PlaybackSession{}, HasMore: v.Total > in.Page*in.Size}
		for _, s := range v.Items {
			out.Items = append(out.Items, b.PlaybackSession{ID: s.ID, UserID: s.UserID, UserName: serverLabel(s.UserName), DeviceID: s.DeviceID, DeviceName: serverLabel(s.DeviceName), ItemID: s.ItemID, ItemName: serverLabel(s.ItemName), State: s.State, PositionTicks: s.PositionTicks, LastProgressAt: s.LastProgressAt})
		}
		return out, nil
	case "playback.summary", "playeraccess.summary":
		var in b.Empty
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		a, _ := pluginactor.Require(ctx, p.Scope)
		if a.Restricted() {
			return nil, unsupportedBusiness()
		}
		if call.Method == "playback.summary" {
			v, e := (playbackruntime.SessionRepository{DB: h.DB}).Summary(ctx)
			return b.PlaybackCounts{Playing: v.Playing, Paused: v.Paused}, e
		}
		v, e := (playeraccess.Repository{DB: h.DB}).Summary(ctx)
		return b.AccessCounts{Sampled: v.Sampled, Allowed: v.Allowed, Denied: v.Denied, MaxRecords: 10000}, e
	}
	if strings.HasPrefix(call.Method, "mediausers.") {
		return h.callUsers(ctx, call, admit)
	}
	if strings.HasPrefix(call.Method, "mediaprobe.") {
		return h.callProcessing(ctx, call, admit)
	}
	if strings.HasPrefix(call.Method, "metadata.") {
		return h.callNativeDetails(ctx, call, admit)
	}
	return nil, unsupportedBusiness()
}
func (h AcquisitionHost) callUsers(ctx context.Context, call contracts.HostCall, admit func() error) (any, error) {
	r := mediausers.Repository{DB: h.DB}
	if call.Method == "mediausers.list" {
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		v, e := r.Page(ctx, (in.Page-1)*in.Size, in.Size, in.Query)
		if e != nil {
			return nil, e
		}
		out := b.Page[b.ServerUser]{Items: []b.ServerUser{}, HasMore: len(v) > in.Size}
		if len(v) > in.Size {
			v = v[:in.Size]
		}
		for _, u := range v {
			s, e := safeUser(u)
			if e != nil {
				return nil, e
			}
			out.Items = append(out.Items, s)
		}
		return out, nil
	}
	if call.Method == "mediausers.get" {
		id, e := businessID(call.Args)
		if e != nil {
			return nil, e
		}
		v, e := r.Get(ctx, id)
		if e != nil {
			return nil, e
		}
		return safeUser(v)
	}
	if call.Method == "mediausers.devices" || call.Method == "mediausers.history" {
		var in b.Selection
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) || pageOK(in.PageRequest) != nil {
			return nil, invalidBusiness()
		}
		if in.Query != "" {
			return nil, unsupportedBusiness()
		}
		if call.Method == "mediausers.devices" {
			v, e := r.Devices(ctx, in.ID, (in.Page-1)*in.Size, in.Size)
			if e != nil {
				return nil, e
			}
			out := b.Page[b.ServerDevice]{Items: []b.ServerDevice{}, HasMore: v.Total > in.Page*in.Size}
			for _, d := range v.Items {
				out.Items = append(out.Items, b.ServerDevice{ID: d.ID, Name: serverLabel(d.Name), Client: serverLabel(d.Client), Version: serverLabel(d.AppVersion), LastSeenAt: d.LastSeenAt})
			}
			return out, nil
		}
		v, e := r.History(ctx, in.ID, (in.Page-1)*in.Size, in.Size)
		if e != nil {
			return nil, e
		}
		out := b.Page[b.WatchHistory]{Items: []b.WatchHistory{}, HasMore: v.Total > in.Page*in.Size}
		for _, i := range v.Items {
			out.Items = append(out.Items, b.WatchHistory{ItemID: i.ItemID, Name: serverLabel(i.Name), Type: i.Type, PositionTicks: i.PositionTicks, PlayCount: i.PlayCount, Played: i.Played, LastPlayedAt: i.LastPlayedAt})
		}
		return out, nil
	}
	if call.Method == "mediausers.revokeDevice" {
		var in b.DeviceRevocation
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) || !validUUID(in.DeviceID) || call.ExpectedRevision == nil || *call.ExpectedRevision != in.Revision {
			return nil, invalidBusiness()
		}
		if e := admit(); e != nil {
			return nil, e
		}
		e := r.RevokeDeviceExpected(ctx, in.ID, in.DeviceID, in.Revision)
		return b.Receipt{ID: in.DeviceID, State: "revoked"}, e
	}
	if call.Method != "mediausers.update" {
		return nil, unsupportedBusiness()
	}
	var in b.UserPatch
	if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) || call.ExpectedRevision == nil || *call.ExpectedRevision != in.Revision || in.ClearExpiry && in.ExpiresAt != nil || in.ClearParentalRating && in.MaxParentalRating != nil {
		return nil, invalidBusiness()
	}
	current, e := r.Get(ctx, in.ID)
	if e != nil {
		return nil, e
	}
	if current.Revision != in.Revision {
		return nil, contracts.NewError(contracts.Conflict)
	}
	native := mediausers.Input{Username: current.Username, DisplayName: current.DisplayName, Enabled: current.Enabled, Revision: current.Revision, Grants: current.Grants, MaxStreams: &current.MaxStreams, MaxParentalRating: current.MaxParentalRating, ExpiresAt: current.ExpiresAt}
	if in.DisplayName != nil {
		native.DisplayName = *in.DisplayName
	}
	if in.Enabled != nil {
		native.Enabled = *in.Enabled
	}
	if in.MaxStreams != nil {
		native.MaxStreams = in.MaxStreams
	}
	if in.Grants != nil {
		native.Grants = copyDTO[[]mediausers.Grant](*in.Grants)
	}
	if in.MaxParentalRating != nil || in.ClearParentalRating {
		native.MaxParentalRating = in.MaxParentalRating
	}
	if in.ExpiresAt != nil || in.ClearExpiry {
		native.ExpiresAt = in.ExpiresAt
	}
	// Round-trip through the native input decoder preserves explicit nullable
	// semantics; no password field is accepted from the worker or generated.
	raw, _ := json.Marshal(native)
	if json.Unmarshal(raw, &native) != nil {
		return nil, invalidBusiness()
	}
	if e = admit(); e != nil {
		return nil, e
	}
	v, e := r.Save(ctx, in.ID, native)
	if e != nil {
		return nil, e
	}
	return safeUser(v)
}
