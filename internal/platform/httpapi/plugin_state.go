package httpapi

import (
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"net/http"
)

func (a *API) pluginStateList(w http.ResponseWriter, r *http.Request) {
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := s.StateSnapshots(r.Context(), r.PathValue("id"))
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
func (a *API) pluginStateOperation(w http.ResponseWriter, r *http.Request) {
	var q pluginv2.StateRequest
	if !decode(w, r, &q) {
		return
	}
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	account, e := a.sessionAccount(r)
	if e != nil {
		fail(w, 401, "unauthorized", "请登录管理员")
		return
	}
	out, e := s.StateOperation(r.Context(), account.ID, r.PathValue("id"), q)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 200, out)
}
