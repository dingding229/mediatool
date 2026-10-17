package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"strings"
)

// UI runs only declared providers through the same generation/grant-fenced lane.
// It does not evaluate code, fetch network assets or return host secrets.
func (s Store) UI(ctx context.Context, id, kind, key string) (json.RawMessage, error) {
	d, e := s.Snapshot(ctx, id)
	if e != nil {
		return nil, e
	}
	var plugin string
	if e = s.DB.QueryRow(ctx, `SELECT plugin_id FROM plugin_v2_instances WHERE id=$1`, id).Scan(&plugin); e != nil {
		return nil, e
	}
	def, e := definition(plugin)
	if e != nil {
		return nil, e
	}
	descriptor := kind
	if kind == "page" || kind == "dashboard" {
		descriptor += "." + key
	} else if kind != "form" && kind != "meta" {
		return nil, ErrInvalid
	}
	raw, e := s.Invoke(ctx, d.InstanceID, "ui", descriptor, json.RawMessage(`{}`), "")
	if e != nil {
		return nil, e
	}
	if e = validateUI(def.Manifest, kind, raw); e != nil {
		return nil, e
	}
	return raw, nil
}
func validateUI(m contracts.Manifest, kind string, raw json.RawMessage) error {
	if len(raw) > 128<<10 {
		return ErrInvalid
	}
	if kind == "meta" {
		var list []contracts.DashboardMeta
		if contracts.Decode(raw, &list) != nil || len(list) > 16 {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for _, d := range list {
			declared := false
			for _, k := range m.Extensions.Dashboards {
				if k == d.Key {
					declared = true
				}
			}
			if !declared || seen[d.Key] || len(d.Title) > 120 || d.Cols < 1 || d.Cols > 12 || d.Refresh < 15 || d.Refresh > 86400 {
				return ErrInvalid
			}
			seen[d.Key] = true
		}
		return nil
	}
	var nodes []contracts.Component
	if kind == "form" {
		var f contracts.Form
		if contracts.Decode(raw, &f) != nil {
			return ErrInvalid
		}
		var v map[string]json.RawMessage
		if json.Unmarshal(f.Defaults, &v) != nil || v == nil {
			return ErrInvalid
		}
		nodes = f.Content
	} else {
		var p contracts.Page
		if contracts.Decode(raw, &p) != nil {
			return ErrInvalid
		}
		nodes = p.Content
	}
	count := 0
	var visit func([]contracts.Component, int) bool
	visit = func(ns []contracts.Component, depth int) bool {
		if depth > 12 {
			return false
		}
		for _, n := range ns {
			count++
			if count > 256 || len(n.Component) > 64 || len(n.Key) > 120 || !uiModel(n.Model) || len(n.Props) > 32 {
				return false
			}
			if n.When != nil && (!uiModel(n.When.Model) || len(n.When.Equals) > 4096 || contracts.ValidateJSON(n.When.Equals) != nil) {
				return false
			}
			for k, v := range n.Props {
				if len(k) > 64 || len(v) > 8192 || contracts.ValidateJSON(v) != nil {
					return false
				}
			}
			if !visit(n.Children, depth+1) {
				return false
			}
		}
		return true
	}
	if !visit(nodes, 0) {
		return ErrInvalid
	}
	return nil
}
func uiModel(s string) bool {
	return len(s) <= 64 && !strings.ContainsAny(s, ".[]/\x00") && s != "__proto__" && s != "prototype" && s != "constructor"
}

type UISurface struct {
	InstanceID string `json:"instanceId"`
	PluginID   string `json:"pluginId"`
}

// UISurfaces lists only live UI-capable workers, not an arbitrarily paginated
// slice of all installed instances. The supervisor has a global eight-worker cap.
func (s Store) UISurfaces(ctx context.Context) ([]UISurface, error) {
	rows, e := s.DB.Query(ctx, `SELECT i.id::text,i.plugin_id FROM plugin_v2_instances i JOIN plugin_v2_extensions x ON x.instance_id=i.id JOIN plugin_v2_snapshots p ON p.id=x.snapshot_id JOIN plugin_v2_definitions d ON d.id=i.plugin_id WHERE i.desired_state='enabled' AND i.runtime_state='running' AND i.generation=p.generation AND i.grant_revision=p.grant_revision AND i.lease_until>clock_timestamp() AND i.approved_fingerprint=d.fingerprint AND d.availability='available' AND EXISTS(SELECT 1 FROM plugin_v2_grants g WHERE g.instance_id=i.id AND g.scope='ui.json' AND g.fingerprint=i.approved_fingerprint) ORDER BY i.id LIMIT 8`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []UISurface{}
	for rows.Next() {
		var v UISurface
		if e = rows.Scan(&v.InstanceID, &v.PluginID); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
