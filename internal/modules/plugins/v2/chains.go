package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/dingding229/mediatool-plugin/sdk/events"
	"media-tool/internal/modules/releasename"
	"path"
	"strings"
	"time"
)

type hookContext struct{}
type hook struct {
	instance   string
	descriptor contracts.Descriptor
}

// hooks reads only current, enabled generations. At most eight hooks participate;
// overload fails back to the built-in implementation instead of a partial chain.
func (s Store) hooks(ctx context.Context, kind, id string) ([]hook, error) {
	if s.DB == nil {
		return nil, ErrUnavailable
	}
	observed := time.Now()
	key := kind + "/" + id
	epoch, absent := absentPolicy(s.DB, key)
	if absent {
		return []hook{}, nil
	}
	rows, e := s.DB.Query(ctx, `/* plugins:policy-hooks */ SELECT i.id::text,d FROM plugin_v2_extensions x JOIN plugin_v2_snapshots p ON p.id=x.snapshot_id JOIN plugin_v2_instances i ON i.id=x.instance_id JOIN plugin_v2_definitions f ON f.id=i.plugin_id, jsonb_array_elements(p.descriptors->'descriptors') d WHERE i.desired_state='enabled' AND i.runtime_state='running' AND i.generation=p.generation AND i.grant_revision=p.grant_revision AND i.lease_until>clock_timestamp() AND i.approved_fingerprint=f.fingerprint AND f.availability='available' AND d->>'kind'=$1 AND d->>'id'=$2 ORDER BY coalesce((d->>'priority')::int,0) DESC,i.id,d->>'handler' LIMIT 9`, kind, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	list := []hook{}
	for rows.Next() {
		var v hook
		var raw []byte
		if e = rows.Scan(&v.instance, &raw); e != nil {
			return nil, e
		}
		if json.Unmarshal(raw, &v.descriptor) != nil {
			return nil, ErrInvalid
		}
		list = append(list, v)
	}
	if len(list) > 8 {
		return nil, ErrUnavailable
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(list) == 0 {
		rememberAbsentPolicy(s.DB, key, epoch, observed.Add(hookAbsenceTTL))
	}
	return list, nil
}

// RecognizeName is a pure-hint extension point shared by workbench and library.
// IDs, paths, identity evidence and file operations never enter its contract.
// Each handler gets 200ms (queue time included), total chain+module <=2s.
func (s Store) RecognizeName(ctx context.Context, name string) releasename.Hint {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	base := releasename.Parse(name)
	if ctx.Value(hookContext{}) != nil {
		return base
	}
	ctx, cancel := context.WithTimeout(context.WithValue(ctx, hookContext{}, true), 2*time.Second)
	defer cancel()
	lists, err := s.nameHooks(ctx)
	if err != nil || len(lists["chain"])+len(lists["module"]) == 0 {
		return base
	}
	value := contracts.NameRecognition{Name: name, Hint: contracts.NameHint{Title: base.Title, Year: base.Year, MediaType: base.MediaType, Season: base.Season, Episodes: base.Episodes}, Continue: true}
	if contracts.ValidateName(value) != nil {
		return base
	}
	for _, kind := range []string{"chain", "module"} {
		key := "name.recognize"
		if kind == "module" {
			key = "metadata.name_recognize"
		}
		list := lists[kind]
		for _, h := range list {
			// Closed DTOs contain only strings, ints, bools and RawMessage just produced
			// by Marshal; they cannot contain unsupported values or cycles.
			args, _ := json.Marshal(value)
			if kind == "chain" {
				args, _ = json.Marshal(events.ChainEnvelope{Type: contracts.ChainNameRecognize, SchemaVersion: 1, Payload: args})
			}
			bounded, stop := context.WithTimeout(ctx, 200*time.Millisecond)
			raw, e := s.Invoke(bounded, h.instance, kind, key, args, "")
			stop()
			var next contracts.NameRecognition
			if e != nil || contracts.Decode(raw, &next) != nil || contracts.ValidateName(next) != nil {
				continue
			}
			value = next
			// A successful module override replaces built-in parse; chains may intercept.
			if !value.Continue || kind == "module" {
				return applyHint(base, value)
			}
		}
	}
	return applyHint(base, value)
}
func applyHint(base releasename.Hint, v contracts.NameRecognition) releasename.Hint {
	if v.Name != "" {
		base = releasename.Parse(v.Name)
	}
	base.Title = v.Hint.Title
	base.Year = v.Hint.Year
	base.MediaType = v.Hint.MediaType
	base.Season = v.Hint.Season
	base.Episodes = v.Hint.Episodes
	base.Episode = 0
	if len(base.Episodes) > 0 {
		base.Episode = base.Episodes[0]
	}
	return base
}

// ExecuteCommand is the internal entry point; callers must already authenticate
// and audit the operator. Telegram ingress is deliberately not implemented here.
func (s Store) ExecuteCommand(ctx context.Context, cmd string, args json.RawMessage, key string) (json.RawMessage, error) {
	list, e := s.hooks(ctx, "command", cmd)
	if e != nil {
		return nil, e
	}
	if len(list) != 1 {
		return nil, ErrNotFound
	}
	return s.Invoke(ctx, list[0].instance, "command", cmd, args, key)
}
