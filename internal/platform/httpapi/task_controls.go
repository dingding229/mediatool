package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/manualdownloads"
	"net/http"
	"time"
)

// Resolve only closed business kinds, never arbitrary tables, queues or SQL.
func (a *API) taskJob(ctx context.Context, kind, id string) (string, error) {
	var job string
	var err error
	switch kind {
	case "jobs":
		return id, nil
	case "downloads":
		err = a.Deps.DB.QueryRow(ctx, `SELECT j.id::text FROM download_tasks d JOIN jobs j ON j.idempotency_key=CASE d.download_state WHEN 'strm_ready' THEN 'emby.refresh:'||d.id::text WHEN 'refresh_requested' THEN 'emby.confirm:'||d.id::text ELSE 'download.add:'||d.id::text END WHERE d.id=$1`, id).Scan(&job)
	case "transfers":
		err = a.Deps.DB.QueryRow(ctx, `SELECT id::text FROM jobs WHERE idempotency_key=$1`, "transfer:"+id).Scan(&job)
	case "scans":
		err = a.Deps.DB.QueryRow(ctx, `SELECT id::text FROM jobs WHERE payload->>'scanId'=$1 AND kind LIKE 'library.scan%' ORDER BY created_at DESC LIMIT 1`, id).Scan(&job)
	case "repairs":
		err = a.Deps.DB.QueryRow(ctx, `SELECT job_id::text FROM strm_repair_batches WHERE id=$1`, id).Scan(&job)
	case "organize":
		err = a.Deps.DB.QueryRow(ctx, `SELECT id::text FROM jobs WHERE kind='files.batch' AND payload->>'batchId'=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&job)
	default:
		return "", errors.New("此类同步操作须使用原预览/核对/批准入口，不通过后台任务重放")
	}
	return job, err
}
func (a *API) taskControl(w http.ResponseWriter, r *http.Request) {
	if a.Deps.DB == nil {
		fail(w, 503, "tasks_unavailable", "后台任务数据库不可用")
		return
	}
	id, kind := r.PathValue("id"), r.PathValue("kind")
	if !isWorkbenchUUID(id) {
		fail(w, 400, "invalid_task", "任务ID无效")
		return
	}
	if kind == "ops" {
		a.directTaskControl(w, r, kind, id)
		return
	}
	job, err := a.taskJob(r.Context(), kind, id)
	if err != nil {
		fail(w, 409, "task_control_unavailable", err.Error())
		return
	}
	repo := jobs.Repository{DB: a.Deps.DB}
	if r.Method == http.MethodGet {
		out, err := repo.ControlStatus(r.Context(), job)
		if err != nil {
			fail(w, 404, "task_not_found", "任务不存在")
			return
		}
		write(w, 200, out)
		return
	}
	var in struct {
		Confirm  bool  `json:"confirm"`
		Revision int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || in.Revision < 1 {
		fail(w, 400, "confirmation_required", "先读取任务并明确确认操作")
		return
	}
	var out jobs.ControlResult
	switch r.PathValue("action") {
	case "cancel":
		out, err = repo.Cancel(r.Context(), job, in.Revision)
		if err == nil && kind == "scans" {
			if e := a.library(); e != nil {
				err = e.ControlScan(r.Context(), id, "cancel")
				if err == nil {
					out.State = "cancelled"
					out.Reason = "扫描已取消，已有部分索引保留；不完整覆盖不计缺失"
				}
			}
		}
	case "retry":
		out, err = repo.Retry(r.Context(), job, in.Revision, a.verifyTask)
	default:
		fail(w, 400, "invalid_task_action", "仅支持取消或核对后重试")
		return
	}
	if err != nil {
		fail(w, 409, "task_control_failed", err.Error())
		return
	}
	status := 200
	if out.Queued || out.StopPending {
		status = 202
	}
	write(w, status, out)
}
func (a *API) verifyTask(ctx context.Context, j jobs.Job) (jobs.Verification, error) {
	var p struct {
		TaskID     string `json:"taskId"`
		ID         string `json:"id"`
		TransferID string `json:"transferId"`
		BatchID    string `json:"batchId"`
		CleanupID  string `json:"cleanupId"`
	}
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return jobs.Verification{}, err
	}
	switch j.Kind {
	case "library.scan":
		e := a.library()
		if e == nil {
			return jobs.Verification{}, errors.New("扫描服务不可用")
		}
		var payload struct {
			ID string `json:"scanId"`
		}
		if err := json.Unmarshal(j.Payload, &payload); err != nil {
			return jobs.Verification{}, err
		}
		scan, err := e.ScanByID(ctx, payload.ID)
		if err != nil {
			return jobs.Verification{}, err
		}
		if scan.State == "succeeded" {
			return jobs.Verification{Complete: true, Reason: "扫描已经完成"}, nil
		}
		if _, err = a.Deps.DB.Exec(ctx, `UPDATE library_scans SET state='paused' WHERE id=$1 AND state IN('running','pending','cancelled')`, payload.ID); err != nil {
			return jobs.Verification{}, err
		}
		if err = e.ControlScan(ctx, payload.ID, "resume"); err != nil {
			return jobs.Verification{}, err
		}
		var next string
		if err = a.Deps.DB.QueryRow(ctx, `SELECT id::text FROM jobs WHERE kind='library.scan' AND payload->>'scanId'=$1 ORDER BY created_at DESC LIMIT 1`, payload.ID).Scan(&next); err != nil {
			return jobs.Verification{}, err
		}
		return jobs.Verification{Complete: true, NextJobID: next, Reason: "原扫描已核对，新一代完整扫描已排队；旧覆盖不作为缺失证据"}, nil
	case "files.batch":
		e, err := a.files()
		if err != nil {
			return jobs.Verification{}, err
		}
		out, err := e.ReconcileBatch(ctx, p.BatchID)
		if err != nil {
			return jobs.Verification{}, err
		}
		if out.State != "succeeded" {
			return jobs.Verification{}, errors.New("已核对整理路径，剩余步骤需要重新预览批准，未重放旧计划")
		}
		return jobs.Verification{Complete: true, Reason: "全部整理目标已核对完成"}, nil
	case "system.check", "search.run", "search.site", "library.scan.root", "library.scan.directory":
		return jobs.Verification{Reason: "只读任务已核对，可重新排队"}, nil
	case "download.add":
		if a.acquisition() == nil {
			return jobs.Verification{}, errors.New("下载服务未配置，保留未知状态")
		}
		ok, err := a.acquisition().VerifyAdd(ctx, p.TaskID)
		return jobs.Verification{Complete: ok, Reason: "已按原下载器、哈希及保存路径核对"}, err
	case "download.manual.add":
		if a.Deps.Integrations == nil {
			return jobs.Verification{}, errors.New("下载连接不可用，保留未知状态")
		}
		e := manualdownloads.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}
		state, err := e.Reconcile(ctx, p.ID)
		if err != nil {
			return jobs.Verification{}, err
		}
		if state == "absent_confirmed" {
			return jobs.Verification{}, errors.New("下载器未见目标但仍可能延迟接受，请在手动下载页明确确认重复风险后重试")
		}

		return jobs.Verification{Complete: state == "completed" || state == "downloading", Reason: "已只读核对原手动下载目标"}, nil
	case "transfer.upload":
		if a.acquisition() == nil {
			return jobs.Verification{}, errors.New("上传服务未配置，保留未知状态")
		}
		err := a.acquisition().Control(ctx, p.TransferID, acquisition.UploadControl{Action: "reconcile", AllowRestartIfAbsent: true})
		if err != nil {
			return jobs.Verification{}, err
		}
		// Verified upload still needs journaled import. The handler skips upload
		// for verified/imported state; this queues the existing remaining phase.
		var complete bool
		if err = a.Deps.DB.QueryRow(ctx, `SELECT state='imported' FROM transfer_tasks WHERE id=$1`, p.TransferID).Scan(&complete); err != nil {
			return jobs.Verification{}, err
		}
		return jobs.Verification{Complete: complete, Reason: "云盘文件及活动会话已核对，只继续未完成阶段"}, nil
	case "download.cleanup":
		e := a.acquisition()
		if e == nil {
			return jobs.Verification{}, errors.New("清理服务不可用")
		}
		if err := e.ReconcileCleanup(ctx, p.TaskID); err != nil {
			return jobs.Verification{}, err
		}
		var complete bool
		if err := a.Deps.DB.QueryRow(ctx, `SELECT state='succeeded' FROM download_cleanup_requests WHERE task_id=$1`, p.TaskID).Scan(&complete); err != nil {
			return jobs.Verification{}, err
		}
		if !complete {
			return jobs.Verification{}, errors.New("清理已核对但未完成，仍需原清理入口确认做种要求和范围")
		}
		return jobs.Verification{Complete: true, Reason: "下载器与本地清理结果已核对完成"}, nil
	case "library.strm.repair":
		e := a.library()
		if e == nil {
			return jobs.Verification{}, errors.New("修复服务不可用")
		}
		return e.VerifyRepairJob(ctx, p.BatchID)
	default:
		return jobs.Verification{}, errors.New("此任务不能证明可安全重放；保留未知状态，请使用原业务核对/预览入口")
	}
}

// Host maintenance remains a synchronous ledger, not a queue job.
func (a *API) directTaskControl(w http.ResponseWriter, r *http.Request, kind, id string) {
	var state string
	if err := a.Deps.DB.QueryRow(r.Context(), `SELECT state FROM ops_requests WHERE id=$1`, id).Scan(&state); err != nil {
		fail(w, 404, "task_not_found", "操作记录不存在")
		return
	}
	result := jobs.ControlResult{ID: id, State: state, Revision: 1, Reason: "同步操作仅沿原批准范围核对", ExternalEffects: "已发生的宿主操作不能撤销"}
	if r.Method == http.MethodGet {
		write(w, 200, result)
		return
	}
	var in struct {
		Confirm  bool  `json:"confirm"`
		Revision int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || in.Revision != 1 {
		fail(w, 400, "confirmation_required", "请先读取当前操作，再明确确认")
		return
	}
	bounded, stop := context.WithTimeout(r.Context(), 3*time.Second)
	defer stop()
	r = r.WithContext(bounded)
	if r.PathValue("action") == "cancel" {
		tag, e := a.Deps.DB.Exec(r.Context(), `UPDATE ops_requests SET state='cancelled',message='管理员取消' WHERE id=$1 AND state IN('preview','approved','cancelled')`, id)
		if e != nil || tag.RowsAffected() != 1 {
			fail(w, 409, "task_not_cancellable", "宿主已执行的维护操作不能由应用撤回；请核对原维护单")
			return
		}
		result.State = "cancelled"
		result.Reason = "已取消尚未执行的批准"
		write(w, 200, result)
		return
	}
	if r.PathValue("action") != "retry" {
		fail(w, 400, "invalid_task_action", "操作无效")
		return
	}
	fail(w, 409, "host_verification_required", "宿主维护必须先使用原维护单核验，不通过通用接口重放")
}

func (a *API) registerAccountTasks(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v1/auth/sessions", a.protected(a.listAdministratorSessions))
	mux.HandleFunc("POST /api/v1/auth/sessions/{id}/revoke", a.protected(a.revokeAdministratorSession))
	mux.HandleFunc("GET /api/v1/tasks/{kind}/{id}", a.protected(a.taskControl))
	mux.HandleFunc("POST /api/v1/tasks/{kind}/{id}/{action}", a.protected(a.taskControl))
}
