package httpapi

import (
	"crypto/sha256"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/clientip"
	"net/http"
	"strconv"
)

func adminSessionMetadata(r *http.Request) accounts.SessionMetadata {
	return accounts.SessionMetadata{IP: clientip.Observed(r).ClientIP, UserAgent: r.UserAgent()}
}
func sessionDigest(r *http.Request) []byte {
	c, err := r.Cookie(auth.CookieName)
	if err != nil {
		return nil
	}
	sum := sha256.Sum256([]byte(c.Value))
	return sum[:]
}
func (a *API) listAdministratorSessions(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		fail(w, 400, "invalid_page", "分页范围无效")
		return
	}
	out, err := a.accountRepository().Sessions(r.Context(), sessionDigest(r), page)
	if err != nil {
		fail(w, 503, "sessions_unavailable", "管理端会话读取失败，请重试读取")
		return
	}
	write(w, 200, out)
}
func (a *API) revokeAdministratorSession(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		fail(w, 400, "confirmation_required", "请确认撤销指定管理端登录")
		return
	}
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_session", "会话ID无效")
		return
	}
	current, err := a.accountRepository().RevokeSession(r.Context(), r.PathValue("id"), sessionDigest(r))
	if err != nil {
		fail(w, 409, "session_revoke_failed", "撤销未确认，请读取会话核对")
		return
	}
	if current {
		a.Auth.ClearCookie(w)
	}
	write(w, 200, map[string]bool{"revoked": true, "loginRequired": current})
}
