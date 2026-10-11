package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/sites"
	"net/http"
	"strconv"
	"time"
)

func (a *API) searchEngine() *search.Engine {
	if a.Deps.Integrations == nil {
		return nil
	}
	return &search.Engine{DB: a.Deps.DB, Vault: a.Deps.Integrations.Repository.Vault, Connections: a.Deps.Integrations, Redis: a.Deps.Redis}
}
func (a *API) listDefinitions(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	list, err := (sites.Repository{DB: a.Deps.DB}).List(r.Context())
	if err != nil {
		fail(w, 503, "sites_unavailable", "站点配置暂不可用")
		return
	}
	write(w, 200, list)
}
func (a *API) importDefinitions(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var body struct {
		LicenseAccepted bool     `json:"licenseAccepted"`
		IDs             []string `json:"ids"`
		SourceCommit    string   `json:"sourceCommit"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !body.LicenseAccepted {
		fail(w, 400, "license_confirmation_required", "请先阅读并确认上游配置库的使用授权")
		return
	}
	if err := sites.ValidateImport(body.IDs, body.SourceCommit); err != nil {
		fail(w, 400, "site_import_invalid", "每批最多30个不重复的站点ID；指定来源提交时须使用完整40位SHA")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	list, err := (sites.Repository{DB: a.Deps.DB}).ImportSnapshot(ctx, body.IDs, body.SourceCommit)
	if err != nil {
		if errors.Is(err, sites.ErrImportCommitUnconfirmed) {
			fail(w, 503, "site_import_unconfirmed", "保存结果未确认，请先刷新配置列表核对来源提交，不要直接重试或切换新快照")
			return
		}
		fail(w, 502, "site_import_failed", "站点配置导入失败，请检查仓库网络、站点 ID 和兼容性")
		return
	}
	write(w, 200, list)
}
func (a *API) startSearch(w http.ResponseWriter, r *http.Request) {
	engine := a.searchEngine()
	if engine == nil {
		fail(w, 503, "search_unavailable", "请先连接数据库并设置凭据加密密钥")
		return
	}
	var body struct {
		Keyword    string   `json:"keyword"`
		AccountIDs []string `json:"accountIds"`
		Refresh    bool     `json:"refresh"`
	}
	if !decode(w, r, &body) {
		return
	}
	id, cached, err := engine.StartCached(r.Context(), body.Keyword, body.AccountIDs, body.Refresh)
	if err != nil {
		fail(w, 400, "search_create_failed", err.Error())
		return
	}
	write(w, 202, map[string]any{"id": id, "cached": cached})
}
func (a *API) listSearches(w http.ResponseWriter, r *http.Request) {
	engine := a.searchEngine()
	if engine == nil {
		fail(w, 503, "search_unavailable", "请先连接数据库")
		return
	}
	runs, err := engine.List(r.Context())
	if err != nil {
		fail(w, 503, "search_unavailable", "搜索记录暂不可用")
		return
	}
	write(w, 200, runs)
}
func (a *API) getSearch(w http.ResponseWriter, r *http.Request) {
	engine := a.searchEngine()
	if engine == nil {
		fail(w, 503, "search_unavailable", "请先连接数据库")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	run, err := engine.Get(r.Context(), r.PathValue("id"), page, limit)
	if err != nil {
		if errors.Is(err, search.ErrSnapshotUnavailable) {
			fail(w, 409, "search_refresh_required", err.Error())
			return
		}
		fail(w, 404, "search_not_found", "搜索记录不存在或暂不可用")
		return
	}
	write(w, 200, run)
}
