package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/dingding229/mediatool-plugin/sdk/events"
	"github.com/jackc/pgx/v5"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/pluginactor"
	"media-tool/internal/platform/trace"
	"time"
)

type policyDeadline struct{}
type policyBudget struct{}

var ErrResourceRejected = errors.New("插件资源策略拒绝本次下载")

// acquisitionChain is prepare-only: no caller business transaction is held while
// waiting. Each accepted result adds a live commit fence, never cached authority.
// Policy handlers have only their declared chain scope, not downloader capability.
func (s Store) acquisitionChain(ctx context.Context, typ contracts.ChainEventType, input any, apply func(context.Context, json.RawMessage) bool) (context.Context, error) {
	return s.policyHooks(ctx, "chain", string(typ), input, apply)
}
func (s Store) policyHooks(ctx context.Context, kind, typ string, input any, apply func(context.Context, json.RawMessage) bool) (context.Context, error) {
	if ctx.Value(hookContext{}) != nil {
		return ctx, nil
	}
	until := time.Now().Add(2 * time.Second)
	if budget, ok := ctx.Value(policyBudget{}).(time.Time); ok && budget.Before(until) {
		until = budget
	}
	bounded, cancel := context.WithDeadline(context.WithValue(ctx, hookContext{}, true), until)
	defer cancel()
	deadline, _ := bounded.Deadline()
	bounded = context.WithValue(bounded, policyDeadline{}, deadline)
	list, e := s.hooks(bounded, kind, typ)
	if e != nil {
		trace.Warn(ctx, "plugins", "资源策略回落内置逻辑", "chain", string(typ), "reason", "registry_unavailable")
		return ctx, nil
	}
	if len(list) == 0 {
		return ctx, nil
	}
	for _, h := range list {
		if h.instance == pluginactor.InstanceID(ctx) {
			continue
		} // Never RPC back into the blocked caller's single lane.
		snap, d, err := s.Descriptor(bounded, h.instance, kind, typ)
		if err != nil {
			trace.Warn(ctx, "plugins", "资源策略回落内置逻辑", "chain", string(typ), "reason", "unavailable_or_revoked")
			continue
		}
		raw, err := json.Marshal(input)
		if err != nil {
			return ctx, ErrInvalid
		}
		args := raw
		if kind == "chain" {
			args, err = json.Marshal(events.ChainEnvelope{Type: contracts.ChainEventType(typ), SchemaVersion: 1, Payload: raw})
		}
		if err != nil {
			return ctx, ErrInvalid
		}
		perHandler, stop := context.WithTimeout(bounded, 200*time.Millisecond)
		result, err := s.invokeSnapshot(perHandler, snap, d, args, "")
		stop()
		if err != nil {
			trace.Warn(ctx, "plugins", "资源策略回落内置逻辑", "chain", string(typ), "reason", "timeout_or_failed")
			continue
		}
		if !apply(bounded, result) {
			trace.Warn(ctx, "plugins", "拒绝越权资源策略结果并回落", "chain", string(typ), "reason", "invalid_result")
			continue
		}
		ctx = dbutil.WithCommitGuard(ctx, func(c context.Context, tx pgx.Tx) error { return s.fencePolicy(c, tx, snap, d) })
	}
	return ctx, nil
}
func (s Store) fencePolicy(ctx context.Context, tx pgx.Tx, snap Snapshot, d contracts.Descriptor) error {
	tag, e := tx.Exec(ctx, `UPDATE plugin_v2_instances i SET updated_at=i.updated_at FROM plugin_v2_definitions f WHERE i.id=$1 AND i.generation=$2 AND i.grant_revision=$3 AND i.desired_state='enabled' AND i.runtime_state='running' AND i.lease_until>clock_timestamp() AND f.id=i.plugin_id AND f.fingerprint=i.approved_fingerprint AND f.availability='available' AND EXISTS(SELECT 1 FROM plugin_v2_extensions x WHERE x.instance_id=i.id AND x.snapshot_id=$4) AND NOT EXISTS(SELECT 1 FROM unnest($5::text[]) AS needed(scope) WHERE NOT EXISTS(SELECT 1 FROM plugin_v2_grants g WHERE g.instance_id=i.id AND g.scope=needed.scope AND g.fingerprint=i.approved_fingerprint))`, snap.InstanceID, snap.Generation, snap.GrantRevision, snap.ID, d.RequiredScopes)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
func (s Store) SelectResources(ctx context.Context, v contracts.ResourceSelectionRequest) (context.Context, []string, error) {
	if contracts.ValidateSelectionRequest(v) != nil {
		return ctx, nil, ErrInvalid
	}
	ids := []string{}
	for _, c := range v.Candidates {
		ids = append(ids, c.ID)
	}
	ctx, e := s.acquisitionChain(ctx, contracts.ChainResourceSelection, &v, func(_ context.Context, raw json.RawMessage) bool {
		var next contracts.ResourceSelectionResult
		if contracts.Decode(raw, &next) != nil || contracts.ValidateSelection(v, next) != nil {
			return false
		}
		byID := map[string]contracts.ResourceCandidate{}
		for _, c := range v.Candidates {
			byID[c.ID] = c
		}
		v.Candidates = []contracts.ResourceCandidate{}
		for _, id := range next.IDs {
			v.Candidates = append(v.Candidates, byID[id])
		}
		ids = next.IDs
		return true
	})
	return ctx, ids, e
}
func (s Store) PrepareDownload(ctx context.Context, v contracts.ResourceDownloadRequest) (context.Context, bool, error) {
	if contracts.ValidateDownloadRequest(v) != nil {
		return ctx, false, ErrInvalid
	}
	allowed := true
	ctx, e := s.acquisitionChain(ctx, contracts.ChainResourceDownload, &v, func(_ context.Context, raw json.RawMessage) bool {
		var next contracts.ResourceDownloadResult
		if decodeDownloadPolicy(raw, &next) != nil || contracts.ValidateDownload(v, next) != nil {
			return false
		}
		allowed = allowed && next.Allow // Later handlers can never reverse a veto.
		v.CleanupAfterImport = next.CleanupAfterImport
		return true
	})
	if e == nil && !allowed {
		e = ErrResourceRejected
	}
	return ctx, v.CleanupAfterImport, e
}
func validAcquisitionResult(id string, args, result json.RawMessage) bool {
	var envelope events.ChainEnvelope
	if contracts.Decode(args, &envelope) != nil || string(envelope.Type) != id || envelope.SchemaVersion != 1 {
		return false
	}
	switch contracts.ChainEventType(id) {
	case contracts.ChainResourceSelection:
		var q contracts.ResourceSelectionRequest
		var r contracts.ResourceSelectionResult
		return contracts.Decode(envelope.Payload, &q) == nil && contracts.Decode(result, &r) == nil && contracts.ValidateSelection(q, r) == nil
	case contracts.ChainResourceDownload:
		var q contracts.ResourceDownloadRequest
		var r contracts.ResourceDownloadResult
		return contracts.Decode(envelope.Payload, &q) == nil && decodeDownloadPolicy(result, &r) == nil && contracts.ValidateDownload(q, r) == nil
	}
	return false
}

// Missing/null booleans are malformed policy results, not an implicit veto.
func decodeDownloadPolicy(raw json.RawMessage, out *contracts.ResourceDownloadResult) error {
	var required struct {
		Allow   *bool `json:"allow"`
		Cleanup *bool `json:"cleanupAfterImport"`
	}
	if contracts.Decode(raw, &required) != nil || required.Allow == nil || required.Cleanup == nil {
		return ErrInvalid
	}
	out.Allow = *required.Allow
	out.CleanupAfterImport = *required.Cleanup
	return nil
}
