package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/nativetmdb"
	"net/http"
)

type NativeTMDBRefresher interface {
	Refresh(context.Context, nativetmdb.Target) (string, error)
}
type nativeTMDBQueued struct {
	DetailID string `json:"detailId"`
	Queued   bool   `json:"queued"`
}

func (a *API) refreshNativeTMDB(w http.ResponseWriter, r *http.Request) {
	var in nativetmdb.Target
	if !decode(w, r, &in) {
		return
	}
	if !in.Valid() {
		fail(w, 400, "invalid_tmdb_target", "请选择有效的电影、剧集或季身份")
		return
	}
	service := a.Deps.NativeTMDBRefresh
	if service == nil {
		if !a.integrationReady(w) {
			return
		}
		service = nativetmdb.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}
	}
	id, err := service.Refresh(r.Context(), in)
	if errors.Is(err, nativetmdb.ErrInvalid) {
		fail(w, 400, "tmdb_target_not_exposable", "仅能刷新已确认且存在有效STRM的媒体")
		return
	}
	if err != nil {
		fail(w, 503, "tmdb_refresh_unavailable", "TMDB详情刷新暂不可用，请检查连接设置")
		return
	}
	write(w, 202, nativeTMDBQueued{DetailID: id, Queued: true})
}
