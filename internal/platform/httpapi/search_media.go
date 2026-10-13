package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func (a *API) saveSearchMedia(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var in struct {
		ID        int    `json:"id"`
		MediaType string `json:"mediaType"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.ID < 1 || (in.MediaType != "movie" && in.MediaType != "series") {
		fail(w, 400, "invalid_media", "请选择明确媒体身份")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	client, err := a.Deps.Integrations.TMDB(ctx)
	if err != nil {
		fail(w, 503, "tmdb_unavailable", "TMDB不可用")
		return
	}
	item, err := client.Details(ctx, in.MediaType, in.ID)
	if err != nil || item.ID != in.ID || item.MediaType != in.MediaType {
		fail(w, 502, "tmdb_failed", "媒体信息查询失败")
		return
	}
	raw, _ := json.Marshal(item)
	tag, err := a.Deps.DB.Exec(ctx, `UPDATE search_runs SET media_context=$2 WHERE id=$1`, r.PathValue("id"), raw)
	if err != nil || tag.RowsAffected() != 1 {
		fail(w, 404, "search_unavailable", "搜索记录不存在")
		return
	}
	write(w, 200, item)
}
