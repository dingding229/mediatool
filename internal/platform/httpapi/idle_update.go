package httpapi

import (
	"context"
	"errors"
	"github.com/felixge/httpsnoop"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/idleupdate"
	"media-tool/internal/platform/updateidle"
	"net/http"
	"strings"
	"time"
)

func (a *API) idleUpdater() *idleupdate.Service {
	if a.Deps.IdleUpdater != nil {
		return a.Deps.IdleUpdater
	}
	return idleupdate.New(a.Deps.DB, a.onlineUpdater(), a.updateChecker())
}
func (a *API) getIdleUpdate(w http.ResponseWriter, r *http.Request) {
	v, e := a.idleUpdater().Get(r.Context())
	if e != nil {
		fail(w, 503, "idle_update_unavailable", "闲时更新配置暂不可读取")
		return
	}
	write(w, 200, v)
}
func (a *API) checkIdleUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	v, e := a.idleUpdater().Check(ctx)
	if e != nil {
		fail(w, 503, "idle_update_unavailable", "执行条件暂不可读取，未启动更新")
		return
	}
	write(w, 200, v)
}
func (a *API) saveIdleUpdate(w http.ResponseWriter, r *http.Request) {
	var in idleupdate.SaveRequest
	if !decode(w, r, &in) {
		return
	}
	if e := in.Validate(); e != nil {
		fail(w, 400, "invalid_idle_update", e.Error())
		return
	}
	if in.Enabled {
		if in.Password == "" || !in.Confirm {
			fail(w, 400, "update_confirmation_required", "开启或修改闲时更新须输入当前密码并确认无人值守风险")
			return
		}
		_, ok := a.reauthenticateUpdate(w, r, in.Password)
		in.Password = ""
		if !ok {
			return
		}
	}
	account, e := a.sessionAccount(r)
	if e != nil {
		fail(w, 401, "unauthorized", "请重新登录")
		return
	}
	in.Password = ""
	v, e := a.idleUpdater().Save(r.Context(), in, account)
	if errors.Is(e, idleupdate.ErrConflict) {
		fail(w, 409, "idle_update_conflict", e.Error())
		return
	}
	if e != nil {
		fail(w, 503, "idle_update_unavailable", "配置未确认保存，请刷新后重试")
		return
	}
	write(w, 200, v)
}

// Observe successful issuance, not access logs or asynchronous statistics. Keep
// the gate admission through the durable timestamp write to close the check race.
func idleMedia(next http.Handler, db *pgxpool.Pool, scope func(*http.Request) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !scope(r) {
			next.ServeHTTP(w, r)
			return
		}
		release, ok := updateidle.Default.Enter()
		if !ok {
			w.Header().Set("Retry-After", "5")
			fail(w, 503, "idle_update_admission", "正在完成闲时更新最终复查，请稍后重试")
			return
		}
		defer release()
		m := httpsnoop.CaptureMetrics(next, w, r)
		if db != nil && (m.Code == 302 || m.Code >= 200 && m.Code < 300 && strings.HasSuffix(strings.ToLower(r.URL.Path), "/playbackinfo")) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
			defer cancel()
			if _, e := db.Exec(ctx, `UPDATE idle_update_activity SET last_issued_at=greatest(last_issued_at,clock_timestamp())WHERE singleton`); e != nil {
				// A failed activity write must not create a false idle window after DB recovery.
				idleupdate.ActivityWriteFailed()
			} else {
				idleupdate.ActivityWriteSucceeded()
			}
		}
	})
}
