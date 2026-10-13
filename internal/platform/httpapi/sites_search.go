package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/sites"
	"net/http"
	"net/url"
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
	// Interactive search always covers every enabled account. Subscription searches retain their own scope.
	accounts, err := a.Deps.Integrations.Repository.List(r.Context())
	if err != nil {
		fail(w, 503, "sites_unavailable", "站点账号不可用")
		return
	}
	body.AccountIDs = nil
	for _, account := range accounts {
		if account.Kind == "ptsite" && account.Enabled {
			body.AccountIDs = append(body.AccountIDs, account.ID)
		}
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
	q, parseErr := url.ParseQuery(r.URL.RawQuery)
	if parseErr != nil {
		fail(w, 400, "invalid_filters", "筛选参数编码无效")
		return
	}
	page, limit := 1, 50
	for key, target := range map[string]*int{"page": &page, "limit": &limit} {
		if raw := q.Get(key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				fail(w, 400, "invalid_filters", "分页参数无效")
				return
			}
			*target = n
		}
	}
	if raw := q.Get("freeOnly"); raw != "" && raw != "true" && raw != "false" {
		fail(w, 400, "invalid_filters", "免费筛选参数无效")
		return
	}

	f := search.Filter{Resolutions: q["resolution"], Sources: q["source"], Codecs: q["codec"], AccountIDs: q["accountId"], Keyword: q.Get("keyword"), Sort: q.Get("sort"), Resolution: q.Get("resolution"), Source: q.Get("source"), Codec: q.Get("codec"), AccountID: q.Get("accountId"), FreeOnly: q.Get("freeOnly") == "true"}
	for key, target := range map[string]*int64{"minSize": &f.MinSize, "maxSize": &f.MaxSize} {
		if raw := q.Get(key); raw != "" {
			n, e := strconv.ParseInt(raw, 10, 64)
			if e != nil {
				fail(w, 400, "invalid_filters", "文件大小筛选无效")
				return
			}
			*target = n
		}
	}
	if raw := q.Get("minSeeders"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil {
			fail(w, 400, "invalid_filters", "做种人数筛选无效")
			return
		}
		f.MinSeeders = n
	}
	for _, group := range []*[]string{&f.Resolutions, &f.Sources, &f.Codecs, &f.AccountIDs} {
		if len(*group) == 1 && (*group)[0] == "" {
			*group = nil
		}
	}
	if f.Validate() != nil || page < 1 || page > 10000 || limit < 1 || limit > 200 {
		fail(w, 400, "invalid_filters", "筛选或分页参数无效")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	run, err := engine.GetFiltered(ctx, r.PathValue("id"), page, limit, f)
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
