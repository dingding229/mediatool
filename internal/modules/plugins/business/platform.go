package business

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"media-tool/internal/modules/cacheview"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/modules/messaging"
	"media-tool/internal/modules/networkproxy"
	"media-tool/internal/modules/settings"
	systemmod "media-tool/internal/modules/system"
	"media-tool/internal/platform/buildinfo"
	"media-tool/internal/platform/pluginactor"
	"regexp"
	"slices"
	"strings"
)

var safeSHA = regexp.MustCompile(`^[a-f0-9]{64}$`)

func platformMethod(method string) bool {
	return strings.HasPrefix(method, "integrations.") || strings.HasPrefix(method, "settings.") || strings.HasPrefix(method, "cache.") || strings.HasPrefix(method, "jobs.") || strings.HasPrefix(method, "maintenance.") || strings.HasPrefix(method, "system.") || strings.HasPrefix(method, "network.") || strings.HasPrefix(method, "networkproxy.") || method == "notifications.publish" || method == "notifications.deliveries" || method == "notifications.retry"
}
func platformHealth(ctx context.Context, h AcquisitionHost) b.HealthSummary {
	v := systemmod.Inspect(ctx, h.DB, h.Redis)
	return b.HealthSummary{Postgres: safeState(v.PostgreSQL.State), Redis: safeState(v.Redis.State), Migration: v.Migration, SampledAt: v.SampledAt}
}
func ownedJob(v jobs.Owned) b.OwnedJob {
	return b.OwnedJob{ID: v.ID, Kind: v.Kind, State: v.State, Attempts: v.Attempts, CreatedAt: v.CreatedAt, AvailableAt: v.AvailableAt, Cancelable: v.Cancelable}
}
func (h AcquisitionHost) callPlatform(ctx context.Context, call contracts.HostCall, p contracts.Permission, admit func() error) (any, error) {
	spec, ok := b.Lookup(call.Method)
	if !ok || !spec.Supported {
		return nil, unsupportedBusiness()
	}
	if spec.Scope != p.Scope {
		return nil, contracts.NewError(contracts.PermissionDenied)
	}
	actor, e := pluginactor.Require(ctx, p.Scope)
	if e != nil {
		return nil, contracts.NewError(contracts.PermissionDenied)
	}
	if !mediaBounds(call.Args) {
		return nil, invalidBusiness()
	}
	switch call.Method {
	case "system.info", "system.modules", "system.health", "maintenance.health", "networkproxy.status":
		var in b.Empty
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		if call.Method == "system.info" {
			return b.SystemInfo{Version: buildinfo.Version, Revision: buildinfo.Revision, ContractVersion: contracts.ContractVersion}, nil
		}
		if call.Method == "system.modules" {
			out := []b.SystemModule{}
			for _, m := range systemmod.Modules() {
				out = append(out, b.SystemModule{ID: m.ID, Name: serverLabel(m.Name), Stage: m.Stage})
			}
			return out, nil
		}
		if call.Method == "networkproxy.status" {
			if h.DB == nil {
				return nil, unsupportedBusiness()
			}
			v, e := (networkproxy.Repository{DB: h.DB}).Summary(ctx)
			return b.ProxySummary{Configured: v.Configured, TMDB: v.TMDB, Sites: v.Sites, Revision: v.Revision}, e
		}
		return platformHealth(ctx, h), nil
	case "integrations.list", "integrations.status", "integrations.test":
		if h.Connections == nil {
			return nil, unsupportedBusiness()
		}
		page := b.PageRequest{Page: 1, Size: 1}
		id := ""
		if call.Method == "integrations.list" {
			if decodeBusiness(call.Args, &page) != nil || pageOK(page) != nil {
				return nil, invalidBusiness()
			}
		} else {
			id, e = businessID(call.Args)
			if e != nil {
				return nil, e
			}
		}
		v, e := h.Connections.Repository.Summaries(ctx, (page.Page-1)*page.Size, page.Size, page.Query, id)
		if e != nil {
			return nil, e
		}
		if call.Method == "integrations.test" {
			if len(v) != 1 {
				return nil, contracts.NewError(contracts.NotFound)
			}
			if v[0].Kind == "open115" {
				return nil, unsupportedBusiness()
			}
			if e = admit(); e != nil {
				return nil, e
			}
			result, e := h.Connections.Test(ctx, id)
			return b.ConnectionCheck{Passed: result.Passed, State: safeState(result.State)}, e
		}
		out := b.Page[b.Connection]{Items: []b.Connection{}, HasMore: len(v) > page.Size}
		if len(v) > page.Size {
			v = v[:page.Size]
		}
		for _, r := range v {
			out.Items = append(out.Items, b.Connection{ID: r.ID, Kind: r.Kind, Name: serverLabel(r.Name), Enabled: r.Enabled, Revision: r.Revision, State: safeState(r.State), CheckedAt: r.CheckedAt})
		}
		if call.Method == "integrations.status" {
			if len(out.Items) != 1 {
				return nil, contracts.NewError(contracts.NotFound)
			}
			return out.Items[0], nil
		}
		return out, nil
	case "settings.get", "settings.patch":
		if h.DB == nil {
			return nil, unsupportedBusiness()
		}
		r := settings.Repository{DB: h.DB}
		keys := []string{}
		var values map[string]json.RawMessage
		var revision int64
		if call.Method == "settings.get" {
			var in b.SettingKeys
			if decodeBusiness(call.Args, &in) != nil {
				return nil, invalidBusiness()
			}
			keys = in.Keys
		} else {
			var in b.SettingPatch
			if decodeBusiness(call.Args, &in) != nil || call.ExpectedRevision == nil || *call.ExpectedRevision != in.Revision {
				return nil, invalidBusiness()
			}
			values = in.Values
			revision = in.Revision
			for k := range values {
				keys = append(keys, k)
			}
		}
		if len(keys) < 1 || len(keys) > 200 {
			return nil, invalidBusiness()
		}
		for _, key := range keys {
			if !settings.PluginKey(key) || !selected(p, "settingKeys", key) {
				return nil, contracts.NewError(contracts.PermissionDenied)
			}
		}
		if call.Method == "settings.get" {
			v, rev, e := r.PluginGet(ctx, keys)
			return b.SettingsView{Values: v, Revision: rev}, e
		}
		if e = admit(); e != nil {
			return nil, e
		}
		v, rev, e := r.PluginPatch(ctx, values, revision)
		return b.SettingsView{Values: v, Revision: rev}, e
	case "jobs.list", "jobs.get", "jobs.cancel":
		if h.DB == nil {
			return nil, unsupportedBusiness()
		}
		r := jobs.Repository{DB: h.DB}
		page := b.PageRequest{Page: 1, Size: 1}
		id, kind, state := "", "", ""
		if call.Method == "jobs.list" {
			var in b.JobQuery
			if decodeBusiness(call.Args, &in) != nil || pageOK(in.PageRequest) != nil {
				return nil, invalidBusiness()
			}
			page, kind, state = in.PageRequest, in.Kind, in.State
			if page.Query != "" {
				return nil, unsupportedBusiness()
			}
		} else {
			id, e = businessID(call.Args)
			if e != nil {
				return nil, e
			}
		}
		if call.Method == "jobs.cancel" {
			if e = admit(); e != nil {
				return nil, e
			}
			e = r.PluginCancel(ctx, actor.InstanceID(), id)
			if errors.Is(e, jobs.ErrUnsupportedKind) || errors.Is(e, jobs.ErrOperatorRequired) {
				return nil, unsupportedBusiness()
			}
			return b.Receipt{ID: id, State: "cancelled"}, e
		}
		v, e := r.PluginPage(ctx, actor.InstanceID(), (page.Page-1)*page.Size, page.Size, kind, state, id)
		if e != nil {
			return nil, e
		}
		out := b.Page[b.OwnedJob]{Items: []b.OwnedJob{}, HasMore: len(v) > page.Size}
		if len(v) > page.Size {
			v = v[:page.Size]
		}
		for _, j := range v {
			out.Items = append(out.Items, ownedJob(j))
		}
		if call.Method == "jobs.get" {
			if len(out.Items) != 1 {
				return nil, contracts.NewError(contracts.NotFound)
			}
			return out.Items[0], nil
		}
		return out, nil
	case "cache.list", "cache.status", "cache.invalidate":
		if h.Redis == nil || h.DB == nil {
			return nil, unsupportedBusiness()
		}
		s := cacheview.Store{DB: h.DB, Redis: h.Redis}
		if call.Method == "cache.invalidate" {
			var in b.CacheInvalidation
			if decodeBusiness(call.Args, &in) != nil || len(in.IDs) < 1 || len(in.IDs) > 200 || in.Namespace != "tmdb" && in.Namespace != "search" {
				return nil, invalidBusiness()
			}
			for _, id := range in.IDs {
				if in.Namespace == "tmdb" && !safeSHA.MatchString(id) || in.Namespace == "search" && !validUUID(id) {
					return nil, invalidBusiness()
				}
			}
			slices.Sort(in.IDs)
			in.IDs = slices.Compact(in.IDs)
			for _, id := range in.IDs {
				if e = admit(); e != nil {
					return nil, e
				}
				if e = s.Invalidate(ctx, in.Namespace, id); e != nil {
					return nil, e
				}
			}
			return b.Count{Count: len(in.IDs)}, nil
		}
		var in b.NamespaceQuery
		if decodeBusiness(call.Args, &in) != nil || pageOK(in.PageRequest) != nil || in.Size > 100 || in.Namespace != "tmdb" && in.Namespace != "search" {
			return nil, invalidBusiness()
		}
		if in.Query != "" {
			return nil, unsupportedBusiness()
		}
		v, n, truncated, e := s.PluginPage(ctx, in.Namespace, in.Page, in.Size)
		if e != nil {
			return nil, e
		}
		if call.Method == "cache.status" {
			return b.CacheSummary{Namespace: in.Namespace, Entries: n, Truncated: truncated}, nil
		}
		out := b.CachePage{Page: b.Page[b.CacheEntry]{Items: []b.CacheEntry{}, HasMore: n > in.Page*in.Size}, Truncated: truncated}
		for _, v := range v {
			out.Items = append(out.Items, b.CacheEntry{ID: v.ID, Bytes: int64(v.Bytes), TTLSeconds: v.TTLSeconds, State: safeState(v.State)})
		}
		return out, nil
	case "maintenance.alerts":
		if h.DB == nil {
			return nil, unsupportedBusiness()
		}
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		if in.Query != "" {
			return nil, unsupportedBusiness()
		}
		v, e := (maintenance.Engine{DB: h.DB}).AlertPage(ctx, (in.Page-1)*in.Size, in.Size)
		if e != nil {
			return nil, e
		}
		out := b.Page[b.AlertSummary]{Items: []b.AlertSummary{}, HasMore: len(v) > in.Size}
		if len(v) > in.Size {
			v = v[:in.Size]
		}
		for _, a := range v {
			out.Items = append(out.Items, b.AlertSummary{Code: serverLabel(a.Code), State: safeState(a.State), Hits: a.Hits, Value: a.Value, LastSeenAt: a.LastSeenAt})
		}
		return out, nil
	case "notifications.publish", "notifications.deliveries", "notifications.retry":
		if h.DB == nil {
			return nil, unsupportedBusiness()
		}
		d := messaging.Dispatcher{DB: h.DB, Connections: h.Connections}
		if call.Method == "notifications.publish" {
			var in b.Notice
			if decodeBusiness(call.Args, &in) != nil || in.Title == "" {
				return nil, invalidBusiness()
			}
			if e = admit(); e != nil {
				return nil, e
			}
			id, e := d.PluginPublish(ctx, actor.InstanceID(), call.OperationKey, in.Title, in.Text)
			return b.Receipt{ID: id, State: "pending"}, e
		}
		if call.Method == "notifications.retry" {
			var in b.DeliveryRetry
			if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) {
				return nil, invalidBusiness()
			}
			if h.Connections == nil {
				return nil, unsupportedBusiness()
			}
			if e = admit(); e != nil {
				return nil, e
			}
			e = d.PluginRetry(ctx, actor.InstanceID(), in.ID, in.AcceptDuplicateRisk)
			return b.Receipt{ID: in.ID, State: "pending"}, e
		}
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		if in.Query != "" {
			return nil, unsupportedBusiness()
		}
		v, e := d.PluginDeliveries(ctx, actor.InstanceID(), (in.Page-1)*in.Size, in.Size)
		if e != nil {
			return nil, e
		}
		out := b.Page[b.DeliverySummary]{Items: []b.DeliverySummary{}, HasMore: len(v) > in.Size}
		if len(v) > in.Size {
			v = v[:in.Size]
		}
		for _, v := range v {
			out.Items = append(out.Items, b.DeliverySummary{ID: v.ID, EventID: v.EventID, State: v.State, Attempts: v.Attempts, CreatedAt: v.CreatedAt, NextAttemptAt: v.NextAttemptAt})
		}
		return out, nil
	case "network.http":
		return h.hostHTTP(ctx, call, p, admit)
	}
	return nil, unsupportedBusiness()
}

func (h AcquisitionHost) hostHTTP(ctx context.Context, call contracts.HostCall, p contracts.Permission, admit func() error) (any, error) {
	if h.Network == nil {
		return nil, unsupportedBusiness()
	}
	var in b.HostRequest
	if decodeBusiness(call.Args, &in) != nil {
		return nil, invalidBusiness()
	}
	if in.Profile != "host" || in.Method != "GET" && in.Method != "HEAD" {
		return nil, unsupportedBusiness()
	}
	if !selected(p, "hosts", in.Origin) || len(p.Resources["hosts"]) == 0 {
		return nil, contracts.NewError(contracts.PermissionDenied)
	}
	if e := admit(); e != nil {
		return nil, e
	}
	actor, e := pluginactor.Require(ctx, p.Scope)
	if e != nil {
		return nil, e
	}
	v, e := h.Network.Request(ctx, actor.InstanceID(), in.Profile, in.Origin, in.Path, in.Method, p.Resources["hosts"])
	if errors.Is(e, networkproxy.ErrHostLimited) {
		return nil, contracts.NewError(contracts.UpstreamLimited)
	}
	if errors.Is(e, networkproxy.ErrHostUnsupported) {
		return nil, unsupportedBusiness()
	}
	return b.HTTPResult{Status: v.Status, ContentType: v.ContentType, BytesRead: v.BytesRead}, e
}
