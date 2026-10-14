package httpapi

import (
	"context"
	"media-tool/internal/modules/filemanager"
	"media-tool/internal/modules/library"
	"net/http"
	"strings"
	"time"
)

func (a *API) previewFileBatch(w http.ResponseWriter, r *http.Request) {
	e, err := a.files()
	if err != nil {
		fileError(w, err)
		return
	}
	var in struct {
		Items []filemanager.Input `json:"items"`
	}
	if !decode(w, r, &in) {
		return
	}
	batch, err := e.PreviewBatch(r.Context(), in.Items)
	if err != nil {
		fileError(w, err)
		return
	}
	write(w, 200, batch)
}
func (a *API) getFileBatch(w http.ResponseWriter, r *http.Request) {
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_id", "ID无效")
		return
	}
	e, err := a.files()
	if err != nil {
		fileError(w, err)
		return
	}
	out, err := e.Batch(r.Context(), r.PathValue("id"))
	if err != nil {
		fileError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) approveFileBatch(w http.ResponseWriter, r *http.Request) {
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_id", "ID无效")
		return
	}
	e, err := a.files()
	if err != nil {
		fileError(w, err)
		return
	}
	var in struct {
		Digest  string `json:"digest"`
		Confirm bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := e.ApproveBatch(r.Context(), r.PathValue("id"), in.Digest, in.Confirm)
	if err != nil {
		fileError(w, err)
		return
	}
	write(w, 202, out)
}
func (a *API) reconcileFileBatch(w http.ResponseWriter, r *http.Request) {
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_id", "ID无效")
		return
	}
	e, err := a.files()
	if err != nil {
		fileError(w, err)
		return
	}
	var in struct{}
	if !decode(w, r, &in) {
		return
	}
	out, err := e.ReconcileBatch(r.Context(), r.PathValue("id"))
	if err != nil {
		fileError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) controlScan(w http.ResponseWriter, r *http.Request) {
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_id", "ID无效")
		return
	}
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	var in struct{}
	if !decode(w, r, &in) {
		return
	}
	if err := e.ControlScan(r.Context(), r.PathValue("id"), r.PathValue("action")); err != nil {
		fail(w, 409, "scan_control_failed", err.Error())
		return
	}
	write(w, 200, map[string]bool{"accepted": true})
}
func (a *API) scanDetails(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	p, err := pagination(r)
	if err != nil {
		fail(w, 400, "invalid_page", "分页无效")
		return
	}
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_id", "扫描ID无效")
		return
	}
	found, err := e.ScanByID(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, "scan_not_found", "扫描不存在")
		return
	}
	rows, err := a.Deps.DB.Query(r.Context(), `SELECT path,state FROM library_scan_directories WHERE scan_id=$1 ORDER BY path LIMIT $2 OFFSET $3`, r.PathValue("id"), p.Limit, p.Offset)
	if err != nil {
		fail(w, 503, "scan_unavailable", "扫描游标不可用")
		return
	}
	defer rows.Close()
	dirs := []map[string]string{}
	for rows.Next() {
		var path, state string
		if rows.Scan(&path, &state) != nil {
			fail(w, 503, "scan_unavailable", "扫描游标不可用")
			return
		}
		dirs = append(dirs, map[string]string{"path": path, "state": state})
	}
	if rows.Err() != nil {
		fail(w, 503, "scan_unavailable", "扫描游标不可用")
		return
	}
	write(w, 200, map[string]any{"scan": found, "directories": dirs})
}

func (a *API) recognizeCandidate(w http.ResponseWriter, r *http.Request) {
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_id", "ID无效")
		return
	}
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	var in struct{}
	if !decode(w, r, &in) {
		return
	}
	out, err := e.Recognize(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 409, "recognition_failed", err.Error())
		return
	}
	write(w, 200, out)
}

func (a *API) libraryActivity(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	p, err := pagination(r)
	if err != nil {
		fail(w, 400, "invalid_page", "分页无效")
		return
	}
	f := library.HistoryFilter{Kind: r.URL.Query().Get("kind"), State: r.URL.Query().Get("state"), Query: r.URL.Query().Get("q"), ConnectionID: r.URL.Query().Get("connectionId"), BatchID: r.URL.Query().Get("batchId"), Limit: p.Limit, Offset: p.Offset}
	for _, id := range []string{f.ConnectionID, f.BatchID} {
		if id != "" && !isWorkbenchUUID(id) {
			fail(w, 400, "invalid_id", "筛选ID无效")
			return
		}
	}
	if f.Kind != "" && !strings.Contains("|file|batch|scan|transfer|output|emby|strmRepair|", "|"+f.Kind+"|") {
		fail(w, 400, "invalid_kind", "历史类型无效")
		return
	}
	for key, target := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if raw := r.URL.Query().Get(key); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				fail(w, 400, "invalid_time", "时间必须为RFC3339")
				return
			}
			*target = &parsed
		}
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		fail(w, 400, "invalid_time", "开始时间须早于结束时间")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	items, total, err := e.History(ctx, f)
	if err != nil {
		fail(w, 503, "history_unavailable", "文件与入库历史暂不可用")
		return
	}
	write(w, 200, map[string]any{"items": items, "total": total})
}
func (a *API) managedOutputs(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	p, err := pagination(r)
	if err != nil || len(r.URL.Query().Get("q")) > 256 {
		fail(w, 400, "invalid_page", "筛选无效")
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind != "" && kind != "strm" {
		fail(w, 400, "invalid_kind", "输出类型无效")
		return
	}
	items, total, err := e.ManagedAssetsKind(r.Context(), p.Limit, p.Offset, r.URL.Query().Get("q"), kind == "strm")
	if err != nil {
		fail(w, 503, "outputs_unavailable", "托管输出清单暂不可用")
		return
	}
	write(w, 200, map[string]any{"items": items, "total": total})
}
func (a *API) checkManagedEmby(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	var in struct{}
	if !decode(w, r, &in) {
		return
	}
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_id", "资产ID无效")
		return
	}
	if err := e.QueueRefresh(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 503, "refresh_unavailable", "刷新请求无法排队")
		return
	}
	write(w, 202, map[string]string{"state": "pending", "message": "已排队定向刷新；需逐路径可见性确认，不代表已入库或可解码"})
}

func isWorkbenchUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

func (a *API) managedOutputContent(w http.ResponseWriter, r *http.Request) {
	if !isWorkbenchUUID(r.PathValue("id")) {
		fail(w, 400, "invalid_id", "资产ID无效")
		return
	}
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	out, err := e.OutputContent(r.Context(), r.PathValue("id"), r.URL.Query().Get("path"))
	if err != nil {
		fail(w, 409, "output_conflict", "输出不存在、已改变或不属于当前资产，不读取未知文件，请使用修复流程核对")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}

func (a *API) removeLibraryHistory(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	var in struct {
		Items   []library.HistorySelection `json:"items"`
		Confirm bool                       `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	for _, item := range in.Items {
		if !isWorkbenchUUID(item.ID) {
			fail(w, 400, "invalid_id", "历史ID无效")
			return
		}
	}
	if err := e.HideHistory(r.Context(), in.Items, in.Confirm); err != nil {
		fail(w, 409, "history_changed", err.Error())
		return
	}
	write(w, 200, map[string]any{"removed": len(in.Items), "mediaDeleted": false, "recoveryEvidenceRetained": true})
}
