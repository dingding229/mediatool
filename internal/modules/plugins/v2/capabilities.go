package v2

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
)

type Capability struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	State      string `json:"state"`
	NextAction string `json:"nextAction"`
	Reason     string `json:"reason"`
}

func capabilityState(in Instance, declared, granted, registered bool, last string) (string, string, string) {
	if !declared || in.Availability == Unsupported || in.Availability == Missing || in.Reason == DefinitionMissing || in.Reason == DefinitionUnsupported {
		return "unsupported", "inspect_definition", "definition_or_capability_unavailable"
	}
	if !granted || in.Reason == PermissionReview {
		return "authorization_required", "review_permissions", "administrator_grant_required"
	}
	if in.Desired != Enabled {
		return "disabled", "enable_instance", "administrator_enable_required"
	}
	if in.State == Backoff || in.Reason == WorkerFailed || in.Reason == ResourceLimit || in.Reason == DataUnavailable || last == "failed" || last == "unknown" {
		return "runtime_failed", "inspect_run", "runtime_outcome_requires_inspection"
	}
	if in.State != Running || !registered {
		return "starting", "wait", "no_current_runtime_snapshot"
	}
	return "ready", "none", "registered_and_granted"
}
func (s Store) Capabilities(ctx context.Context, id string) ([]Capability, error) {
	if !validUUID(id) {
		return nil, ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return nil, e
	}
	defer rollback(ctx, tx)
	in, e := readInstance(ctx, tx, id)
	if e != nil {
		return nil, e
	}
	if e = tx.QueryRow(ctx, `SELECT availability FROM plugin_v2_definitions WHERE id=$1`, in.PluginID).Scan(&in.Availability); e != nil {
		return nil, e
	}
	def, e := definition(in.PluginID)
	if e != nil {
		return []Capability{{Kind: "definition", ID: in.PluginID, State: "unsupported", NextAction: "inspect_definition", Reason: "definition_unavailable"}}, nil
	}
	rows, e := tx.Query(ctx, `SELECT g.scope FROM plugin_v2_grants g JOIN plugin_v2_instances i ON i.id=g.instance_id WHERE i.id=$1 AND g.fingerprint=i.approved_fingerprint`, id)
	if e != nil {
		return nil, e
	}
	grants := map[string]bool{}
	for rows.Next() {
		var scope string
		if e = rows.Scan(&scope); e != nil {
			rows.Close()
			return nil, e
		}
		grants[scope] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	var raw []byte
	var metadata contracts.Snapshot
	e = tx.QueryRow(ctx, `SELECT s.descriptors FROM plugin_v2_extensions x JOIN plugin_v2_snapshots s ON s.id=x.snapshot_id JOIN plugin_v2_instances i ON i.id=x.instance_id JOIN plugin_v2_definitions d ON d.id=i.plugin_id WHERE i.id=$1 AND i.generation=s.generation AND i.grant_revision=s.grant_revision AND i.lease_until>clock_timestamp() AND i.approved_fingerprint=d.fingerprint`, id).Scan(&raw)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return nil, e
	}
	if e == nil {
		if e = json.Unmarshal(raw, &metadata); e != nil {
			return nil, e
		}
	}
	rows, e = tx.Query(ctx, `SELECT DISTINCT ON(handler)handler,state FROM plugin_v2_runs WHERE instance_id=$1 ORDER BY handler,created_at DESC,id DESC`, id)
	if e != nil {
		return nil, e
	}
	last := map[string]string{}
	for rows.Next() {
		var handler, state string
		if e = rows.Scan(&handler, &state); e != nil {
			rows.Close()
			return nil, e
		}
		last[handler] = state
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	ext := def.Manifest.Extensions
	apiIDs := []string{}
	for _, route := range ext.API {
		apiIDs = append(apiIDs, route.ID)
	}
	groups := []struct {
		kind, scope string
		ids         []string
	}{{"api", "api.register", apiIDs}, {"service", "services.register", ext.Services}, {"command", "commands.register", ext.Commands}, {"action", "actions.register", ext.Actions}, {"event", "", ext.Events}, {"chain", "", ext.Chains}, {"module", "", ext.Modules}, {"tool", "agent.tools", ext.AgentTools}, {"authProvider", "auth.provider", ext.AuthProviders}}
	out := []Capability{}
	for _, group := range groups {
		if len(group.ids) == 0 {
			out = append(out, Capability{Kind: group.kind, State: "unsupported", NextAction: "none", Reason: "not_declared"})
			continue
		}
		for _, key := range group.ids {
			registered, granted, run := false, group.scope == "" || grants[group.scope], ""
			if group.kind == "event" {
				granted = grants["events.subscribe."+key]
			}
			if group.kind == "chain" {
				granted = grants["events.chain."+key]
			}
			if group.kind == "module" {
				granted = grants["module."+key]
			}
			for _, d := range metadata.Descriptors {
				if d.Kind == group.kind && (d.ID == key || group.kind == "command" && d.Cmd == key) || group.kind == "authProvider" && d.ProviderID == key {
					registered = true
					run = last[d.Handler]
					for _, scope := range d.RequiredScopes {
						granted = granted && grants[scope]
					}
					break
				}
			}
			state, action, reason := capabilityState(in, true, granted, registered, run)
			out = append(out, Capability{Kind: group.kind, ID: key, State: state, NextAction: action, Reason: reason})
		}
	}
	return out, nil
}
