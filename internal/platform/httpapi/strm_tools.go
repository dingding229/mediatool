package httpapi

import (
	"media-tool/internal/modules/storage"
	"net/http"
)

func (a *API) batchSTRMPreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paths []string `json:"paths"`
	}
	if !decodeLimit(w, r, &in, 200<<10) {
		return
	}
	if len(in.Paths) < 1 || len(in.Paths) > 100 {
		fail(w, 400, "batch_limit", "每批1–100条路径")
		return
	}
	out := []map[string]any{}
	b := a.builder(r.Context())
	for _, p := range in.Paths {
		url, err := b.Build(storage.FileRef{Path: p})
		row := map[string]any{"path": p, "valid": err == nil}
		if err != nil {
			row["message"] = err.Error()
		} else {
			row["url"] = url
			row["content"] = url + "\n"
		}
		out = append(out, row)
	}
	write(w, 200, out)
}
func (a *API) previewSTRMRepair(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库和服务连接不可用")
		return
	}
	var in struct {
		Assets []string `json:"assetIds"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := e.PreviewSTRMRepair(r.Context(), in.Assets)
	if err != nil {
		fail(w, 409, "strm_repair_preview", err.Error())
		return
	}
	write(w, 200, out)
}
func (a *API) getSTRMRepair(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	out, err := e.STRMRepairBatch(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, "strm_repair_not_found", "修复批次不可用")
		return
	}
	write(w, 200, out)
}
func (a *API) approveSTRMRepair(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	var in struct {
		Digest  string `json:"digest"`
		Confirm bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := e.ApproveSTRMRepair(r.Context(), r.PathValue("id"), in.Digest, in.Confirm)
	if err != nil {
		fail(w, 409, "strm_repair_conflict", err.Error())
		return
	}
	write(w, 202, out)
}
func (a *API) retrySTRMRepair(w http.ResponseWriter, r *http.Request) {
	e := a.library()
	if e == nil {
		fail(w, 503, "library_unavailable", "数据库不可用")
		return
	}
	var in struct {
		Digest  string `json:"digest"`
		Confirm bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := e.RetrySTRMRepair(r.Context(), r.PathValue("id"), in.Digest, in.Confirm)
	if err != nil {
		fail(w, 409, "strm_repair_conflict", err.Error())
		return
	}
	write(w, 202, out)
}
func (a *API) registerSTRMTools(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/strm/repairs/{id}/retry", a.protected(a.retrySTRMRepair))
	mux.HandleFunc("POST /api/v1/strm/batch-preview", a.protected(a.batchSTRMPreview))
	mux.HandleFunc("POST /api/v1/strm/repairs/preview", a.protected(a.previewSTRMRepair))
	mux.HandleFunc("GET /api/v1/strm/repairs/{id}", a.protected(a.getSTRMRepair))
	mux.HandleFunc("POST /api/v1/strm/repairs/{id}/approve", a.protected(a.approveSTRMRepair))
}
