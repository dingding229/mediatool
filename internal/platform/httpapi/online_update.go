package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/onlineupdate"
	"media-tool/internal/modules/updatecheck"
	"net/http"
	"time"
)

func (a *API) onlineUpdater() *onlineupdate.Service {
	if a.Deps.OnlineUpdater != nil {
		return a.Deps.OnlineUpdater
	}
	return onlineupdate.New(a.Deps.DB, a.updateChecker())
}
func (a *API) onlineUpdateStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	v, e := a.onlineUpdater().Status(ctx)
	if e != nil {
		fail(w, 503, "online_update_unavailable", "更新状态无法读取，请检查私有更新记录")
		return
	}
	write(w, 200, v)
}
func (a *API) executeOnlineUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
		Version  string `json:"version"`
		Digest   string `json:"digest"`
		Confirm  bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || in.Password == "" {
		fail(w, 400, "update_confirmation_required", "请输入当前管理员密码，并确认中断播放和数据库恢复风险")
		return
	}
	actor, ok := a.reauthenticateUpdate(w, r, in.Password)
	in.Password = ""
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	j, e := a.onlineUpdater().Start(ctx, actor, in.Version, in.Digest)
	switch {
	case errors.Is(e, onlineupdate.ErrDisabled):
		fail(w, 403, "online_update_disabled", e.Error())
	case errors.Is(e, onlineupdate.ErrBusy):
		fail(w, 409, "online_update_busy", e.Error())
	case errors.Is(e, updatecheck.ErrUntrusted):
		fail(w, 409, "untrusted_update", "Release 与 Hub 再次校验未通过，已拒绝并记录审计")
	case e != nil:
		fail(w, 503, "online_update_rejected", "更新未确认启动，请读取更新状态或检查部署权限")
	default:
		write(w, 202, j)
	}
}

func (a *API) reauthenticateUpdate(w http.ResponseWriter, r *http.Request, password string) (string, bool) {
	account, e := a.sessionAccount(r)
	if e != nil {
		fail(w, 401, "unauthorized", "请重新登录")
		return "", false
	}
	permit := passwordPermit(w)
	if permit == nil {
		return "", false
	}
	defer permit()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	allowed, e := a.accountRepository().TakeAttempt(ctx, a.Config.SessionKey, "online-update:"+account.ID, 5)
	if e != nil || !allowed {
		fail(w, 429, "rate_limited", "更新密码校验过于频繁，请稍后重试")
		return "", false
	}
	checked, e := a.accountRepository().Authenticate(ctx, account.Username, password)
	password = ""
	if e != nil || checked.ID != account.ID || checked.Version != account.Version {
		if errors.Is(e, accounts.ErrUnavailable) {
			fail(w, 503, "auth_unavailable", "密码服务暂不可用")
		} else {
			fail(w, 403, "invalid_current_password", "当前密码不正确或账号已变化")
		}
		return "", false
	}
	return account.Username, true
}
func (a *API) controlOnlineUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action   string `json:"action"`
		JobID    string `json:"jobId"`
		Password string `json:"password"`
		Confirm  bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || in.Password == "" {
		fail(w, 400, "update_confirmation_required", "请输入管理员密码并明确确认此操作")
		return
	}
	actor, ok := a.reauthenticateUpdate(w, r, in.Password)
	in.Password = ""
	if !ok {
		return
	}
	e := a.onlineUpdater().SetControl(r.Context(), actor, in.Action, in.JobID)
	if errors.Is(e, onlineupdate.ErrBusy) {
		fail(w, 409, "online_update_busy", "请先完成人工恢复确认或等待当前操作结束")
		return
	}
	if e != nil {
		fail(w, 409, "online_update_control_rejected", "执行设置未变更，请读取当前状态")
		return
	}
	write(w, 200, map[string]bool{"ok": true})
}
