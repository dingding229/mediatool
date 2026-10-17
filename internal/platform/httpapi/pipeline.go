package httpapi

import (
	"encoding/json"
	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/messaging"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"strings"
	"time"
)

func (a *API) listTransfers(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	rows, err := a.Deps.DB.Query(r.Context(), `SELECT t.id::text,d.id::text,t.destination,t.state,t.phase,t.bytes_transferred,t.hash_bytes,f.size_bytes,t.last_error,t.updated_at,t.control_revision,f.id::text FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id JOIN download_tasks d ON d.id=f.task_id ORDER BY t.created_at DESC LIMIT 200`)
	if err != nil {
		fail(w, 503, "transfers_unavailable", "传输记录暂不可用")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, taskID, destination, state, phase, message, assetID string
		var bytes, hashed, total, revision int64
		var updated time.Time
		if rows.Scan(&id, &taskID, &destination, &state, &phase, &bytes, &hashed, &total, &message, &updated, &revision, &assetID) != nil {
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
		out = append(out, map[string]any{"id": id, "taskId": taskID, "destination": destination, "state": state, "phase": phase, "processingProgress": progress, "processingStage": stage, "transferMethod": method, "bytes": bytes, "hashBytes": hashed, "total": total, "message": message, "updatedAt": updated, "controlRevision": revision, "assetId": assetID})
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
	kind := r.PathValue("kind")
	id := r.PathValue("id")
	if kind != "downloads" && kind != "transfers" {
		fail(w, 400, "invalid_task", "不支持的任务类型")
		return
	}
	tx, err := a.Deps.DB.Begin(r.Context())
	if err != nil {
		fail(w, 503, "retry_unavailable", "数据库暂不可用")
		return
	}
	defer deferutil.Rollback(r.Context(), tx)
	var pattern string
	if kind == "downloads" {
		var state string
		var sourceRequired bool
		if tx.QueryRow(r.Context(), `SELECT download_state,source_recovery_required FROM download_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&state, &sourceRequired) != nil {
			fail(w, 404, "task_not_found", "下载任务不存在")
			return
		}
		if sourceRequired {
			fail(w, 409, "source_reconfirm_required", "请先预览并重新确认下载来源；通用重试不会更换快照")
			return
		}
		if state != "pending" && state != "refresh_requested" && state != "strm_ready" {
			fail(w, 409, "task_not_retryable", "当前任务阶段不允许直接重试")
			return
		}
		if state == "pending" {
			pattern = "download.add:" + id
		} else if state == "strm_ready" {
			pattern = "emby.refresh:" + id
		} else {
			pattern = "emby.confirm:" + id
		}
	} else {
		var state, session string
		if tx.QueryRow(r.Context(), `SELECT state,coalesce(upload_id,'') FROM transfer_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&state, &session) != nil {
			fail(w, 404, "task_not_found", "传输任务不存在")
			return
		}
		if state == "needs_output_reconciliation" {
			fail(w, 409, "output_reconciliation_required", "请先在云盘索引明确预览/确认输出迁移，再使用恢复入库；普通重试不会自动追加布局")
			return
		}
		if state == "imported" {
			fail(w, 409, "task_not_retryable", "此文件已入库")
			return
		}
		if (state == "starting" || state == "uncertain") && session == "" {
			fail(w, 409, "upload_outcome_unknown", "上传启动结果未知，请先在CloudDrive2核对，不能盲目重建会话")
			return
		}
		pattern = "transfer:" + id
	}
	tag, err := tx.Exec(r.Context(), `UPDATE jobs SET state='pending',attempts=0,last_error=NULL,available_at=now(),updated_at=now()WHERE idempotency_key=$1 AND state='failed'`, pattern)
	if err != nil {
		fail(w, 503, "retry_unavailable", "任务重试失败")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "task_not_retryable", "任务正在运行、尚未失败或没有对应后台任务")
		return
	}
	audit, _ := json.Marshal(map[string]string{"taskId": id, "kind": kind})
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','pipeline.retry',$1)`, audit); err != nil {
		fail(w, 503, "retry_unavailable", "审计写入失败")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 503, "retry_unavailable", "任务重试未提交")
		return
	}
	write(w, 200, map[string]bool{"queued": true})
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
	if a.Deps.DB.QueryRow(r.Context(), `SELECT integration_id::text,destination,coalesce(endpoint_snapshot,'')FROM transfer_tasks WHERE id=$1`, r.PathValue("id")).Scan(&connectionID, &destination, &endpoint) != nil {
		fail(w, 404, "transfer_unavailable", "上传任务不存在")
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
