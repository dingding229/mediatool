package httpapi

import (
	"media-tool/internal/modules/library"
	"net/http"
)

func (a *API) library() *library.Engine {
	if a.Deps.Integrations == nil {
		return nil
	}
	return &library.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}
}
func (a *API) scanLibrary(w http.ResponseWriter, r *http.Request) {
	engine := a.library()
	if engine == nil {
		fail(w, 503, "library_unavailable", "请先配置数据库和服务连接")
		return
	}
	var in struct {
		Root string `json:"root"`
	}
	if !decode(w, r, &in) {
		return
	}
	id, err := engine.Start(r.Context(), in.Root)
	if err != nil {
		fail(w, 400, "scan_create_failed", err.Error())
		return
	}
	write(w, 202, map[string]string{"id": id})
}
func (a *API) listLibraryScans(w http.ResponseWriter, r *http.Request) {
	engine := a.library()
	if engine == nil {
		fail(w, 503, "library_unavailable", "请先连接数据库")
		return
	}
	if servePage(w, r, a.Deps.DB, `SELECT count(*) FROM library_scans`, nil, func(start, limit int) ([]library.Scan, error) { return engine.ScansPage(r.Context(), start, limit) }) {
		return
	}
	list, err := engine.Scans(r.Context())
	if err != nil {
		fail(w, 503, "scans_unavailable", "扫描记录暂不可用")
		return
	}
	write(w, 200, list)
}
func (a *API) listLibraryCandidates(w http.ResponseWriter, r *http.Request) {
	engine := a.library()
	if engine == nil {
		fail(w, 503, "library_unavailable", "请先连接数据库")
		return
	}
	p, err := pagination(r)
	if err != nil {
		fail(w, 400, "invalid_pagination", "分页参数无效")
		return
	}
	state := r.URL.Query().Get("state")
	if state == "pending" {
		state = "needs_confirmation"
	} // backwards-compatible UI/API alias
	if state != "" && state != "needs_confirmation" && state != "confirmed" && state != "artifacts_pending" {
		fail(w, 400, "invalid_state", "候选状态无效")
		return
	}
	limit := p.Limit
	if !p.Paged {
		limit = 500
	}
	list, err := engine.CandidatePage(r.Context(), limit, p.Offset, state)
	if err != nil {
		fail(w, 503, "candidates_unavailable", "候选记录暂不可用")
		return
	}
	if p.Paged {
		var total int
		if a.Deps.DB.QueryRow(r.Context(), `SELECT count(*)FROM library_candidates WHERE ($1='' OR state=$1 OR ($1='needs_confirmation' AND state IN ('pending','needs_output_reconciliation')))`, state).Scan(&total) != nil {
			fail(w, 503, "candidates_unavailable", "读取分页失败")
			return
		}
		write(w, 200, map[string]any{"items": list, "total": total, "page": p.Page, "pageSize": p.Limit})
		return
	}
	write(w, 200, list)
}
func (a *API) confirmLibraryCandidate(w http.ResponseWriter, r *http.Request) {
	engine := a.library()
	if engine == nil {
		fail(w, 503, "library_unavailable", "请先连接数据库")
		return
	}
	var in library.Confirmation
	if !decode(w, r, &in) {
		return
	}
	id, err := engine.Confirm(r.Context(), r.PathValue("id"), in)
	if err != nil {
		fail(w, 400, "library_confirm_failed", err.Error())
		return
	}
	write(w, 200, map[string]string{"assetId": id})
}

func (a *API) listScanSchedules(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	list, err := e.Schedules(r.Context())
	if err != nil {
		fail(w, 503, "schedule_unavailable", "定时扫描不可用")
		return
	}
	write(w, 200, list)
}
func (a *API) saveScanSchedule(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	var in library.ScheduleConfig
	if !decode(w, r, &in) {
		return
	}
	saved, err := e.SaveSchedule(r.Context(), in)
	if err != nil {
		fail(w, 400, "schedule_invalid", err.Error())
		return
	}
	write(w, 200, saved)
}
func (a *API) batchConfirmCandidates(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	var in struct {
		Items []struct {
			ID           string               `json:"id"`
			Confirmation library.Confirmation `json:"confirmation"`
		} `json:"items"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Items) < 1 || len(in.Items) > 20 {
		fail(w, 400, "batch_limit", "每批需1到20个明确身份的候选")
		return
	}
	out := []map[string]any{}
	for _, item := range in.Items {
		asset, err := e.Confirm(r.Context(), item.ID, item.Confirmation)
		message := ""
		if err != nil {
			message = "确认失败，请单独检查媒体身份、修订或文件冲突"
		}
		out = append(out, map[string]any{"id": item.ID, "assetId": asset, "succeeded": err == nil, "message": message})
	}
	write(w, 200, out)
}

func (a *API) correctAssetIdentity(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	var in library.Confirmation
	if !decode(w, r, &in) {
		return
	}
	if err := e.Correct(r.Context(), r.PathValue("id"), in); err != nil {
		fail(w, 409, "identity_correction_failed", err.Error())
		return
	}
	write(w, 200, map[string]bool{"corrected": true})
}
func (a *API) retryArtifactRepair(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	j := library.Journal{DB: a.Deps.DB, Vault: a.Deps.Integrations.Repository.Vault}
	if err := j.Apply(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 409, "artifact_repair_failed", "输出仍有冲突或权限问题，不会覆盖手工内容")
		return
	}
	if err := a.library().FinalizeRecovered(r.Context()); err != nil {
		fail(w, 503, "artifact_finalize_failed", "文件已恢复，状态确认待重试")
		return
	}
	write(w, 200, map[string]bool{"repaired": true})
}

func (a *API) previewAssetIdentity(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	var in library.Confirmation
	if !decode(w, r, &in) {
		return
	}
	result, err := e.PreviewIdentityMigration(r.Context(), r.PathValue("id"), in)
	if err != nil {
		fail(w, 409, "identity_preview_failed", err.Error())
		return
	}
	write(w, 200, result)
}
