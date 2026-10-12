package httpapi

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/plugins"
	"net/http"
	"time"
)

func (a *API) pluginRepositories(w http.ResponseWriter, r *http.Request) {
	if a.Deps.DB == nil {
		fail(w, 503, "market_unavailable", "请先连接数据库")
		return
	}
	list, e := a.pluginRegistry().Repositories(r.Context())
	if e != nil {
		fail(w, 503, "market_unavailable", "插件仓库暂不可用")
		return
	}
	write(w, 200, list)
}
func (a *API) addPluginRepository(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if e := a.pluginRegistry().AddRepository(ctx, in.Name, in.URL); e != nil {
		fail(w, 400, "invalid_repository", "仓库不可用；只支持本程序HTTPS公开插件清单")
		return
	}
	write(w, 201, map[string]bool{"saved": true})
}
func (a *API) deletePluginRepository(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("confirm") != "true" {
		fail(w, 400, "confirmation_required", "请确认删除仓库配置")
		return
	}
	_, e := a.Deps.DB.Exec(r.Context(), `DELETE FROM extension_repositories WHERE id=$1`, r.PathValue("id"))
	if e != nil {
		fail(w, 400, "invalid_repository", "仓库删除失败")
		return
	}
	write(w, 200, map[string]bool{"deleted": true})
}
func (a *API) pluginMarket(w http.ResponseWriter, r *http.Request) {
	repos, e := a.pluginRegistry().Repositories(r.Context())
	if e != nil {
		fail(w, 503, "market_unavailable", "仓库不可用")
		return
	}
	for _, repo := range repos {
		if repo.ID == r.PathValue("id") {
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			list, e := plugins.FetchCatalog(ctx, repo.URL)
			if e != nil {
				fail(w, 502, "market_fetch_failed", "插件市场清单获取失败")
				return
			}
			write(w, 200, list)
			return
		}
	}
	fail(w, 404, "repository_not_found", "仓库不存在")
}
func (a *API) marketPreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RepositoryID string `json:"repositoryId"`
		PluginID     string `json:"pluginId"`
		Digest       string `json:"digest"`
		Revision     int64  `json:"revision"`
		Confirm      bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	p, e := a.pluginRegistry().MarketPackage(ctx, in.RepositoryID, in.PluginID)
	if e != nil {
		fail(w, 502, "package_fetch_failed", "包获取/摘要校验失败；没有安装")
		return
	}
	if !in.Confirm {
		write(w, 200, p)
		return
	}
	if in.Digest != p.Digest || a.pluginRegistry().Install(ctx, p, in.Revision) != nil {
		fail(w, 409, "package_changed", "包或修订改变；请重新预览")
		return
	}
	write(w, 200, map[string]bool{"installed": true, "enabled": false})
}
func (a *API) pluginState(w http.ResponseWriter, r *http.Request) {
	out, e := a.pluginRegistry().FrameworkState(r.Context(), r.PathValue("id"))
	if e != nil {
		fail(w, 404, "plugin_not_found", "插件不存在")
		return
	}
	write(w, 200, out)
}
func (a *API) clearPluginData(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
		Confirm  bool  `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || a.pluginRegistry().ResetData(r.Context(), r.PathValue("id"), in.Revision) != nil {
		fail(w, 409, "plugin_conflict", "请确认清理或刷新修订")
		return
	}
	write(w, 200, map[string]bool{"cleared": true, "enabled": false})
}
func (a *API) pluginFrameworkCall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	kind, key := r.PathValue("kind"), r.PathValue("key")
	allowed := false
	list, e := a.pluginRegistry().List(r.Context())
	if e != nil {
		fail(w, 503, "plugin_unavailable", "插件不可用")
		return
	}
	event := ""
	payload := map[string]any{}
	if r.Method == "POST" {
		var in struct {
			Confirm bool           `json:"confirm"`
			Data    map[string]any `json:"data"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !in.Confirm {
			fail(w, 400, "confirmation_required", "请明确执行插件操作")
			return
		}
		payload = in.Data
		if payload == nil {
			payload = map[string]any{}
		}
	}
	for _, item := range list {
		if item.Manifest.ID != id || !item.Enabled || item.Manifest.Runtime != "wasm" {
			continue
		}
		m := item.Manifest
		switch kind {
		case "page":
			for _, p := range m.Pages {
				if p.ID == key {
					allowed = true
				}
			}
			event = "plugin.page"
			payload["page"] = key
		case "dashboard":
			allowed = m.Dashboard
			event = "plugin.dashboard"
		case "command":
			for _, c := range m.Commands {
				if c.ID == key {
					allowed = true
				}
			}
			event = "plugin.command"
			payload["command"] = key
		case "api":
			for _, route := range m.Routes {
				if route.Path == key && route.Method == r.Method {
					allowed = true
				}
			}
			event = "plugin.api"
			payload["path"] = key
			payload["method"] = r.Method
			payload["query"] = r.URL.Query()
		}
	}
	if !allowed {
		fail(w, 404, "plugin_route_unavailable", "插件未启用或没有声明该能力")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	raw, e := a.pluginRegistry().Invoke(ctx, id, event, payload, false)
	if e != nil {
		fail(w, 502, "plugin_execution_failed", "插件执行未取得有效结果，请查看记录，不会自动重发")
		return
	}
	var out any
	if len(raw) > 0 && json.Unmarshal(raw, &out) != nil {
		fail(w, 502, "plugin_output_invalid", "插件结果无效")
		return
	}
	write(w, 200, out)
}
func (a *API) pluginServiceRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
		Confirm  bool  `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || a.pluginRegistry().RunService(r.Context(), r.PathValue("id"), r.PathValue("key"), in.Revision) != nil {
		fail(w, 409, "plugin_service_conflict", "插件服务未启用或修订改变")
		return
	}
	write(w, 202, map[string]bool{"queued": true})
}

func (a *API) pluginUI(w http.ResponseWriter, r *http.Request) {
	var html string
	var enabled bool
	var raw []byte
	if a.Deps.DB.QueryRow(r.Context(), `SELECT ui_html,enabled,manifest FROM extension_plugins WHERE id=$1`, r.PathValue("id")).Scan(&html, &enabled, &raw) != nil || !enabled || html == "" {
		fail(w, 404, "plugin_ui_unavailable", "插件UI未启用")
		return
	}
	var m plugins.Manifest
	if json.Unmarshal(raw, &m) != nil || !m.CustomUI {
		fail(w, 404, "plugin_ui_unavailable", "插件没有声明UI")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; connect-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'; sandbox allow-scripts")
	w.Write([]byte(html))
}

func (a *API) clonePlugin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Revision int64  `json:"revision"`
		Confirm  bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || a.pluginRegistry().Clone(r.Context(), r.PathValue("id"), in.ID, in.Name, in.Revision) != nil {
		fail(w, 409, "plugin_clone_conflict", "分身ID、配置修订或来源不满足要求")
		return
	}
	write(w, 201, map[string]bool{"cloned": true, "enabled": false})
}
