package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/networkproxy"
	"media-tool/internal/modules/settings"
	"net/http"
	"strconv"
	"time"
)

func (a *API) mediaDetails(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	kind := r.URL.Query().Get("mediaType")
	id, e := strconv.Atoi(r.URL.Query().Get("tmdbId"))
	if e != nil || id < 1 || kind != "movie" && kind != "series" {
		fail(w, 400, "invalid_media", "请选择明确电影/剧集身份")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	prefs, e := (settings.Repository{DB: a.Deps.DB}).Get(ctx)
	if e != nil {
		fail(w, 503, "settings_unavailable", "系统设置不可用")
		return
	}
	profile, e := a.Deps.Integrations.Repository.Default(ctx, "tmdb")
	if e != nil {
		fail(w, 503, "tmdb_not_configured", "请配置TMDB")
		return
	}
	proxyRevision, err := (networkproxy.Repository{DB: a.Deps.DB}).Revision(ctx)
	if err != nil {
		fail(w, 503, "proxy_unavailable", "代理设置不可用")
		return
	}
	seed, _ := json.Marshal([]any{"detail-v1", id, kind, profile.Record.ID, profile.Record.Revision, profile.String("language"), prefs.Revision, proxyRevision})
	sum := sha256.Sum256(seed)
	key := "media-tool:tmdb-cache:" + hex.EncodeToString(sum[:])
	ttl := time.Duration(prefs.Value.TMDBCacheSeconds) * time.Second
	var envelope struct {
		Item metadata.Item `json:"item"`
	}
	cached := false
	if ttl > 0 && a.Deps.Redis != nil {
		small, stop := context.WithTimeout(ctx, 250*time.Millisecond)
		raw, err := a.Deps.Redis.Get(small, key).Bytes()
		stop()
		cached = err == nil && len(raw) <= 1<<20 && json.Unmarshal(raw, &envelope) == nil && envelope.Item.ID == id && envelope.Item.MediaType == kind
	}
	if !cached {
		client, err := a.Deps.Integrations.TMDB(ctx)
		if err == nil {
			envelope.Item, err = client.Details(ctx, kind, id)
		}
		if err != nil || envelope.Item.ID != id || envelope.Item.MediaType != kind {
			fail(w, 502, "tmdb_details_failed", "媒体详情查询失败，未缓存失败结果")
			return
		}
		if ttl > 0 && a.Deps.Redis != nil {
			raw, _ := json.Marshal(map[string]any{"item": envelope.Item, "query": envelope.Item.Title, "mediaType": kind, "createdAt": time.Now().UTC()})
			if len(raw) <= 1<<20 {
				small, stop := context.WithTimeout(ctx, 250*time.Millisecond)
				a.Deps.Redis.Set(small, key, raw, ttl)
				stop()
			}
		}
	}
	write(w, 200, map[string]any{"item": envelope.Item, "cached": cached})
}
