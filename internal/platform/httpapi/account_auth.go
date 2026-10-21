package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	"net"
	"net/http"
	"time"
)

// Bound password work across all HTTP handlers; each Argon2 operation uses
// 19MiB. Network throttles use PG and do not trust forwarded client addresses.
var passwordSlots = make(chan struct{}, 2)

func (a *API) accountRepository() accounts.Repository { return accounts.Repository{DB: a.Deps.DB} }
func (a *API) authSetup(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	registered, err := a.accountRepository().Registered(ctx)
	if err != nil {
		fail(w, 503, "auth_unavailable", "请连接PostgreSQL并执行迁移后注册管理员")
		return
	}
	write(w, 200, map[string]any{"registrationOpen": !registered, "passwordMinLength": auth.PasswordMinLength, "passwordMaxLength": auth.PasswordMaxLength})
}
func (a *API) checkAuthRequest(w http.ResponseWriter, r *http.Request) bool {
	if !a.sameOrigin(r) {
		fail(w, 403, "csrf_rejected", "请求来源或防跨站请求标识无效")
		return false
	}
	if a.Deps.DB == nil {
		fail(w, 503, "auth_unavailable", "账号登录需要PostgreSQL和已执行的数据库迁移")
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	globalAllowed, globalErr := a.accountRepository().TakeAttempt(ctx, a.Config.SessionKey, "global", 120)
	if globalErr != nil {
		fail(w, 503, "auth_unavailable", "登录服务暂不可用")
		return false
	}
	if !globalAllowed {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "rate_limited", "登录请求过于频繁，请稍后重试")
		return false
	}
	allowed, err := a.accountRepository().TakeAttempt(ctx, a.Config.SessionKey, "ip:"+host, 20)
	if err != nil {
		fail(w, 503, "auth_unavailable", "登录服务暂不可用")
		return false
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "rate_limited", "登录或注册尝试过于频繁，请一分钟后重试")
		return false
	}
	return true
}
func passwordPermit(w http.ResponseWriter) func() {
	select {
	case passwordSlots <- struct{}{}:
		return func() { <-passwordSlots }
	default:
		w.Header().Set("Retry-After", "1")
		fail(w, 429, "auth_busy", "登录服务繁忙，请稍后重试")
		return nil
	}
}
func (a *API) register(w http.ResponseWriter, r *http.Request) {
	if !a.checkAuthRequest(w, r) {
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Confirm  string `json:"confirmPassword"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	registered, err := a.accountRepository().Registered(ctx)
	if err != nil {
		fail(w, 503, "auth_unavailable", "注册服务暂不可用")
		return
	}
	if registered {
		fail(w, 409, "registration_closed", "管理员已注册，公开注册已关闭，请使用账号密码登录")
		return
	}
	if _, err = accounts.NormalizeUsername(in.Username); err != nil {
		fail(w, 400, "invalid_username", err.Error())
		return
	}
	if err = auth.ValidatePassword(in.Password); err != nil {
		fail(w, 400, "invalid_password", err.Error())
		return
	}
	if in.Password != in.Confirm {
		fail(w, 400, "password_mismatch", "两次输入的密码不一致")
		return
	}
	release := passwordPermit(w)
	if release == nil {
		return
	}
	defer release()
	token, err := a.Auth.Issue(time.Now())
	if err != nil {
		fail(w, 500, "session_failed", "无法建立管理会话")
		return
	}
	account, err := a.accountRepository().Register(ctx, in.Username, in.Password, token, adminSessionMetadata(r))
	if errors.Is(err, accounts.ErrClosed) {
		fail(w, 409, "registration_closed", "管理员已注册，公开注册已关闭")
		return
	}
	if err != nil {
		fail(w, 503, "registration_failed", "注册未确认成功，请检查登录服务后重试")
		return
	}
	a.Auth.SetCookie(w, token)
	write(w, 201, map[string]any{"authenticated": true, "account": account, "registrationOpen": false})
}
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	if !a.checkAuthRequest(w, r) {
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Username) > 128 || len(in.Password) > 512 {
		fail(w, 401, "invalid_credentials", "账号或密码不正确")
		return
	}
	release := passwordPermit(w)
	if release == nil {
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	account, err := a.accountRepository().Authenticate(ctx, in.Username, in.Password)
	if errors.Is(err, accounts.ErrCredentials) {
		fail(w, 401, "invalid_credentials", "账号或密码不正确")
		return
	}
	if err != nil {
		fail(w, 503, "auth_unavailable", "登录服务暂不可用")
		return
	}
	token, err := a.Auth.Issue(time.Now())
	if err != nil {
		fail(w, 500, "session_failed", "无法建立管理会话")
		return
	}
	if err = a.accountRepository().CreateSession(ctx, account, token, adminSessionMetadata(r)); err != nil {
		fail(w, 503, "session_unavailable", "账号状态已改变或会话数据库不可用，请重新登录")
		return
	}
	a.Auth.SetCookie(w, token)
	write(w, 200, map[string]any{"authenticated": true, "account": account})
}
func (a *API) accountSession(w http.ResponseWriter, r *http.Request) {
	account, err := a.sessionAccount(r)
	if err != nil {
		fail(w, 401, "unauthorized", "请使用管理员账号密码登录")
		return
	}
	write(w, 200, map[string]any{"authenticated": true, "account": account})
}
func (a *API) changeAccountPassword(w http.ResponseWriter, r *http.Request) {
	account, err := a.sessionAccount(r)
	if err != nil {
		fail(w, 401, "unauthorized", "请重新登录")
		return
	}
	var in struct {
		Current string `json:"currentPassword"`
		Next    string `json:"newPassword"`
		Confirm string `json:"confirmPassword"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Next != in.Confirm {
		fail(w, 400, "password_mismatch", "两次输入的新密码不一致")
		return
	}
	if err = auth.ValidatePassword(in.Next); err != nil {
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
	allowed, err := a.accountRepository().TakeAttempt(ctx, a.Config.SessionKey, "password-change:"+account.ID, 10)
	if err != nil {
		fail(w, 503, "auth_unavailable", "密码服务暂不可用")
		return
	}
	if !allowed {
		fail(w, 429, "rate_limited", "密码修改尝试过于频繁，请稍后重试")
		return
	}
	if err = a.accountRepository().ChangePassword(ctx, account, in.Current, in.Next); errors.Is(err, accounts.ErrCredentials) {
		fail(w, 400, "invalid_current_password", "当前密码不正确或账号已改变")
		return
	} else if err != nil {
		fail(w, 400, "password_change_failed", "密码修改未确认成功，请核对当前密码和新密码")
		return
	}
	a.Auth.ClearCookie(w)
	write(w, 200, map[string]bool{"changed": true, "loginRequired": true})
}
