package httpapi

import (
	"context"
	"media-tool/internal/modules/catalog"
	"net/http"
	"time"
)

func (a *API) listCatalog(w http.ResponseWriter, r *http.Request) {
	if a.Deps.DB == nil {
		fail(w, 503, "catalog_unavailable", "请先连接数据库")
		return
	}
	p, err := pagination(r)
	if err != nil {
		fail(w, 400, "invalid_pagination", "分页参数无效")
		return
	}
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "managed"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	e := catalog.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}
	items, err := e.List(ctx, source, r.URL.Query().Get("q"), r.URL.Query().Get("mediaType"), p.Page, p.Limit)
	if err != nil {
		fail(w, 503, "catalog_unavailable", "媒体库读取失败；Emby内容需配置并启用Emby连接和媒体库API Key")
		return
	}
	write(w, 200, items)
}
func (a *API) catalogImage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	e := catalog.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}
	image, mime, err := e.Image(ctx, r.PathValue("id"))
	if err != nil {
		fail(w, 404, "image_unavailable", "海报不可用")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(200)
	w.Write(image)
}
