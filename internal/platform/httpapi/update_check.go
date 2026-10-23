package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/updatecheck"
	"media-tool/internal/platform/buildinfo"
	"net/http"
	"time"
)

func (a *API) updateChecker() *updatecheck.Service {
	if a.Deps.UpdateChecker != nil {
		return a.Deps.UpdateChecker
	}
	return updatecheck.New(a.Deps.DB, a.Deps.Logger)
}
func (a *API) versionInfo(w http.ResponseWriter, r *http.Request) { write(w, 200, buildinfo.Current()) }
func (a *API) updateState(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	v, e := a.updateChecker().Get(ctx)
	if e != nil {
		fail(w, 503, "updates_unavailable", "更新检查状态暂不可用")
		return
	}
	write(w, 200, v)
}
func (a *API) checkUpdates(w http.ResponseWriter, r *http.Request) {
	v, e := a.updateChecker().Check(r.Context())
	if errors.Is(e, updatecheck.ErrBusy) {
		fail(w, 409, "update_check_busy", "更新检查正在进行，请稍后刷新")
		return
	}
	if e != nil {
		fail(w, 503, "updates_unavailable", "更新检查暂不可用")
		return
	}
	write(w, 200, v)
}
func (a *API) saveUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Enabled == nil {
		fail(w, 400, "invalid_update_settings", "请选择是否自动检查")
		return
	}
	v, e := a.updateChecker().SetEnabled(r.Context(), *in.Enabled)
	if e != nil {
		fail(w, 503, "updates_unavailable", "更新检查设置未保存，请重新读取")
		return
	}
	write(w, 200, v)
}
func (a *API) updateNotice(w http.ResponseWriter, r *http.Request) {
	v, e := a.updateChecker().TakeNotice(r.Context())
	if e != nil {
		fail(w, 503, "updates_unavailable", "更新提示暂不可用")
		return
	}
	write(w, 200, struct {
		Release *updatecheck.Release `json:"release"`
	}{v})
}
