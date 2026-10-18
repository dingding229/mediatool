package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"media-tool/internal/platform/pluginactor"
	"media-tool/internal/platform/pluginstate"
	"strings"
	"time"
)

var ErrEffectUnknown = errors.New("plugin effect outcome unknown; operator verification required")

type effectState struct {
	RequestHash string `json:"requestHash"`
	State       string `json:"state"`
	InstanceID  string `json:"instanceId"`
	SnapshotID  string `json:"snapshotId"`
}

// ExternalEffect is not a pure chain: dispatch errors NEVER fall through to a
// second sender. The durable started record fences crashes and job retries.
func (s Store) ExternalEffect(ctx context.Context, module string, q contracts.EffectRequest) (contracts.EffectReceipt, error) {
	fallback := contracts.EffectReceipt{State: "not_started"}
	if module != "notifications.channel" && module != "mediaserver.refresh" || !contracts.ValidEffectRequest(q) {
		return fallback, ErrInvalid
	}
	budget := 10 * time.Second
	if module == "mediaserver.refresh" {
		budget = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	list, e := s.hooks(ctx, "module", module)
	if e != nil {
		return fallback, e
	}
	var snap Snapshot
	var d contracts.Descriptor
	found := false
	for _, h := range list {
		if h.instance == pluginactor.InstanceID(ctx) {
			continue
		}
		snap, d, e = s.Descriptor(ctx, h.instance, "module", module)
		if e == nil {
			found = true
			break
		}
	}
	request, _ := json.Marshal(q)
	requestHash := digest(append([]byte(module+"\x00"), request...))
	key := "plugins.effects." + module + "." + q.RequestID
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return fallback, e
	}
	defer rollback(ctx, tx)
	st := effectState{}
	if e = pluginstate.Lock(ctx, tx, key, &st); e != nil {
		return fallback, e
	}
	if st.State != "" && st.RequestHash != requestHash {
		return fallback, ErrConflict
	}
	if st.State != "" {
		switch st.State {
		case "not_started", "sent", "requested":
			return contracts.EffectReceipt{State: st.State}, tx.Commit(ctx)
		default:
			return contracts.EffectReceipt{State: "unknown"}, ErrEffectUnknown
		}
	}
	if !found {
		return fallback, nil
	}
	if e = s.fencePolicy(ctx, tx, snap, d); e != nil {
		return fallback, e
	}
	st = effectState{RequestHash: requestHash, State: "offering", InstanceID: snap.InstanceID, SnapshotID: snap.ID}
	if e = pluginstate.Put(ctx, tx, key, st); e != nil {
		return fallback, e
	}
	if e = dbutil.Commit(ctx, tx); e != nil {
		return fallback, e
	}
	// The offer is contractually pure. No execute RPC has been dispatched, so an
	// unavailable/declining offer can safely choose the builtin provider.
	offer := q
	offer.Phase = "offer"
	raw, _ := json.Marshal(offer)
	raw, offerErr := s.invokeSnapshot(ctx, snap, d, raw, "effect-offer-"+q.RequestID)
	decision := contracts.EffectReceipt{}
	validOffer := offerErr == nil && contracts.Decode(raw, &decision) == nil && contracts.ValidateEffect(offer, decision) == nil
	ready := validOffer && decision.State == "ready"
	declined := validOffer && decision.State == "not_started"
	save, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	tx, e = s.DB.Begin(save)
	if e != nil {
		stop()
		return fallback, ErrEffectUnknown
	}
	defer rollback(save, tx)
	current := effectState{}
	if e = pluginstate.Lock(save, tx, key, &current); e != nil {
		stop()
		return fallback, ErrEffectUnknown
	}
	if current.State != "offering" || current.SnapshotID != snap.ID {
		stop()
		return fallback, ErrEffectUnknown
	}
	if ready && s.fencePolicy(save, tx, snap, d) != nil {
		ready = false
		declined = true
	}
	current.State = "unknown"
	if declined {
		current.State = "not_started"
	}
	if ready {
		current.State = "started"
	}
	if e = pluginstate.Put(save, tx, key, current); e != nil {
		stop()
		return fallback, ErrEffectUnknown
	}
	if e = dbutil.Commit(save, tx); e != nil {
		stop()
		return fallback, ErrEffectUnknown
	}
	stop()
	if !ready {
		if !declined {
			return contracts.EffectReceipt{State: "unknown"}, ErrEffectUnknown
		}
		return fallback, nil
	}
	execution := q
	execution.Phase = "execute"
	raw, _ = json.Marshal(execution)
	raw, e = s.invokeSnapshot(ctx, snap, d, raw, "effect-execute-"+q.RequestID)
	receipt := contracts.EffectReceipt{State: "unknown"}
	if e == nil {
		if contracts.Decode(raw, &receipt) != nil || contracts.ValidateEffect(execution, receipt) != nil {
			receipt.State = "unknown"
		}
	}
	save, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancelSave()
	tx, e = s.DB.Begin(save)
	if e != nil {
		return receipt, ErrEffectUnknown
	}
	defer rollback(save, tx)
	if e = pluginstate.Lock(save, tx, key, &current); e != nil {
		return receipt, ErrEffectUnknown
	}
	if current.State != "started" || current.SnapshotID != snap.ID {
		return receipt, ErrEffectUnknown
	}
	if e = s.fencePolicy(save, tx, snap, d); e != nil {
		receipt.State = "unknown"
	}
	current.State = receipt.State
	if e = pluginstate.Put(save, tx, key, current); e != nil {
		return receipt, ErrEffectUnknown
	}
	if e = dbutil.Commit(save, tx); e != nil {
		return receipt, ErrEffectUnknown
	}
	if receipt.State == "unknown" {
		return receipt, ErrEffectUnknown
	}
	return receipt, nil
}
func (s Store) HasNotificationModule(ctx context.Context) bool {
	list, e := s.hooks(ctx, "module", "notifications.channel")
	return e == nil && len(list) > 0
}
func validEffectResult(id string, args, result json.RawMessage) bool {
	if id != "notifications.channel" && id != "mediaserver.refresh" {
		return false
	}
	var q contracts.EffectRequest
	var r contracts.EffectReceipt
	return contracts.Decode(args, &q) == nil && contracts.Decode(result, &r) == nil && contracts.ValidateEffect(q, r) == nil
}
func (s Store) CommandRoute(ctx context.Context, cmd string) (Snapshot, contracts.Descriptor, error) {
	if cmd == "" || len(cmd) > 64 || strings.ContainsAny(cmd, "/ \r\n\x00") {
		return Snapshot{}, contracts.Descriptor{}, ErrInvalid
	}
	list, e := s.hooks(ctx, "command", cmd)
	if e != nil {
		return Snapshot{}, contracts.Descriptor{}, e
	}
	if len(list) != 1 {
		return Snapshot{}, contracts.Descriptor{}, ErrNotFound
	}
	return s.Descriptor(ctx, list[0].instance, "command", cmd)
}

// QueueCommandTx commits the input receipt, command mailbox and live grants in
// the caller's ingress transaction. No RPC is performed while holding it.
func (s Store) QueueCommandTx(ctx context.Context, tx pgx.Tx, snap Snapshot, d contracts.Descriptor, args contracts.CommandInput, key string) error {
	if d.Kind != "command" || len([]rune(args.Arguments)) > 200 || args.Channel != "telegram" || len(args.Actor) > 100 {
		return ErrInvalid
	}
	raw, _ := json.Marshal(args)
	_, e := s.submitTx(ctx, tx, snap, d, raw, key, "", nil, time.Now().Add(10*time.Minute))
	return e
}
func emitPluginConfig(ctx context.Context, tx pgx.Tx, id string, rev int64) error {
	return outbox.EmitVersionTx(ctx, tx, "plugin.config.changed", id, fmt.Sprint(rev), contracts.PluginConfigChanged{InstanceID: id, Revision: rev})
}
func eventMayDeliver(typ string, raw json.RawMessage, instance string) bool {
	if contracts.TargetedMessage(contracts.EventType(typ)) {
		var m contracts.InboundMessage
		return contracts.Decode(raw, &m) == nil && m.InstanceID == instance
	}
	// All queued facts are journaled, but plugin-origin notifications do not feed
	// the notification event bus back into another plugin's post_message loop.
	if typ == "notification.queued" {
		var n contracts.NotificationQueued
		return contracts.Decode(raw, &n) == nil && n.Origin == "builtin"
	}
	return true
}

// BuiltinEffect journals the fallback too: a successful remote refresh followed
// by a local DB error must not cause the retried job to request another scan.
// Any interrupted/network-failed attempt remains unknown, never auto-replayed.
func (s Store) BuiltinEffect(ctx context.Context, module string, q contracts.EffectRequest, call func(context.Context) error) error {
	if module != "mediaserver.refresh" || !contracts.ValidEffectRequest(q) {
		return ErrInvalid
	}
	request, _ := json.Marshal(q)
	requestHash := digest(append([]byte(module+"\x00"), request...))
	key := "plugins.effects." + module + "." + q.RequestID
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	st := effectState{}
	if e = pluginstate.Lock(ctx, tx, key, &st); e != nil {
		return e
	}
	if st.State != "" && st.RequestHash != requestHash {
		return ErrConflict
	}
	if st.State == "requested" {
		return tx.Commit(ctx)
	}
	if st.State != "" && st.State != "not_started" {
		return ErrEffectUnknown
	}
	if e = dbutil.CheckCommit(ctx, tx); e != nil {
		return e
	}
	st.State = "builtin_started"
	st.RequestHash = requestHash
	if e = pluginstate.Put(ctx, tx, key, st); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	result := call(ctx)
	save, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	tx, e = s.DB.Begin(save)
	if e != nil {
		return ErrEffectUnknown
	}
	defer rollback(save, tx)
	if e = pluginstate.Lock(save, tx, key, &st); e != nil {
		return ErrEffectUnknown
	}
	if st.State != "builtin_started" {
		return ErrEffectUnknown
	}
	st.State = "requested"
	if result != nil {
		st.State = "unknown"
	}
	if e = pluginstate.Put(save, tx, key, st); e != nil {
		return ErrEffectUnknown
	}
	if e = dbutil.Commit(save, tx); e != nil {
		return ErrEffectUnknown
	}
	if result != nil {
		return ErrEffectUnknown
	}
	return nil
}
