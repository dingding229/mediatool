package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/messaging"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/flow"
	"media-tool/internal/platform/httpjson"
	"net/http"
	"strings"
	"time"
)

func (a *API) listTransfers(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	rows, err := a.Deps.DB.Query(r.Context(), `SELECT t.id::text,d.id::text,t.destination,t.state,t.phase,t.bytes_transferred,t.hash_bytes,f.size_bytes,t.last_error,t.updated_at,t.control_revision,f.id::text,coalesce((SELECT state FROM jobs WHERE idempotency_key='transfer:'||t.id::text),'') FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id JOIN download_tasks d ON d.id=f.task_id ORDER BY t.created_at DESC LIMIT 200`)
	if err != nil {
		fail(w, 503, "transfers_unavailable", "传输记录暂不可用")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, taskID, destination, state, phase, message, assetID, jobState string
		var bytes, hashed, total, revision int64
		var updated time.Time
		if rows.Scan(&id, &taskID, &destination, &state, &phase, &bytes, &hashed, &total, &message, &updated, &revision, &assetID, &jobState) != nil {
			fail(w, 503, "transfers_unavailable", "传输记录读取失败")
			return
		}
		if strings.HasPrefix(phase, "output_resume:") {
			phase = "output_resume_approved"
		}
		progress, stage := acquisition.TransferProgress(state, phase, bytes, hashed, total)
		method := "stream"
		if (state == "verified" || state == "imported" || phase == "verified") && bytes < total {
			method = "instant_or_reused"
		}
		out = append(out, map[string]any{"id": id, "taskId": taskID, "destination": destination, "state": state, "phase": phase, "processingProgress": progress, "processingStage": stage, "transferMethod": method, "bytes": bytes, "hashBytes": hashed, "total": total, "message": message, "updatedAt": updated, "controlRevision": revision, "assetId": assetID, "flow": acquisition.TransferFlow(state, jobState)})
	}
	if rows.Err() != nil {
		fail(w, 503, "records_unavailable", "记录读取中断，请刷新而非重建任务")
		return
	}
	write(w, 200, out)
}
func (a *API) listMedia(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	rows, err := a.Deps.DB.Query(r.Context(), `SELECT m.id::text,m.metadata,count(b.asset_id) FROM media_items m LEFT JOIN media_asset_bindings b ON b.media_id=m.id GROUP BY m.id ORDER BY m.title LIMIT 200`)
	if err != nil {
		fail(w, 503, "media_unavailable", "媒体索引暂不可用")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id string
		var raw []byte
		var count int
		if rows.Scan(&id, &raw, &count) != nil {
			continue
		}
		var meta map[string]any
		if checkedErr := json.Unmarshal(raw, &meta); checkedErr != nil {
			fail(w, 503, "operation_failed", "读取或写入失败，请稍后重试")
			return
		}
		out = append(out, map[string]any{"id": id, "metadata": meta, "assetCount": count})
	}
	write(w, 200, out)
}
func (a *API) listNotifications(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	out, err := (messaging.Dispatcher{DB: a.Deps.DB, Connections: a.Deps.Integrations}).List(r.Context())
	if err != nil {
		fail(w, 503, "notifications_unavailable", "通知记录暂不可用")
		return
	}
	write(w, 200, out)
}
func (a *API) retryPipeline(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	kind, id := r.PathValue("kind"), r.PathValue("id")
	if (kind != "downloads" && kind != "transfers") || !isWorkbenchUUID(id) {
		fail(w, 400, "invalid_task", "任务类型或ID无效")
		return
	}
	tx, err := a.Deps.DB.Begin(r.Context())
	if err != nil {
		pipelineReadError(w, err)
		return
	}
	defer deferutil.Rollback(r.Context(), tx)
	var pattern, state string
	var status flow.Status
	if kind == "downloads" {
		var sourceRequired bool
		err = tx.QueryRow(r.Context(), `SELECT download_state,source_recovery_required FROM download_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&state, &sourceRequired)
		if pipelineReadError(w, err) {
			return
		}
		pattern = acquisition.DownloadRetryKey(state, id)
		status = acquisition.DownloadFlow(state, sourceRequired, "failed")
	} else {
		err = tx.QueryRow(r.Context(), `SELECT state FROM transfer_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&state)
		if pipelineReadError(w, err) {
			return
		}
		pattern = "transfer:" + id
		status = acquisition.TransferFlow(state, "failed")
	}
	if !status.Retryable {
		pipelineFailure(w, 409, status.Reason, "请按当前阶段的下一步操作核对；普通重试不会重放未知结果或绕过确认", status)
		return
	}
	var job string
	err = tx.QueryRow(r.Context(), `SELECT state FROM jobs WHERE idempotency_key=$1 FOR UPDATE`, pattern).Scan(&job)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		pipelineReadError(w, err)
		return
	}
	if job != "failed" {
		status.Retryable = false
		status.NextAction = "wait"
		status.Reason = "job_active"
		if job == "" {
			status.NextAction = "inspect_task"
			status.Reason = "job_missing"
		}
		pipelineFailure(w, 409, "task_not_retryable", "没有已失败的后台任务可重试；请刷新或核对任务", status)
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE jobs SET state='pending',attempts=0,last_error=NULL,available_at=now(),updated_at=now()WHERE idempotency_key=$1 AND state='failed'`, pattern)
	if err != nil {
		pipelineReadError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		pipelineFailure(w, 409, "task_not_retryable", "后台任务状态已变化，请刷新", flow.Status{Phase: status.Phase, NextAction: "refresh_status", Reason: "job_changed"})
		return
	}
	audit, err := json.Marshal(map[string]string{"taskId": id, "kind": kind})
	if err != nil {
		pipelineReadError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','pipeline.retry',$1)`, audit); err != nil {
		pipelineReadError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		pipelineFailure(w, 503, "retry_unavailable", "提交结果未知，先刷新核对，不要盲目重复", flow.Status{Phase: status.Phase, NextAction: "verify", Reason: "commit_outcome_unknown"})
		return
	}
	write(w, 200, map[string]any{"queued": true, "flow": flow.Status{Phase: status.Phase, NextAction: "wait", Reason: "retry_queued"}})
}
func (a *API) retryNotification(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var body struct {
		AcceptDuplicateRisk bool `json:"acceptDuplicateRisk"`
	}
	if !decode(w, r, &body) {
		return
	}
	dispatcher := messaging.Dispatcher{DB: a.Deps.DB, Connections: a.Deps.Integrations}
	if err := dispatcher.Retry(r.Context(), r.PathValue("id"), body.AcceptDuplicateRisk); err != nil {
		fail(w, 409, "notification_retry_blocked", err.Error())
		return
	}
	write(w, 200, map[string]bool{"queued": true})
}

func (a *API) controlUpload(w http.ResponseWriter, r *http.Request) {
	engine := a.acquisition()
	if engine == nil {
		fail(w, 503, "transfer_unavailable", "请先连接数据库")
		return
	}
	var body acquisition.UploadControl
	if !decode(w, r, &body) {
		return
	}
	if err := engine.Control(r.Context(), r.PathValue("id"), body); err != nil {
		fail(w, 409, "transfer_control_failed", err.Error())
		return
	}
	write(w, 200, map[string]bool{"accepted": true})
}

func (a *API) transferSessions(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var connectionID, destination, endpoint string
	if pipelineReadError(w, a.Deps.DB.QueryRow(r.Context(), `SELECT integration_id::text,destination,coalesce(endpoint_snapshot,'')FROM transfer_tasks WHERE id=$1`, r.PathValue("id")).Scan(&connectionID, &destination, &endpoint)) {
		return
	}
	c, err := a.Deps.Integrations.Repository.Get(r.Context(), connectionID)
	if err != nil || endpoint != "" && endpoint != c.String("endpoint") {
		fail(w, 409, "endpoint_changed", "先恢复原上传端点再核对会话")
		return
	}
	client, err := a.Deps.Integrations.CloudByID(r.Context(), connectionID)
	if err != nil {
		fail(w, 503, "cloud_unavailable", "CloudDrive2不可用")
		return
	}
	list, err := client.UploadsAt(r.Context(), destination)
	if err != nil {
		fail(w, 502, "sessions_unavailable", "无法完整获取远端会话，不能据此重建上传")
		return
	}
	write(w, 200, list)
}

func pipelineReadError(w http.ResponseWriter, e error) bool {
	if e == nil {
		return false
	}
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 404, "task_not_found", "任务不存在")
	} else {
		pipelineFailure(w, 503, "pipeline_unavailable", "任务暂不可读取，请刷新核对，不要重建任务", flow.Status{Phase: "inspect", NextAction: "refresh_status", Reason: "storage_unavailable"})
	}
	return true
}
func pipelineFailure(w http.ResponseWriter, status int, code, message string, f flow.Status) {
	if e := httpjson.Write(w, status, Envelope{Error: &APIError{Code: code, Message: message, Flow: &f}}); e != nil {
		deferutil.Report(e, "pipeline response")
	}
}
