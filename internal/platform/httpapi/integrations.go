package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/metadata"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"media-tool/internal/modules/integrations"
)

func (a *API) integrationReady(w http.ResponseWriter) bool {
	if a.Deps.Integrations == nil {
		fail(w, 503, "integrations_unavailable", "请连接 PostgreSQL、执行迁移并配置 APP_SECRET_KEY")
		return false
	}
	return true
}
func (a *API) listIntegrations(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	list, err := a.Deps.Integrations.Repository.List(ctx)
	if err != nil {
		integrationError(w, err)
		return
	}
	write(w, 200, list)
}
func (a *API) saveIntegration(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var in integrations.Input
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	id := r.PathValue("id")
	if id == "" {
		in.Revision = 0
	}
	record, err := a.Deps.Integrations.Repository.Save(ctx, id, in)
	if err != nil {
		integrationError(w, err)
		return
	}
	status := 200
	if id == "" {
		status = 201
	}
	write(w, status, record)
}
func (a *API) deleteIntegration(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := a.Deps.Integrations.Repository.Delete(ctx, r.PathValue("id")); err != nil {
		integrationError(w, err)
		return
	}
	write(w, 200, map[string]bool{"deleted": true})
}
func (a *API) testIntegration(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	result, err := a.Deps.Integrations.Test(r.Context(), r.PathValue("id"))
	if err != nil {
		integrationError(w, err)
		return
	}
	write(w, 200, result)
}
func integrationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, integrations.ErrNotFound):
		fail(w, 404, "integration_not_found", "连接不存在")
	case errors.Is(err, integrations.ErrConflict):
		fail(w, 409, "integration_conflict", "同类型只能启用一个连接；请先停用已有连接")
	case errors.Is(err, integrations.ErrRevision):
		fail(w, 409, "integration_changed", "连接已被修改，请刷新后重试")
	case errors.Is(err, integrations.ErrUnavailable):
		fail(w, 503, "integrations_unavailable", "连接数据暂不可用，请检查数据库与迁移")
	default:
		fail(w, 400, "invalid_integration", err.Error())
	}
}
func (a *API) metadataSearch(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	client, err := a.Deps.Integrations.TMDB(r.Context())
	if err != nil {
		fail(w, 503, "tmdb_not_configured", "请在连接管理中配置并启用 TMDB")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	items, err := client.Search(ctx, r.URL.Query().Get("mediaType"), r.URL.Query().Get("q"))
	if err != nil {
		fail(w, 502, "metadata_search_failed", "媒体查询失败，请检查 TMDB Token、类型和网络")
		return
	}
	write(w, 200, items)
}

func (a *API) metadataEpisodes(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 4096 || len(q["tmdbId"]) != 1 || len(q["season"]) != 1 || len(q["episodes"]) != 1 {
		fail(w, 400, "invalid_episode_query", "请提供明确的TMDB剧集ID、季号和实际集号")
		return
	}
	for key := range q {
		if key != "tmdbId" && key != "season" && key != "episodes" {
			fail(w, 400, "invalid_episode_query", "不支持的单集查询参数")
			return
		}
	}
	id, err := strconv.Atoi(q.Get("tmdbId"))
	if err != nil {
		fail(w, 400, "invalid_episode_query", "剧集ID无效")
		return
	}
	season, err := strconv.Atoi(q.Get("season"))
	if err != nil {
		fail(w, 400, "invalid_episode_query", "季号无效")
		return
	}
	parts := strings.Split(q.Get("episodes"), ",")
	numbers := []int{}
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			fail(w, 400, "invalid_episode_query", "实际集号须为明确的整数列表")
			return
		}
		numbers = append(numbers, n)
	}
	numbers, err = metadata.NormalizeEpisodes(season, numbers)
	if err != nil || id < 1 {
		fail(w, 400, "invalid_episode_query", "季集覆盖无效或重复")
		return
	}
	client, err := a.Deps.Integrations.TMDB(r.Context())
	if err != nil {
		fail(w, 503, "tmdb_not_configured", "请配置并启用TMDB")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	episodes, err := client.EpisodeDetails(ctx, id, season, numbers)
	if err != nil {
		fail(w, 502, "episode_metadata_failed", "单集元数据未验证；请核对身份、季集和TMDB网络，未写入输出")
		return
	}
	write(w, 200, episodes)
}

func (a *API) revealIntegrationSecrets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if !a.integrationReady(w) {
		return
	}
	var in struct {
		Revision int64    `json:"revision"`
		Fields   []string `json:"fields"`
		Confirm  bool     `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		fail(w, 400, "reveal_confirmation_required", "请明确确认读取已保存凭据；公网管理请使用HTTPS")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	values, err := a.Deps.Integrations.Repository.Reveal(ctx, r.PathValue("id"), in.Revision, in.Fields)
	if err != nil {
		integrationError(w, err)
		return
	}
	write(w, 200, map[string]any{"revision": in.Revision, "values": values})
}
