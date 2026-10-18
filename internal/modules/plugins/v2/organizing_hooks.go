package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
)

var ErrOrganizeRejected = errors.New("插件整理策略拒绝或跳过本次操作")

func (s Store) TransferDecision(ctx context.Context, typ contracts.ChainEventType, q contracts.TransferPlan) (context.Context, error) {
	if typ != contracts.ChainTransferIntercept && typ != contracts.ChainTransferOverwriteCheck || contracts.ValidateTransfer(q) != nil {
		return ctx, ErrInvalid
	}
	allowed := true
	ctx, e := s.acquisitionChain(ctx, typ, q, func(_ context.Context, raw json.RawMessage) bool {
		var next contracts.TransferDecision
		if contracts.Decode(raw, &next) != nil || contracts.ValidateDecision(next) != nil {
			return false
		}
		allowed = allowed && next.Decision == "pass"
		return true
	})
	if e == nil && !allowed {
		e = ErrOrganizeRejected
	}
	return ctx, e
}
func (s Store) TransferPaths(ctx context.Context, typ contracts.ChainEventType, q contracts.TransferPlan, validate func(context.Context, []string) bool) (context.Context, []string, error) {
	if typ != contracts.ChainTransferRename && typ != contracts.ChainTransferRenameBuild || contracts.ValidateTransfer(q) != nil {
		return ctx, nil, ErrInvalid
	}
	ctx, e := s.acquisitionChain(ctx, typ, &q, func(checkCtx context.Context, raw json.RawMessage) bool {
		var next contracts.TransferPaths
		if contracts.Decode(raw, &next) != nil || contracts.ValidateTransferPaths(q, next) != nil || !validate(checkCtx, next.Paths) {
			return false
		}
		q.Paths = next.Paths
		return true
	})
	return ctx, q.Paths, e
}

// Only host-approved opaque handles enter the DTO. Callers may deliberately
// offer just the already-selected root: a hook cannot expand actor authority.
func (s Store) SelectStorage(ctx context.Context, q contracts.StorageSelection) (context.Context, string, error) {
	if contracts.ValidateStorage(q, contracts.StorageChoice{Handle: q.Selected}) != nil {
		return ctx, "", ErrInvalid
	}
	ctx, e := s.acquisitionChain(ctx, contracts.ChainStorageOperSelection, &q, func(_ context.Context, raw json.RawMessage) bool {
		var next contracts.StorageChoice
		if contracts.Decode(raw, &next) != nil || contracts.ValidateStorage(q, next) != nil {
			return false
		}
		q.Selected = next.Handle
		return true
	})
	return ctx, q.Selected, e
}
func validOrganizingResult(id string, args, result json.RawMessage) bool {
	var envelope struct {
		Type          contracts.ChainEventType `json:"type"`
		SchemaVersion int                      `json:"schemaVersion"`
		Payload       json.RawMessage          `json:"payload"`
	}
	if contracts.Decode(args, &envelope) != nil || string(envelope.Type) != id || envelope.SchemaVersion != 1 {
		return false
	}
	if envelope.Type == contracts.ChainStorageOperSelection {
		var q contracts.StorageSelection
		var r contracts.StorageChoice
		return contracts.Decode(envelope.Payload, &q) == nil && contracts.Decode(result, &r) == nil && contracts.ValidateStorage(q, r) == nil
	}
	var q contracts.TransferPlan
	if contracts.Decode(envelope.Payload, &q) != nil || contracts.ValidateTransfer(q) != nil {
		return false
	}
	switch envelope.Type {
	case contracts.ChainTransferRename, contracts.ChainTransferRenameBuild:
		var r contracts.TransferPaths
		return contracts.Decode(result, &r) == nil && contracts.ValidateTransferPaths(q, r) == nil
	case contracts.ChainTransferIntercept, contracts.ChainTransferOverwriteCheck:
		var r contracts.TransferDecision
		return contracts.Decode(result, &r) == nil && contracts.ValidateDecision(r) == nil
	}
	return false
}
