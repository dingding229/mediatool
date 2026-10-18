package business

import (
	"context"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"media-tool/internal/modules/mediaprobe/admin"
	"media-tool/internal/modules/mediaprobe/stillstore"
	"media-tool/internal/modules/nativetmdb"
	"strconv"
)

// The collector is injected from the existing singleton service/cache. No new
// runner/ffmpeg/link resolver is constructed or made reachable from Host.
type stillCollector interface {
	Collect(context.Context) (stillstore.GCReport, error)
}

func (h AcquisitionHost) callProcessing(ctx context.Context, call contracts.HostCall, admit func() error) (any, error) {
	if h.Processing == nil {
		return nil, unsupportedBusiness()
	}
	switch call.Method {
	case "mediaprobe.status":
		var in b.Empty
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		v, e := h.Processing.Summary(ctx)
		if e != nil {
			return nil, e
		}
		out := b.ProcessingStatus{Enabled: v.Settings.Enabled, ProbeEnabled: v.Settings.ProbeEnabled, StillsEnabled: v.Settings.StillsEnabled, Probe: []b.ProcessingStateCount{}, Still: []b.ProcessingStateCount{}, UsedBytes: v.Capacity.UsedBytes, ReservedBytes: v.Capacity.ReservedBytes, LimitBytes: v.Capacity.LimitBytes, UnknownSizes: v.Capacity.UnknownSizes}
		for _, s := range v.Probe {
			out.Probe = append(out.Probe, b.ProcessingStateCount{State: s.State, Count: s.Count})
		}
		for _, s := range v.Still {
			out.Still = append(out.Still, b.ProcessingStateCount{State: s.State, Count: s.Count})
		}
		return out, nil
	case "mediaprobe.list":
		var in b.ProcessingQuery
		if decodeBusiness(call.Args, &in) != nil || pageOK(in.PageRequest) != nil {
			return nil, invalidBusiness()
		}
		if in.Query != "" {
			return nil, unsupportedBusiness()
		}
		v, e := h.Processing.List(ctx, admin.Filter{Kind: in.Kind, State: in.State, Page: in.Page, PageSize: in.Size})
		if e != nil {
			return nil, e
		}
		out := b.Page[b.ProcessingTask]{Items: []b.ProcessingTask{}, HasMore: v.Total > int64(in.Page*in.Size)}
		for _, t := range v.Items {
			out.Items = append(out.Items, b.ProcessingTask{ID: t.ID, Name: serverLabel(t.Name), State: t.State, ErrorReason: safeState(t.ErrorReason), Attempts: t.Attempts, NextAttemptAt: t.NextAttemptAt})
		}
		return out, nil
	case "mediaprobe.retry", "mediaprobe.stills.retry":
		var in b.IDs
		if decodeBusiness(call.Args, &in) != nil || len(in.IDs) < 1 || len(in.IDs) > 200 {
			return nil, invalidBusiness()
		}
		kind := "probe"
		if call.Method == "mediaprobe.stills.retry" {
			kind = "still"
		}
		seen := map[string]bool{}
		// Validate the whole batch before admitting any native side effect.
		for _, id := range in.IDs {
			if seen[id] {
				return nil, invalidBusiness()
			}
			seen[id] = true
			if kind == "probe" {
				if !validUUID(id) {
					return nil, invalidBusiness()
				}
			} else if n, e := strconv.ParseInt(id, 10, 64); e != nil || n < 100000 {
				return nil, invalidBusiness()
			}
		}
		out := b.RetryCounts{Selected: len(in.IDs)}
		for _, id := range in.IDs {
			if e := admit(); e != nil {
				return nil, e
			}
			v, e := h.Processing.Retry(ctx, admin.Retry{Kind: kind, ID: id, Confirmed: true})
			if e != nil {
				return nil, e
			}
			out.Queued += v.Queued
			out.Skipped += v.Skipped
		}
		return out, nil
	case "mediaprobe.stills.gc":
		var in b.Empty
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		if h.Collector == nil {
			return nil, unsupportedBusiness()
		}
		if e := admit(); e != nil {
			return nil, e
		}
		v, e := h.Collector.Collect(ctx)
		return b.GCCounts{Visited: v.Visited, Reclaimed: v.Reclaimed, ReclaimedBytes: v.ReclaimedBytes, Done: v.Done, Partial: e != nil || v.Failures > 0}, e
	}
	return nil, unsupportedBusiness()
}
func (h AcquisitionHost) callNativeDetails(ctx context.Context, call contracts.HostCall, admit func() error) (any, error) {
	if h.Connections == nil {
		return nil, unsupportedBusiness()
	}
	var in b.NativeTarget
	if decodeBusiness(call.Args, &in) != nil || pageOK(in.PageRequest) != nil {
		return nil, invalidBusiness()
	}
	target := nativetmdb.Target{Type: in.MediaType, TMDBID: in.TMDBID, Season: in.Season}
	if !target.Valid() {
		return nil, invalidBusiness()
	}
	engine := nativetmdb.Engine{DB: h.DB, Connections: h.Connections}
	if call.Method == "metadata.refreshDetails" {
		if e := admit(); e != nil {
			return nil, e
		}
		id, e := engine.Refresh(ctx, target)
		return b.Receipt{ID: id, State: "requested"}, e
	}
	if call.Method != "metadata.nativeDetails" {
		return nil, unsupportedBusiness()
	}
	lang, e := engine.Language(ctx)
	if e != nil {
		return nil, e
	}
	v, state, e := (nativetmdb.Repository{DB: h.DB}).Stored(ctx, target, lang)
	if e != nil {
		return nil, e
	}
	out := b.NativeMetadata{State: state, Name: serverLabel(v.Name), OriginalTitle: serverLabel(v.OriginalTitle), Overview: serverLabel(v.Overview), RuntimeMinutes: v.RuntimeMinutes, Genres: []string{}}
	if len(v.Genres) > 200 {
		return nil, contracts.NewError(contracts.PayloadTooLarge)
	}
	for _, g := range v.Genres {
		out.Genres = append(out.Genres, serverLabel(g))
	}
	// Provider/native snapshots are bounded independently; page episodes before
	// projection, never return still paths or other image/playback addresses.
	eps := pageSlice(v.Episodes, in.PageRequest)
	out.Episodes = b.Page[b.EpisodeMetadata]{Items: []b.EpisodeMetadata{}, HasMore: eps.HasMore}
	for _, ep := range eps.Items {
		out.Episodes.Items = append(out.Episodes.Items, b.EpisodeMetadata{Season: ep.Season, Number: ep.Number, Name: serverLabel(ep.Name), AirDate: serverLabel(ep.AirDate), RuntimeMinutes: ep.RuntimeMinutes})
	}
	return out, nil
}
