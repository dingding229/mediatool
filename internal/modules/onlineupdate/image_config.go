package onlineupdate

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
)

// Labels are explicitly metadata, not startup settings. Version/commit/build
// time live there and in ldflags. Every other Config key, including future keys,
// is compared without dropping nils, reordering Env, or hiding version values.
var ErrImageConfigChanged = errors.New("镜像启动配置已变化，请按文档手动更新")

func sameImageStartupConfig(current, target map[string]any) bool {
	if current == nil || target == nil {
		return false
	}
	copyConfig := func(in map[string]any) map[string]any {
		out := make(map[string]any, len(in))
		for k, v := range in {
			if k != "Labels" {
				out[k] = v
			}
		}
		return out
	}
	return reflect.DeepEqual(copyConfig(current), copyConfig(target))
}

func (d *docker) imageStartupPreflight(ctx context.Context) error {
	// Use the running container's immutable image ID, not its overridden Config
	// or a tag. Retain target as the last inspect so create's existing cache and
	// privilege allowlist continue to use target-image defaults.
	old, e := d.request(ctx, "GET", "/images/"+d.self.Image+"/json", nil)
	if e != nil {
		return e
	}
	next, e := d.request(ctx, "GET", "/images/"+d.target+"/json", nil)
	if e != nil {
		return e
	}
	var a, b struct {
		ID     string `json:"Id"`
		Config map[string]any
	}
	if json.Unmarshal(old, &a) != nil || json.Unmarshal(next, &b) != nil || a.ID != d.self.Image || b.ID == "" || a.Config == nil || b.Config == nil {
		return ErrUnsafe
	}
	if !sameImageStartupConfig(a.Config, b.Config) {
		return ErrImageConfigChanged
	}
	return nil
}
