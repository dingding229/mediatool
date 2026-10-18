package settings

import (
	"context"
	"encoding/json"
	"media-tool/internal/platform/pluginactor"
)

// Keep secret/security domains outside the wire, even if a plugin asks for an
// unknown key or submits a valid-looking full application-settings object.
func PluginKey(key string) bool {
	switch key {
	case "movieTemplate", "seriesTemplate", "searchCacheSeconds", "tmdbCacheSeconds":
		return true
	}
	return false
}
func (r Repository) PluginGet(ctx context.Context, keys []string) (map[string]json.RawMessage, int64, error) {
	if _, e := pluginactor.Require(ctx, "settings.read"); e != nil {
		return nil, 0, e
	}
	if len(keys) < 1 || len(keys) > 200 {
		return nil, 0, ErrInvalid
	}
	for _, k := range keys {
		if !PluginKey(k) {
			return nil, 0, ErrInvalid
		}
	}
	v, e := r.Get(ctx)
	if e != nil {
		return nil, 0, e
	}
	raw, _ := json.Marshal(v.Value)
	var all map[string]json.RawMessage
	_ = json.Unmarshal(raw, &all)
	out := map[string]json.RawMessage{}
	for _, k := range keys {
		out[k] = all[k]
	}
	return out, v.Revision, nil
}
func (r Repository) PluginPatch(ctx context.Context, values map[string]json.RawMessage, revision int64) (map[string]json.RawMessage, int64, error) {
	if _, e := pluginactor.Require(ctx, "settings.write"); e != nil {
		return nil, 0, e
	}
	if len(values) < 1 || len(values) > 200 {
		return nil, 0, ErrInvalid
	}
	keys := []string{}
	for k := range values {
		if !PluginKey(k) {
			return nil, 0, ErrInvalid
		}
		keys = append(keys, k)
	}
	current, e := r.Get(ctx)
	if e != nil {
		return nil, 0, e
	}
	if current.Revision != revision {
		return nil, 0, ErrRevision
	}
	raw, _ := json.Marshal(current.Value)
	var all map[string]json.RawMessage
	_ = json.Unmarshal(raw, &all)
	for k, v := range values {
		all[k] = v
	}
	raw, e = json.Marshal(all)
	if e != nil {
		return nil, 0, ErrInvalid
	}
	if e = json.Unmarshal(raw, &current.Value); e != nil {
		return nil, 0, ErrInvalid
	}
	v, e := r.Save(ctx, current.Value, revision)
	if e != nil {
		return nil, 0, e
	}
	out := map[string]json.RawMessage{}
	raw, _ = json.Marshal(v.Value)
	_ = json.Unmarshal(raw, &all)
	for _, k := range keys {
		out[k] = all[k]
	}
	return out, v.Revision, nil
}
