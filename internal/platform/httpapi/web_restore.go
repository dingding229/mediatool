package httpapi

import (
	"context"
	"crypto/sha256"
	"net/http"
	"time"

	"media-tool/internal/modules/maintenance"
	"media-tool/internal/platform/auth"
)

func (a *API) registerWebRestore(mux routeRegistrar) {
	mux.HandleFunc("POST /api/v1/ops/restores/preview", a.protected(a.webRestorePreview))
	mux.HandleFunc("POST /api/v1/ops/restores/{id}/execute", a.protected(a.webRestoreExecute))
	mux.HandleFunc("POST /api/v1/ops/restores/{id}/reconcile", a.protected(a.webRestoreReconcile))
	mux.HandleFunc("GET /api/v1/ops/restores", a.protected(a.webRestoreList))
}
func (a *API) webRestoreList(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) {
		return
	}
	out, err := a.maintenance().WebRestores(r.Context())
	if err != nil {
		opsError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) webRestorePreview(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) {
		return
	}
	var in struct {
		BackupID string `json:"backupId"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	out, err := a.maintenance().PreviewWebRestore(ctx, in.BackupID)
	if err != nil {
		opsError(w, err)
		return
	}
	write(w, 200, map[string]any{"restore": out, "confirmText": "恢复 " + out.BackupID, "includes": maintenance.RestoreIncludes, "excludes": maintenance.RestoreExcludes, "message": "已验证完整性、密文可读及迁移；确认后先创建恢复前快照。旧 schema 自动迁移，不执行降级。"})
}

// Existing same-origin + X-Requested-With CSRF check is required by protected.
// Require a PG-valid session created within 10 minutes, not last_active_at.
func (a *API) recentRestoreSession(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		return false
	}
	digest := sha256.Sum256([]byte(cookie.Value))
	var recent bool
	err = a.Deps.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM administrator_sessions WHERE token_digest=$1 AND revoked_at IS NULL AND expires_at>now() AND created_at>now()-interval '10 minutes')`, digest[:]).Scan(&recent)
	if err != nil {
		fail(w, 503, "auth_unavailable", "无法核对新近登录，请稍后重试")
		return false
	}
	if !recent {
		fail(w, 403, "recent_login_required", "恢复需要10分钟内的新近登录，请重新登录后重新预览并确认")
		return false
	}
	return true
}
func (a *API) webRestoreExecute(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) || !a.recentRestoreSession(w, r) {
		return
	}
	var in struct {
		Digest string `json:"digest"`
		Text   string `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := a.maintenance().QueueWebRestore(r.Context(), r.PathValue("id"), in.Digest, in.Text)
	if err != nil {
		opsError(w, err)
		return
	}
	if out.State == "pending" || out.State == "running" {
		write(w, 202, out)
		return
	}
	write(w, 200, out)
}
func (a *API) webRestoreReconcile(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) || !opsConfirm(w, r) {
		return
	}
	out, err := a.maintenance().ReconcileWebRestore(r.Context(), r.PathValue("id"))
	if err != nil {
		opsError(w, err)
		return
	}
	write(w, 200, out)
}
