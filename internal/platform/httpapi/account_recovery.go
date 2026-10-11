package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	"net/http"
	"time"
)

func (a *API) issueRecoveryCode(w http.ResponseWriter, r *http.Request) {
	account, err := a.sessionAccount(r)
	if err != nil {
		fail(w, 401, "unauthorized", "请重新登录")
		return
	}
	var in struct {
		Password string `json:"currentPassword"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Password) > 512 {
		fail(w, 400, "invalid_credentials", "当前密码不正确")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	allowed, err := a.accountRepository().TakeAttempt(ctx, a.Config.SessionKey, "recovery-issue:"+account.ID, 5)
	if err != nil {
		fail(w, 503, "auth_unavailable", "账号服务暂不可用")
		return
	}
	if !allowed {
		fail(w, 429, "rate_limited", "操作过于频繁，请稍后重试")
		return
	}
	release := passwordPermit(w)
	if release == nil {
		return
	}
	defer release()
	code, expires, err := a.accountRepository().IssueRecoveryCode(ctx, account, in.Password)
	if errors.Is(err, accounts.ErrCredentials) {
		fail(w, 400, "invalid_credentials", "当前密码不正确或会话已改变")
		return
	}
	if err != nil {
		fail(w, 503, "recovery_unavailable", "恢复码生成未确认成功，请稍后重试")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, map[string]any{"recoveryCode": code, "expiresAt": expires, "shownOnce": true})
}
func (a *API) recoverPassword(w http.ResponseWriter, r *http.Request) {
	if !a.checkAuthRequest(w, r) {
		return
	}
	var in struct {
		Username string `json:"username"`
		Code     string `json:"recoveryCode"`
		Next     string `json:"newPassword"`
		Confirm  string `json:"confirmPassword"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Next != in.Confirm {
		fail(w, 400, "password_mismatch", "两次输入的新密码不一致")
		return
	}
	if err := auth.ValidatePassword(in.Next); err != nil {
		fail(w, 400, "invalid_password", err.Error())
		return
	}
	release := passwordPermit(w)
	if release == nil {
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	err := a.accountRepository().RecoverPassword(ctx, in.Username, in.Code, in.Next)
	if errors.Is(err, accounts.ErrCredentials) {
		fail(w, 400, "invalid_recovery", "账号或恢复码无效、已使用或已过期")
		return
	}
	if err != nil {
		fail(w, 503, "recovery_unavailable", "密码恢复未确认成功，请稍后重试")
		return
	}
	a.Auth.ClearCookie(w)
	write(w, 200, map[string]bool{"changed": true, "loginRequired": true})
}
