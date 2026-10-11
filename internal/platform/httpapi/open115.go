package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"media-tool/internal/modules/integrations"
)

func (a *API) start115Authorization(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	owner, err := a.sessionAccount(r)
	if err != nil {
		fail(w, 401, "unauthorized", "请重新登录")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	result, err := a.Deps.Integrations.StartOfficialAuthorization(ctx, r.PathValue("id"), owner.ID, owner.Version)
	if err != nil {
		officialAuthError(w, err)
		return
	}
	write(w, 200, result)
}
func (a *API) poll115Authorization(w http.ResponseWriter, r *http.Request)   { a.do115Poll(w, r, false) }
func (a *API) cancel115Authorization(w http.ResponseWriter, r *http.Request) { a.do115Poll(w, r, true) }
func (a *API) do115Poll(w http.ResponseWriter, r *http.Request, stop bool) {
	if !a.integrationReady(w) {
		return
	}
	var body struct {
		SessionID string `json:"sessionId"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(body.SessionID) != 36 {
		fail(w, 400, "invalid_auth_session", "授权会话无效")
		return
	}
	owner, err := a.sessionAccount(r)
	if err != nil {
		fail(w, 401, "unauthorized", "请重新登录")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	result, err := a.Deps.Integrations.PollOfficialAuthorization(ctx, r.PathValue("id"), body.SessionID, owner.ID, owner.Version, stop)
	if err != nil {
		officialAuthError(w, err)
		return
	}
	if result.State == "authorized" && !stop {
		check, err := a.Deps.Integrations.Test(ctx, r.PathValue("id"))
		if err != nil || !check.Passed {
			fail(w, 502, "authorization_validation_failed", "授权已保存但自动验证未通过，请检查云盘配置及网络")
			return
		}
	}
	write(w, 200, result)
}
func officialAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, integrations.ErrNotFound):
		fail(w, 404, "integration_not_found", "请先保存115官方直链连接")
	case errors.Is(err, integrations.ErrRevision), errors.Is(err, integrations.ErrAuthSession):
		fail(w, 409, "authorization_changed", "二维码已过期、已取消或连接已修改，请重新开始授权")
	default:
		fail(w, 502, "official_115_authorization_failed", "115官方授权请求失败；请核对自己的AppID、应用审批与网络状态，勿使用他人的应用凭据")
	}
}
