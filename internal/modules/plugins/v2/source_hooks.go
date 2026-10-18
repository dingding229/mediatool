package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/dingding229/mediatool-plugin/sdk/events"
	"github.com/jackc/pgx/v5"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/trace"
	"time"
)

// SourceProviders never invokes a worker. Cached UI consumers still validate the
// exact current descriptor/grants/lease; a snapshot is not cached authorization.
func (s Store) SourceProviders(ctx context.Context, typ contracts.ChainEventType) ([]Snapshot, error) {
	if typ != contracts.ChainDiscoverSource && typ != contracts.ChainRecommendSource {
		return nil, ErrInvalid
	}
	hooks, e := s.hooks(ctx, "chain", string(typ))
	if e != nil {
		return nil, e
	}
	out := []Snapshot{}
	for _, h := range hooks {
		snap, _, e := s.Descriptor(ctx, h.instance, "chain", string(typ))
		if e == nil {
			out = append(out, snap)
		}
	}
	return out, nil
}
func (s Store) SourcePage(ctx context.Context, snap Snapshot, typ contracts.ChainEventType, q contracts.SourceRequest) (contracts.SourceResult, error) {
	empty := contracts.SourceResult{Items: []contracts.SourceCandidate{}}
	if (typ != contracts.ChainDiscoverSource && typ != contracts.ChainRecommendSource) || contracts.ValidateSource(q, empty) != nil {
		return empty, ErrInvalid
	}
	bounded, cancel := context.WithTimeout(context.WithValue(ctx, hookContext{}, true), 200*time.Millisecond)
	defer cancel()
	deadline, _ := bounded.Deadline()
	bounded = context.WithValue(bounded, policyDeadline{}, deadline)
	current, d, e := s.Descriptor(bounded, snap.InstanceID, "chain", string(typ))
	if e != nil || current.ID != snap.ID {
		return empty, ErrConflict
	}
	raw, _ := json.Marshal(q)
	args, _ := json.Marshal(events.ChainEnvelope{Type: typ, SchemaVersion: 1, Payload: raw})
	raw, e = s.invokeSnapshot(bounded, snap, d, args, "")
	if e != nil {
		trace.Warn(ctx, "plugins", "附加来源回落空列表", "chain", string(typ), "reason", "timeout_or_failed")
		return empty, e
	}
	var result contracts.SourceResult
	if contracts.Decode(raw, &result) != nil || contracts.ValidateSource(q, result) != nil {
		trace.Warn(ctx, "plugins", "拒绝非法附加来源并回落空列表", "chain", string(typ), "reason", "invalid_result")
		return empty, ErrInvalid
	}
	fenced := dbutil.WithCommitGuard(bounded, func(c context.Context, tx pgx.Tx) error { return s.fencePolicy(c, tx, snap, d) })
	if e = dbutil.CheckPrepared(fenced, s.DB); e != nil {
		return empty, e
	}
	return result, nil
}
