package httpapi

import (
	"media-tool/internal/modules/acquisition"
	"media-tool/internal/modules/library"
	"net/http"
)

func (a *API) previewOutputResume(w http.ResponseWriter, r *http.Request) {
	if !library.ValidArchiveID(r.PathValue("id")) {
		fail(w, 400, "transfer_id_invalid", "传输ID格式无效")
		return
	}
	if !a.integrationReady(w) {
		return
	}
	e := acquisition.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}
	p, err := e.PreviewOutputResume(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 409, "output_preview_unavailable", err.Error())
		return
	}
	write(w, 200, p)
}
func (a *API) resumeOutputImport(w http.ResponseWriter, r *http.Request) {
	if !library.ValidArchiveID(r.PathValue("id")) {
		fail(w, 400, "transfer_id_invalid", "传输ID格式无效")
		return
	}
	if !a.integrationReady(w) {
		return
	}
	var in struct {
		Revision int64  `json:"controlRevision"`
		Digest   string `json:"planDigest"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Revision < 1 || len(in.Digest) != 64 {
		fail(w, 400, "output_preview_required", "需要当前修订及完整输出核对证明")
		return
	}
	e := acquisition.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}
	if err := e.ResumeOutputImport(r.Context(), r.PathValue("id"), in.Revision, in.Digest); err != nil {
		fail(w, 409, "output_resume_blocked", err.Error())
		return
	}
	write(w, 200, map[string]bool{"approved": true, "uploadRestarted": false})
}
