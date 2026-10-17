package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/nativeplayback"
	playbackruntime "media-tool/internal/modules/nativeplayback/runtime"
	"net/http"
	"strconv"
)

type NativeSessionReader interface {
	List(context.Context, int, int) (nativeplayback.ActiveSessions, error)
}

func (a *API) nativeSessions(w http.ResponseWriter, r *http.Request) {
	start, limit := 0, 100
	for _, v := range []struct {
		key    string
		target *int
	}{{"startIndex", &start}, {"limit", &limit}} {
		if values, ok := r.URL.Query()[v.key]; ok {
			if len(values) != 1 {
				fail(w, 400, "invalid_native_sessions_query", "页码或条数无效")
				return
			}
			n, e := strconv.Atoi(values[0])
			if e != nil {
				fail(w, 400, "invalid_native_sessions_query", "页码或条数无效")
				return
			}
			*v.target = n
		}
	}
	if start < 0 || start > 1000000 || limit < 1 || limit > 200 {
		fail(w, 400, "invalid_native_sessions_query", "页码或条数无效")
		return
	}
	reader := a.Deps.NativeSessions
	if reader == nil {
		reader = playbackruntime.SessionRepository{DB: a.Deps.DB}
	}
	out, err := reader.List(r.Context(), start, limit)
	if errors.Is(err, nativeplayback.ErrInvalid) {
		fail(w, 400, "invalid_native_sessions_query", "页码或条数无效")
		return
	}
	if err != nil {
		fail(w, 503, "native_sessions_unavailable", "播放会话列表暂不可用")
		return
	}
	write(w, 200, out)
}
