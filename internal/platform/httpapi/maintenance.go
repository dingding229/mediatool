package httpapi

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"media-tool/internal/modules/maintenance"
	"media-tool/internal/platform/buildinfo"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

func (a *API) maintenance() maintenance.Engine {
	return maintenance.Engine{DB: a.Deps.DB, Redis: a.Deps.Redis, DSN: a.Config.DatabaseURL, Root: a.Config.MaintenanceDir, OutputRoot: a.Config.STRMOutputDir, Key: a.Config.SecretKey, HostProc: a.Config.HostProcDir, Revision: buildinfo.Revision}
}
func (a *API) registerMaintenance(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/ops", a.protected(a.opsOverview))
	mux.HandleFunc("PUT /api/v1/ops/policy", a.protected(a.opsPolicy))
	mux.HandleFunc("POST /api/v1/ops/backups", a.protected(a.opsBackup))
	mux.HandleFunc("POST /api/v1/ops/backups/{id}/drill", a.protected(a.opsDrill))
	mux.HandleFunc("POST /api/v1/ops/drills/{id}/cleanup", a.protected(a.opsCleanupDrill))
	mux.HandleFunc("POST /api/v1/ops/backups/{id}/pin", a.protected(a.opsPin))
	mux.HandleFunc("POST /api/v1/ops/backups/{id}/delete", a.protected(a.opsDelete))
	mux.HandleFunc("GET /api/v1/ops/backups/{id}/download", a.protected(a.opsDownload))
	mux.HandleFunc("GET /api/v1/ops/trends", a.protected(a.opsTrends))
	mux.HandleFunc("POST /api/v1/ops/alerts/{code}/ack", a.protected(a.opsAck))
	mux.HandleFunc("POST /api/v1/ops/requests/preview", a.protected(a.opsPreview))
	mux.HandleFunc("POST /api/v1/ops/requests/{id}/verify", a.protected(a.opsVerifyExecution))
	mux.HandleFunc("POST /api/v1/ops/requests/{id}/approve", a.protected(a.opsApprove))
	mux.HandleFunc("POST /api/v1/ops/requests/{id}/cancel", a.protected(a.opsCancel))
	mux.HandleFunc("POST /api/v1/ops/resume", a.protected(a.opsResume))
}
func (a *API) opsReady(w http.ResponseWriter) bool {
	w.Header().Set("Cache-Control", "no-store")
	if a.Deps.DB == nil {
		fail(w, 503, "ops_unavailable", "维护中心需要PostgreSQL")
		return false
	}
	return true
}
func opsError(w http.ResponseWriter, err error) {
	status := 400
	if errors.Is(err, maintenance.ErrConflict) {
		status = 409
	}
	fail(w, status, "maintenance_rejected", err.Error())
}
func (a *API) opsOverview(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	e := a.maintenance()
	policy, err := e.Policy(ctx)
	if err != nil {
		opsError(w, err)
		return
	}
	backups, err := e.Backups(ctx)
	if err != nil {
		opsError(w, err)
		return
	}
	drills, err := e.Drills(ctx)
	if err != nil {
		opsError(w, err)
		return
	}
	releases, err := e.Releases(ctx)
	if err != nil {
		opsError(w, err)
		return
	}
	requests, err := e.Requests(ctx)
	if err != nil {
		opsError(w, err)
		return
	}
	alerts, err := e.Alerts(ctx)
	if err != nil {
		opsError(w, err)
		return
	}
	var paused bool
	var revision int64
	var reason string
	if err = e.DB.QueryRow(ctx, `SELECT paused,reason,revision FROM ops_mode WHERE singleton`).Scan(&paused, &reason, &revision); err != nil {
		opsError(w, err)
		return
	}
	_, dumpErr := exec.LookPath("pg_dump")
	_, restoreErr := exec.LookPath("pg_restore")
	var total int64
	if checkedErr := e.DB.QueryRow(ctx, `SELECT coalesce(sum(bytes),0)FROM ops_backups WHERE state<>'deleted'`).Scan(&total); checkedErr != nil {
		fail(w, 503, "operation_failed", "读取或写入失败，请稍后重试")
		return
	}
	write(w, 200, map[string]any{"policy": policy, "backups": backups, "drills": drills, "releases": releases, "requests": requests, "alerts": alerts, "mode": map[string]any{"paused": paused, "reason": reason, "revision": revision}, "toolsReady": dumpErr == nil && restoreErr == nil, "totalBytes": total, "revision": buildinfo.Revision, "version": buildinfo.Version, "schema": buildinfo.Schema})
}
func (a *API) opsPolicy(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) {
		return
	}
	var in maintenance.PolicyRecord
	if !decode(w, r, &in) {
		return
	}
	out, err := a.maintenance().SavePolicy(r.Context(), in)
	if err != nil {
		opsError(w, err)
		return
	}
	write(w, 200, out)
}
func opsConfirm(w http.ResponseWriter, r *http.Request) bool {
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return false
	}
	if !in.Confirm {
		fail(w, 400, "confirmation_required", "请明确确认此维护操作")
		return false
	}
	return true
}
func (a *API) opsBackup(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) || !opsConfirm(w, r) {
		return
	}
	id, err := a.maintenance().QueueBackup(r.Context(), "manual")
	if err != nil {
		opsError(w, err)
		return
	}
	write(w, 202, map[string]string{"id": id})
}
func (a *API) opsDrill(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) || !opsConfirm(w, r) {
		return
	}
	id, err := a.maintenance().QueueDrill(r.Context(), r.PathValue("id"))
	if err != nil {
		opsError(w, err)
		return
	}
	write(w, 202, map[string]string{"id": id})
}
func (a *API) opsPin(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) {
		return
	}
	var in struct {
		Pinned  bool `json:"pinned"`
		Confirm bool `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		opsError(w, maintenance.ErrUnsafe)
		return
	}
	if err := a.maintenance().Pin(r.Context(), r.PathValue("id"), in.Pinned); err != nil {
		opsError(w, err)
		return
	}
	write(w, 200, map[string]bool{"saved": true})
}
func (a *API) opsDelete(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) || !opsConfirm(w, r) {
		return
	}
	if err := a.maintenance().Delete(r.Context(), r.PathValue("id")); err != nil {
		opsError(w, err)
		return
	}
	write(w, 200, map[string]bool{"deleted": true})
}
func (a *API) opsDownload(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) {
		return
	}
	e := a.maintenance()
	_, dir, err := e.VerifyBundle(r.Context(), r.PathValue("id"))
	if err != nil {
		opsError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", `attachment; filename="mediatool-backup-`+r.PathValue("id")+`.tar"`)
	tw := tar.NewWriter(w)
	defer deferutil.Close(tw, "Read/descriptor teardown only; primary operations handle their own I/O result")
	for _, name := range []string{"manifest.json", "database.dump", "outputs.tar"} {
		file, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return
		}
		info, err := file.Stat()
		if err != nil {
			deferutil.Close(file, "Read/descriptor teardown only; primary operations handle their own I/O result")
			return
		}
		if err = tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: info.Size()}); err != nil {
			deferutil.Close(file, "Read/descriptor teardown only; primary operations handle their own I/O result")
			return
		}
		_, err = io.Copy(tw, file)
		deferutil.Close(file, "Read/descriptor teardown only; primary operations handle their own I/O result")
		if err != nil {
			return
		}
	}
}
func (a *API) opsTrends(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) {
		return
	}
	hours, err := strconv.Atoi(r.URL.Query().Get("hours"))
	if err != nil {
		opsError(w, maintenance.ErrUnsafe)
		return
	}
	out, err := a.maintenance().History(r.Context(), hours)
	if err != nil {
		opsError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) opsAck(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) || !opsConfirm(w, r) {
		return
	}
	tag, err := a.Deps.DB.Exec(r.Context(), `UPDATE ops_alerts SET acknowledged_at=now() WHERE code=$1 AND state='active'`, r.PathValue("code"))
	if err != nil || tag.RowsAffected() != 1 {
		opsError(w, maintenance.ErrConflict)
		return
	}
	write(w, 200, map[string]bool{"acknowledged": true})
}
func (a *API) opsPreview(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) {
		return
	}
	var in struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := a.maintenance().Preview(r.Context(), in.Kind, in.ID)
	if err != nil {
		opsError(w, err)
		return
	}
	write(w, 200, map[string]any{"request": out, "confirmText": maintenance.ConfirmText(out), "command": "media-toolctl --root /root/docker/mediatool execute " + out.ID})
}
func (a *API) opsApprove(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) {
		return
	}
	var in struct {
		Digest  string `json:"digest"`
		Text    string `json:"text"`
		Confirm bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		opsError(w, maintenance.ErrUnsafe)
		return
	}
	if err := a.maintenance().Approve(r.Context(), r.PathValue("id"), in.Digest, in.Text); err != nil {
		opsError(w, err)
		return
	}
	write(w, 200, map[string]any{"state": "approved", "message": "已审批，等待宿主机受控执行；当前尚未恢复或回滚", "flow": maintenance.RequestFlow("approved", false, false)})
}
func (a *API) opsCancel(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) || !opsConfirm(w, r) {
		return
	}
	tag, err := a.Deps.DB.Exec(r.Context(), `UPDATE ops_requests SET state='cancelled',message='管理员取消' WHERE id=$1 AND state IN('preview','approved')`, r.PathValue("id"))
	if err != nil || tag.RowsAffected() != 1 {
		opsError(w, maintenance.ErrConflict)
		return
	}
	write(w, 200, map[string]bool{"cancelled": true})
}
func (a *API) opsResume(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) {
		return
	}
	var in struct {
		Revision int64  `json:"revision"`
		Text     string `json:"text"`
		Confirm  bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || in.Text != "解除维护并恢复业务" {
		opsError(w, maintenance.ErrUnsafe)
		return
	}
	if err := a.maintenance().Resume(r.Context(), in.Revision); err != nil {
		opsError(w, err)
		return
	}
	write(w, 200, map[string]bool{"resumed": true})
}

func (a *API) opsCleanupDrill(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) || !opsConfirm(w, r) {
		return
	}
	if err := a.maintenance().CleanupDrill(r.Context(), r.PathValue("id")); err != nil {
		opsError(w, err)
		return
	}
	write(w, 200, map[string]bool{"cleaned": true})
}

func (a *API) opsVerifyExecution(w http.ResponseWriter, r *http.Request) {
	if !a.opsReady(w) || !opsConfirm(w, r) {
		return
	}
	out, e := a.maintenance().VerifyExecution(r.Context(), r.PathValue("id"))
	if e != nil {
		opsError(w, e)
		return
	}
	write(w, 200, out)
}
