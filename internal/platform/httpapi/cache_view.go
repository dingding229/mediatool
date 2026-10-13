package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/cacheview"
	"net/http"
	"strconv"
	"time"
)

func (a *API) cacheStore(w http.ResponseWriter) (cacheview.Store, bool) {
	if a.Deps.Redis == nil || a.Deps.DB == nil {
		fail(w, 503, "cache_unavailable", "缓存服务不可用")
		return cacheview.Store{}, false
	}
	return cacheview.Store{Redis: a.Deps.Redis, DB: a.Deps.DB}, true
}
func (a *API) viewCache(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	s, ok := a.cacheStore(w)
	if !ok {
		return
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		var err error
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 || page > 10000 {
			fail(w, 400, "invalid_page", "缓存分页无效")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	entries, total, truncated, e := s.List(ctx, r.PathValue("kind"), page, 30)
	if e != nil {
		fail(w, 400, "cache_query_failed", "缓存查询失败或类型无效")
		return
	}
	write(w, 200, map[string]any{"items": entries, "total": total, "page": page, "pageSize": 30, "truncated": truncated})
}
func (a *API) cacheDetail(w http.ResponseWriter, r *http.Request) {
	s, ok := a.cacheStore(w)
	if !ok {
		return
	}
	x, e := s.Get(r.Context(), r.PathValue("kind"), r.PathValue("id"), true)
	if e != nil {
		fail(w, 404, "cache_expired", "缓存已过期或不可用")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, x)
}
func (a *API) clearCache(w http.ResponseWriter, r *http.Request) {
	s, ok := a.cacheStore(w)
	if !ok {
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !body.Confirm {
		fail(w, 400, "confirm_required", "请明确确认清理此类缓存")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	n, truncated, e := s.Clear(ctx, r.PathValue("kind"))
	if e != nil {
		fail(w, 400, "cache_clear_failed", "清理失败或类型无效")
		return
	}
	write(w, 200, map[string]any{"deleted": n, "remainingMayExist": truncated})
}
func (a *API) deleteCache(w http.ResponseWriter, r *http.Request) {
	s, ok := a.cacheStore(w)
	if !ok {
		return
	}
	if r.URL.Query().Get("confirm") != "true" {
		fail(w, 400, "confirm_required", "请确认删除缓存项")
		return
	}
	e := s.Delete(r.Context(), r.PathValue("kind"), r.PathValue("id"))
	if e != nil {
		code := 503
		if errors.Is(e, cacheview.ErrInvalid) {
			code = 400
		}
		fail(w, code, "cache_delete_failed", "缓存项删除失败")
		return
	}
	write(w, 200, map[string]bool{"deleted": true})
}
