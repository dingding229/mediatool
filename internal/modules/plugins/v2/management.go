package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"strings"
)

type WorkflowSummary struct {
	ID          string `json:"id"`
	Revision    int64  `json:"revision"`
	Fingerprint string `json:"fingerprint"`
	Approved    string `json:"approved"`
	Steps       int    `json:"steps"`
}

func (r WorkflowRunner) List(ctx context.Context, page int) ([]WorkflowSummary, error) {
	if page < 1 || page > 1000 {
		return nil, ErrInvalid
	}
	rows, e := r.Store.DB.Query(ctx, `SELECT value FROM app_settings WHERE key ~ '^plugins\.workflow\.[a-zA-Z0-9_-]{1,64}$' ORDER BY key LIMIT 20 OFFSET $1`, (page-1)*20)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []WorkflowSummary{}
	for rows.Next() {
		var raw []byte
		var v WorkflowDefinition
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if json.Unmarshal(raw, &v) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, WorkflowSummary{v.DAG.ID, v.Revision, v.Fingerprint, v.Approved, len(v.DAG.Steps)})
	}
	return out, rows.Err()
}
func (r WorkflowRunner) Definition(ctx context.Context, id string) (WorkflowDefinition, error) {
	var v WorkflowDefinition
	if !workflowID(id) {
		return v, ErrInvalid
	}
	e := r.load(ctx, "plugins.workflow."+id, &v)
	return v, e
}
func workflowID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, v := range id {
		if !(v >= 'a' && v <= 'z' || v >= 'A' && v <= 'Z' || v >= '0' && v <= '9' || v == '_' || v == '-') {
			return false
		}
	}
	return true
}

type WorkflowRunView struct {
	contracts.WorkflowRun
	Steps   map[string]string `json:"steps"`
	Reasons map[string]string `json:"reasons"`
}

func (r WorkflowRunner) Runs(ctx context.Context, id string, page int) ([]WorkflowRunView, error) {
	if !workflowID(id) || page < 1 || page > 1000 {
		return nil, ErrInvalid
	}
	rows, e := r.Store.DB.Query(ctx, `SELECT value FROM app_settings WHERE key LIKE 'plugins.workflow.run.%' AND value->>'workflowId'=$1 ORDER BY updated_at DESC,key LIMIT 20 OFFSET $2`, id, (page-1)*20)
	if e != nil {
		return nil, e
	}
	list := []workflowExecution{}
	for rows.Next() {
		var raw []byte
		var v workflowExecution
		if e = rows.Scan(&raw); e != nil {
			break
		}
		if e = json.Unmarshal(raw, &v); e != nil {
			break
		}
		list = append(list, v)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if other != nil {
		return nil, other
	}
	out := []WorkflowRunView{}
	for _, v := range list {
		status, e := r.Status(ctx, id, v.ID)
		if e != nil {
			return nil, e
		}
		view := WorkflowRunView{WorkflowRun: status, Steps: map[string]string{}, Reasons: map[string]string{}}
		for _, n := range v.Definition.DAG.Steps {
			view.Steps[n.ID] = "pending"
		}
		rr, e := r.Store.DB.Query(ctx, `SELECT dedupe_key,state,reason FROM plugin_v2_runs WHERE dedupe_key LIKE $1 ORDER BY created_at LIMIT 32`, "workflow:"+v.ID+":%")
		if e != nil {
			return nil, e
		}
		for rr.Next() {
			var key, state, reason string
			if e = rr.Scan(&key, &state, &reason); e != nil {
				break
			}
			step := strings.TrimPrefix(key, "workflow:"+v.ID+":")
			view.Steps[step] = state
			view.Reasons[step] = reason
		}
		other = rr.Err()
		rr.Close()
		if e != nil {
			return nil, e
		}
		if other != nil {
			return nil, other
		}
		out = append(out, view)
	}
	return out, nil
}

type AuthProviderStatus struct {
	ID         string `json:"id"`
	InstanceID string `json:"instanceId"`
	Bound      int    `json:"bound"`
	Stale      int    `json:"stale"`
	State      string `json:"state"`
	Error      string `json:"error"`
}

func (b Broker) Status(ctx context.Context, instance string) ([]AuthProviderStatus, error) {
	if !validUUID(instance) {
		return nil, ErrInvalid
	}
	var plugin string
	if e := b.Store.DB.QueryRow(ctx, `SELECT plugin_id FROM plugin_v2_instances WHERE id=$1 AND desired_state<>'absent'`, instance).Scan(&plugin); e != nil {
		return nil, e
	}
	d, e := definition(plugin)
	if e != nil {
		return nil, e
	}
	out := []AuthProviderStatus{}
	for _, id := range d.Manifest.Extensions.AuthProviders {
		v := AuthProviderStatus{ID: id, InstanceID: instance, State: "unavailable", Error: "provider_inactive_or_scope_denied"}
		_, _, e := b.provider(ctx, instance, id)
		if e == nil {
			v.State = "registered"
			v.Error = "session_adapter_unsupported"
		}
		if e = b.Store.DB.QueryRow(ctx, `SELECT count(*) FILTER(WHERE value->>'Fingerprint'=$3 AND EXISTS(SELECT 1 FROM administrator_accounts a WHERE a.id::text=value->>'Account')),count(*) FILTER(WHERE value->>'Fingerprint'<>$3 OR NOT EXISTS(SELECT 1 FROM administrator_accounts a WHERE a.id::text=value->>'Account')) FROM app_settings WHERE key LIKE 'plugins.auth.binding.%' AND value->>'instanceId'=$1 AND value->>'Provider'=$2`, instance, id, d.Fingerprint).Scan(&v.Bound, &v.Stale); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
