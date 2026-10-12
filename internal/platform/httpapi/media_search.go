package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/releasename"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/trace"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *API) tmdbMediaSearch(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	q, kind, year := strings.TrimSpace(r.URL.Query().Get("q")), r.URL.Query().Get("mediaType"), r.URL.Query().Get("year")
	if kind == "" {
		kind = "auto"
	}
	if len(q) == 0 || len(q) > 200 || kind != "movie" && kind != "series" && kind != "auto" {
		fail(w, 400, "invalid_media_search", "输入媒体名称；类型支持自动、电影或剧集")
		return
	}
	if year != "" {
		n, err := strconv.Atoi(year)
		if err != nil || n < 1800 || n > 2199 {
			fail(w, 400, "invalid_year", "年份无效")
			return
		}
	}
	hint := releasename.Parse(q)
	originalQuery := q
	if kind == "auto" && (hint.Year != "" || hint.Season > 0 || hint.Resolution != "") && hint.Title != "" {
		q = hint.Title
		if year == "" {
			year = hint.Year
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	prefs, err := (settings.Repository{DB: a.Deps.DB}).Get(ctx)
	if err != nil {
		fail(w, 503, "settings_unavailable", "缓存设置不可用")
		return
	}
	profile, err := a.Deps.Integrations.Repository.Default(ctx, "tmdb")
	if err != nil {
		fail(w, 503, "tmdb_not_configured", "请在系统设置添加并启用TMDB")
		return
	}
	raw, _ := json.Marshal([]any{"auto-search-v2", q, kind, year, profile.Record.ID, profile.Record.Revision, profile.String("language"), prefs.Revision})
	sum := sha256.Sum256(raw)
	key := "media-tool:tmdb-cache:" + hex.EncodeToString(sum[:])
	items := []metadata.Item{}
	cached := false
	ttl := time.Duration(prefs.Value.TMDBCacheSeconds) * time.Second
	if ttl > 0 && a.Deps.Redis != nil && r.URL.Query().Get("refresh") != "true" {
		bounded, stop := context.WithTimeout(ctx, 250*time.Millisecond)
		raw, err := a.Deps.Redis.Get(bounded, key).Bytes()
		stop()
		cached = err == nil && len(raw) <= 1<<20 && json.Unmarshal(raw, &items) == nil
	}
	if !cached {
		client, err := a.Deps.Integrations.TMDB(ctx)
		if err == nil {
			items, err = client.Search(ctx, kind, q)
		}
		if err != nil {
			fail(w, 502, "tmdb_search_failed", "TMDB查询失败，请检查配置或网络；未缓存失败结果")
			return
		}
		if year != "" {
			filtered := []metadata.Item{}
			for _, item := range items {
				if item.Year == year {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		if ttl > 0 && a.Deps.Redis != nil {
			raw, _ := json.Marshal(items)
			if len(raw) <= 1<<20 {
				bounded, stop := context.WithTimeout(ctx, 250*time.Millisecond)
				a.Deps.Redis.Set(bounded, key, raw, ttl)
				stop()
			}
		}
	}
	trace.Info(r.Context(), "search", "TMDB媒体搜索完成", "keyword", originalQuery, "normalizedKeyword", q, "mediaType", kind, "year", year, "cached", cached, "count", len(items))
	if r.URL.Path == "/api/v1/metadata/search" {
		if cached {
			w.Header().Set("X-Media-Cache", "hit")
		} else {
			w.Header().Set("X-Media-Cache", "miss")
		}
		write(w, 200, items)
		return
	}
	write(w, 200, map[string]any{"items": items, "cached": cached, "cacheSeconds": prefs.Value.TMDBCacheSeconds, "mediaType": kind, "query": q, "originalQuery": originalQuery, "recognitionHint": hint})
}
