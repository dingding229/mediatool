package httpapi

import (
	"context"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/platform/secrets"
	"net/http"
	"path/filepath"
	"strconv"
	"time"
)

type PluginInstances interface {
	List(context.Context, int, int) ([]pluginv2.Instance, error)
	Apply(context.Context, string, pluginv2.Request) (pluginv2.Instance, error)
}

func (a *API) pluginInstances() (PluginInstances, error) {
	if a.Deps.PluginInstances != nil {
		return a.Deps.PluginInstances, nil
	}
	if a.Deps.DB == nil {
		return nil, pluginv2.ErrUnavailable
	}
	vault, e := secrets.New(a.Config.SecretKey)
	if e != nil {
		return nil, pluginv2.ErrUnavailable
	}
	root := a.Config.PluginDataRoot
	if root == "" {
		root = "/data/plugins"
	}
	if !filepath.IsAbs(root) {
		return nil, pluginv2.ErrUnavailable
	}
	return pluginv2.Store{DB: a.Deps.DB, Vault: vault, Root: root}, nil
}
func pluginV2Error(w http.ResponseWriter, e error) {
	var rpc *contracts.Error
	if errors.As(e, &rpc) && rpc.Code == contracts.PermissionDenied {
		fail(w, 403, "plugin_permission_denied", "插件扩展缺少已确认的权限")
		return
	}
	if errors.As(e, &rpc) && rpc.Code == contracts.Busy {
		w.Header().Set("Retry-After", "1")
		fail(w, 429, "plugin_busy", "插件执行队列繁忙")
		return
	}

	switch {
	case errors.Is(e, pluginv2.ErrInvalid):
		fail(w, 400, "invalid_plugin_request", "插件参数或权限确认无效")
	case errors.Is(e, pluginv2.ErrConflict):
		fail(w, 409, "plugin_conflict", "插件定义或修订已变化，请刷新并重新确认")
	case errors.Is(e, pluginv2.ErrNotFound):
		fail(w, 404, "plugin_not_found", "当前内置库中未找到插件或实例")
	default:
		fail(w, 503, "plugins_unavailable", "插件服务暂不可用")
	}
}
func (a *API) pluginV2Catalog(w http.ResponseWriter, r *http.Request) {
	start, limit, e := pluginPage(r)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := pluginv2.Catalog()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	total := len(out)
	if start > total {
		start = total
	}
	end := min(start+limit, total)
	page := out[start:end]
	if store, e := a.pluginInstances(); e == nil {
		if counter, ok := store.(interface {
			InstalledCounts(context.Context, []string) (map[string]int, error)
		}); ok {
			ids := []string{}
			for _, d := range page {
				ids = append(ids, d.Manifest.ID)
			}
			counts, e := counter.InstalledCounts(r.Context(), ids)
			if e != nil {
				pluginV2Error(w, e)
				return
			}
			for i := range page {
				n := counts[page[i].Manifest.ID]
				page[i].InstalledCount = &n
			}
		}
	}
	write(w, 200, map[string]any{"items": page, "total": total})
}
func (a *API) pluginV2List(w http.ResponseWriter, r *http.Request) {
	start, limit, e := pluginPage(r)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	s, e := a.pluginInstances()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := s.List(r.Context(), start, limit)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 200, out)
}
func (a *API) pluginV2Operation(w http.ResponseWriter, r *http.Request) {
	var in pluginv2.Request
	if !decode(w, r, &in) {
		return
	}
	s, e := a.pluginInstances()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	account, e := a.sessionAccount(r)
	if e != nil {
		fail(w, 401, "unauthorized", "请登录管理员")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	out, e := s.Apply(ctx, account.ID, in)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	status := 200
	if in.Kind == pluginv2.Enable || in.Kind == pluginv2.Disable || in.Kind == pluginv2.Configure {
		status = 202
	}
	write(w, status, out)
}
func (a *API) pluginV2Inspect(w http.ResponseWriter, r *http.Request) {
	s, e := a.pluginInstances()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	reader, ok := s.(interface {
		Inspect(context.Context, string, string) (pluginv2.Inspection, error)
	})
	if !ok {
		pluginV2Error(w, pluginv2.ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	out, e := reader.Inspect(ctx, r.PathValue("id"), r.URL.Query().Get("after"))
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 200, out)
}

func pluginPage(r *http.Request) (int, int, error) {
	page, limit := 1, 50
	for _, field := range []struct {
		key   string
		value *int
	}{{"page", &page}, {"pageSize", &limit}} {
		if values, ok := r.URL.Query()[field.key]; ok {
			if len(values) != 1 {
				return 0, 0, pluginv2.ErrInvalid
			}
			v, e := strconv.Atoi(values[0])
			if e != nil {
				return 0, 0, pluginv2.ErrInvalid
			}
			*field.value = v
		}
	}
	if page < 1 || page > 10001 || limit < 1 || limit > 200 || (page-1)*limit > 10000 {
		return 0, 0, pluginv2.ErrInvalid
	}
	return (page - 1) * limit, limit, nil
}

func (a *API) pluginV2Resources(w http.ResponseWriter, r *http.Request) {
	write(w, 200, pluginv2.Resources())
}
