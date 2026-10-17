package httpapi

import (
	"context"
	"io"
	"media-tool/internal/modules/catalog"
	"media-tool/internal/modules/mediaserver"
	"media-tool/internal/modules/playback"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"regexp"
	"strings"
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
	var items catalog.Page
	if source == "emby" {
		items, err = (mediaserver.EmbyAdapter{Catalog: e}).AdministrativeList(ctx, r.URL.Query().Get("q"), r.URL.Query().Get("mediaType"), r.URL.Query().Get("parentId"), p.Page, p.Limit)
	} else {
		items, err = e.ListParent(ctx, source, r.URL.Query().Get("q"), r.URL.Query().Get("mediaType"), r.URL.Query().Get("parentId"), p.Page, p.Limit)
	}
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
	image, mime, err := e.ImageType(ctx, r.PathValue("id"), r.URL.Query().Get("type"))
	if err != nil {
		fail(w, 404, "image_unavailable", "海报不可用")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(200)
	if _, checkedErr := w.Write(image); checkedErr != nil {
		deferutil.Report(checkedErr, "internal/platform/httpapi/catalog.go:catalogImage")
		return
	}
}

func (a *API) catalogHome(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	items, err := (catalog.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}).Home(ctx, r.URL.Query().Get("userId"))
	if err != nil {
		fail(w, 503, "emby_home_unavailable", "Emby媒体库首页不可用，请检查已配置API Key、用户及媒体库")
		return
	}
	write(w, 200, items)
}
func (a *API) tmdbPoster(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if !regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(jpg|png|webp)$`).MatchString(p) {
		fail(w, 400, "invalid_poster", "海报路径无效")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://image.tmdb.org/t/p/w342"+p, nil)
	if err != nil {
		fail(w, 404, "poster_unavailable", "海报不可用")
		return
	}
	policy := playback.Policy{AllowedHosts: []string{"image.tmdb.org"}}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: policy.DialContext}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		fail(w, 404, "poster_unavailable", "海报不可用")
		return
	}
	defer deferutil.Close(res.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
	content := strings.Split(res.Header.Get("Content-Type"), ";")[0]
	if res.StatusCode != 200 || content != "image/jpeg" && content != "image/png" && content != "image/webp" {
		fail(w, 404, "poster_unavailable", "海报不可用")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		fail(w, 404, "poster_unavailable", "海报不可用")
		return
	}
	w.Header().Set("Content-Type", content)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, checkedErr := w.Write(raw); checkedErr != nil {
		deferutil.Report(checkedErr, "internal/platform/httpapi/catalog.go:tmdbPoster")
		return
	}
}
func (a *API) catalogItem(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	item, err := (catalog.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}).Item(ctx, r.PathValue("id"))
	if err != nil {
		fail(w, 404, "media_unavailable", "媒体详情不可用")
		return
	}
	write(w, 200, item)
}
