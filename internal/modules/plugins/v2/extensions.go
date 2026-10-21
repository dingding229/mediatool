package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"sort"
	"time"
)

type RunKind string

const (
	RunTool        RunKind = "tool"
	RunChain       RunKind = "chain"
	RunModule      RunKind = "module"
	RunUI          RunKind = "ui"
	RunAPI         RunKind = "api"
	RunServiceKind RunKind = "service"
	RunCommand     RunKind = "command"
	RunAction      RunKind = "action"
	RunEvent       RunKind = "event"
)

type RunState string

const (
	RunPending   RunState = "pending"
	RunRunning   RunState = "running"
	RunSucceeded RunState = "succeeded"
	RunFailed    RunState = "failed"
	RunUnknown   RunState = "unknown"
	RunCanceled  RunState = "canceled"
)

type RunReason string

const (
	RunNoReason      RunReason = ""
	RunRevoked       RunReason = "revoked"
	RunDeadline      RunReason = "deadline"
	RunWorkerLost    RunReason = "worker_lost"
	RunInvalidResult RunReason = "invalid_result"
	RunPluginFailed  RunReason = "plugin_failed"
)

type Snapshot struct {
	ID            string             `json:"id"`
	InstanceID    string             `json:"instanceId"`
	Generation    string             `json:"-"`
	GrantRevision int64              `json:"-"`
	Hash          string             `json:"hash"`
	Metadata      contracts.Snapshot `json:"metadata"`
}
type Run struct {
	ID         string     `json:"id"`
	InstanceID string     `json:"instanceId"`
	Kind       RunKind    `json:"kind"`
	Handler    string     `json:"handler"`
	State      RunState   `json:"state"`
	Reason     RunReason  `json:"reason"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

func (s Store) Publish(ctx context.Context, l Lease, metadata contracts.Snapshot) error {
	defer invalidateNameHooks(s.DB)
	d, e := definition(l.PluginID)
	if e != nil || d.Fingerprint != l.Fingerprint || contracts.ValidateSnapshot(d.Manifest, l.Scopes, metadata) != nil {
		return ErrInvalid
	}
	metadata.Descriptors = append([]contracts.Descriptor{}, metadata.Descriptors...)
	sort.Slice(metadata.Descriptors, func(i, j int) bool {
		a, b := metadata.Descriptors[i], metadata.Descriptors[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		return a.Handler < b.Handler
	})
	raw, e := json.Marshal(metadata)
	if e != nil || len(raw) > 128<<10 {
		return ErrInvalid
	}
	for _, d := range metadata.Descriptors {
		if d.Kind == "tool" && !b.ValidToolEffect(d.EffectKind, d.RequiredScopes) {
			return contracts.NewError(contracts.PermissionDenied)
		}
	}
	hash := digest(raw)
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7062001)`); e != nil {
		return e
	}
	if e = liveLease(ctx, tx, l); e != nil {
		return e
	}
	required := map[string]bool{}
	for _, d := range metadata.Descriptors {
		for _, scope := range d.RequiredScopes {
			required[scope] = true
		}
	}
	scopes := []string{}
	for scope := range required {
		scopes = append(scopes, scope)
	}
	var allowed int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_grants WHERE instance_id=$1 AND fingerprint=$2 AND scope=ANY($3::text[])`, l.ID, l.Fingerprint, scopes).Scan(&allowed); e != nil {
		return e
	}
	if allowed != len(required) {
		return ErrConflict
	}

	// Commands are global; API paths and services are namespaced by instance.
	for _, v := range metadata.Descriptors {
		if v.Kind != "command" {
			continue
		}
		var exists bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_extensions x JOIN plugin_v2_snapshots s ON s.id=x.snapshot_id JOIN plugin_v2_instances i ON i.id=x.instance_id, jsonb_array_elements(s.descriptors->'descriptors') d WHERE i.id<>$1 AND i.desired_state='enabled' AND i.generation=s.generation AND i.grant_revision=s.grant_revision AND i.lease_until>clock_timestamp() AND d->>'kind'='command' AND d->>'cmd'=$2)`, l.ID, v.Cmd).Scan(&exists); e != nil {
			return e
		}
		if exists {
			return ErrConflict
		}
	}
	id := jobs.UUID()
	var oldHash string
	e = tx.QueryRow(ctx, `SELECT id::text,descriptor_hash FROM plugin_v2_snapshots WHERE instance_id=$1 AND generation=$2`, l.ID, l.Token).Scan(&id, &oldHash)
	if e == nil {
		if oldHash != hash {
			return ErrConflict
		}
		return tx.Commit(ctx)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_snapshots(id,instance_id,generation,grant_revision,fingerprint,descriptor_hash,descriptors)VALUES($1,$2,$3,$4,$5,$6,$7)`, id, l.ID, l.Token, l.GrantRevision, l.Fingerprint, hash, raw); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO plugin_v2_extensions(instance_id,snapshot_id)VALUES($1,$2) ON CONFLICT(instance_id)DO UPDATE SET snapshot_id=excluded.snapshot_id`, l.ID, id); e != nil {
		return e
	}
	if e = s.publishSchedules(ctx, tx, l, id, metadata); e != nil {
		return e
	}
	if e = s.publishSubscriptions(ctx, tx, l, id, metadata); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func liveLease(ctx context.Context, tx pgx.Tx, l Lease) error {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT true FROM plugin_v2_instances i JOIN plugin_v2_definitions d ON d.id=i.plugin_id WHERE i.id=$1 AND i.generation=$2 AND i.owner_id=$3 AND i.grant_revision=$4 AND i.desired_state='enabled' AND i.lease_until>clock_timestamp() AND d.availability='available' AND i.approved_fingerprint=d.fingerprint AND i.approved_fingerprint=$5 FOR SHARE OF i`, l.ID, l.Token, l.Owner, l.GrantRevision, l.Fingerprint).Scan(&ok)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrConflict
	}
	return e
}

func (s Store) Snapshot(ctx context.Context, id string) (Snapshot, error) {
	var out Snapshot
	var raw []byte
	if !validUUID(id) {
		return out, ErrInvalid
	}
	e := s.DB.QueryRow(ctx, `SELECT s.id::text,s.instance_id::text,s.generation::text,s.grant_revision,s.descriptor_hash,s.descriptors FROM plugin_v2_extensions x JOIN plugin_v2_snapshots s ON s.id=x.snapshot_id JOIN plugin_v2_instances i ON i.id=x.instance_id JOIN plugin_v2_definitions d ON d.id=i.plugin_id WHERE i.id=$1 AND i.desired_state='enabled' AND i.runtime_state='running' AND i.generation=s.generation AND i.grant_revision=s.grant_revision AND i.lease_until>clock_timestamp() AND i.approved_fingerprint=d.fingerprint AND d.availability='available'`, id).Scan(&out.ID, &out.InstanceID, &out.Generation, &out.GrantRevision, &out.Hash, &raw)
	if errors.Is(e, pgx.ErrNoRows) {
		var available bool
		if check := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_instances i JOIN plugin_v2_definitions d ON d.id=i.plugin_id WHERE i.id=$1 AND i.desired_state='enabled' AND d.availability='available')`, id).Scan(&available); check != nil {
			return out, check
		}
		if !available {
			return out, accessError(ctx, s.DB, id)
		}
		return out, ErrUnavailable
	}
	if e != nil {
		return out, e
	}
	if json.Unmarshal(raw, &out.Metadata) != nil {
		return out, ErrUnavailable
	}
	return out, nil
}
func (s Store) Descriptor(ctx context.Context, id, kind, key string) (Snapshot, contracts.Descriptor, error) {
	snap, e := s.Snapshot(ctx, id)
	if e != nil {
		return snap, contracts.Descriptor{}, e
	}
	for _, d := range snap.Metadata.Descriptors {
		if d.Kind == kind && (d.ID == key || d.Handler == key || (kind == "api" && d.Path == key)) {
			if e = s.granted(ctx, id, snap, d); e != nil {
				return snap, d, e
			}
			return snap, d, nil
		}
	}
	return snap, contracts.Descriptor{}, ErrNotFound
}
func (s Store) granted(ctx context.Context, id string, snap Snapshot, d contracts.Descriptor) error {
	var n int
	e := s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_grants WHERE instance_id=$1 AND fingerprint=(SELECT approved_fingerprint FROM plugin_v2_instances WHERE id=$1) AND scope=ANY($2::text[])`, id, d.RequiredScopes).Scan(&n)
	if e != nil {
		return e
	}
	unique := map[string]bool{}
	for _, scope := range d.RequiredScopes {
		unique[scope] = true
	}
	if n != len(unique) {
		return contracts.NewError(contracts.PermissionDenied)
	}
	return nil
}
