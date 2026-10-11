package httpapi

import (
	"net/http"

	"media-tool/internal/modules/library"
)

func (a *API) listArchives(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "archives_unavailable", "数据库不可用")
		return
	}
	for _, name := range []string{"page", "pageSize"} {
		values := r.URL.Query()[name]
		if len(values) > 1 || len(values) == 1 && values[0] == "" {
			fail(w, 400, "archive_page_invalid", "分页参数不能为空或重复")
			return
		}
	}
	p, err := pagination(r)
	if err != nil {
		fail(w, 400, "archive_page_invalid", "分页参数无效")
		return
	}
	list, total, err := e.Archives(r.Context(), p.Limit, p.Offset)
	if err != nil {
		fail(w, 503, "archives_unavailable", "归档记录暂不可用")
		return
	}
	write(w, 200, map[string]any{"items": list, "total": total, "page": p.Page, "pageSize": p.Limit})
}
func (a *API) archivePolicy(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "archives_unavailable", "数据库不可用")
		return
	}
	p, err := e.ArchivePolicy(r.Context())
	if err != nil {
		fail(w, 503, "archives_unavailable", "归档策略不可用")
		return
	}
	write(w, 200, p)
}
func (a *API) saveArchivePolicy(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "archives_unavailable", "数据库不可用")
		return
	}
	var in library.ArchivePolicy
	if !decode(w, r, &in) {
		return
	}
	p, err := e.SaveArchivePolicy(r.Context(), in)
	if err != nil {
		fail(w, 409, "archive_policy_changed", err.Error())
		return
	}
	write(w, 200, p)
}
func (a *API) previewArchiveCleanup(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "archives_unavailable", "数据库不可用")
		return
	}
	var in struct {
		IDs []string `json:"archiveIds"`
	}
	if !decode(w, r, &in) {
		return
	}
	p, err := e.PreviewArchiveCleanup(r.Context(), in.IDs)
	if err != nil {
		fail(w, 409, "archive_preview_failed", err.Error())
		return
	}
	write(w, 200, p)
}
func (a *API) confirmArchiveCleanup(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "archives_unavailable", "数据库不可用")
		return
	}
	var in library.ArchiveCleanupRequest
	if !decode(w, r, &in) {
		return
	}
	p, err := e.ConfirmArchiveCleanup(r.Context(), in)
	if err != nil {
		fail(w, 409, "archive_confirmation_failed", err.Error())
		return
	}
	write(w, 200, p)
}
func (a *API) archiveCleanupResults(w http.ResponseWriter, r *http.Request) {
	if !library.ValidArchiveID(r.PathValue("id")) {
		fail(w, 400, "archive_request_invalid", "请求ID须为有效UUID")
		return
	}
	e := a.library()
	if e == nil {
		fail(w, 503, "archives_unavailable", "数据库不可用")
		return
	}
	list, err := e.ArchiveCleanupResults(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 503, "archive_cleanup_unavailable", "清理结果不可用")
		return
	}
	write(w, 200, list)
}
func (a *API) retryArchiveCleanup(w http.ResponseWriter, r *http.Request) {
	if !library.ValidArchiveID(r.PathValue("id")) {
		fail(w, 400, "archive_operation_invalid", "清理操作ID须为有效UUID")
		return
	}
	e := a.library()
	if e == nil {
		fail(w, 503, "archives_unavailable", "数据库不可用")
		return
	}
	var in struct{}
	if !decode(w, r, &in) {
		return
	}
	if err := e.ApplyArchiveCleanup(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 409, "archive_cleanup_conflict", "清理未完成：归档、当前输出、保留期或恢复状态可能改变；不会覆盖/删除手工内容，请核对后重试")
		return
	}
	write(w, 200, map[string]bool{"completed": true})
}
