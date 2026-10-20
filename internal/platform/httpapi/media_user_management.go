package httpapi

import (
	"net/http"
	"net/url"
)

// The same bounded pagination contract is used by devices/history/sessions.
func managementPagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		fail(w, 400, "invalid_management_page", "分页参数无效")
		return 0, 0, false
	}
	start, e := queryInt(q, "startIndex", 0, 0, 1000000)
	limit, le := queryInt(q, "limit", 50, 1, 200)
	if e != nil || le != nil {
		fail(w, 400, "invalid_management_page", "页码或条数无效")
		return 0, 0, false
	}
	return start, limit, true
}
func (a *API) mediaUserDevices(w http.ResponseWriter, r *http.Request) {
	start, limit, ok := managementPagination(w, r)
	if !ok {
		return
	}
	out, err := a.mediaUsers().Devices(r.Context(), r.PathValue("id"), start, limit)
	if err != nil {
		mediaError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) mediaUserHistory(w http.ResponseWriter, r *http.Request) {
	start, limit, ok := managementPagination(w, r)
	if !ok {
		return
	}
	out, err := a.mediaUsers().History(r.Context(), r.PathValue("id"), start, limit)
	if err != nil {
		mediaError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) revokeMediaDevice(w http.ResponseWriter, r *http.Request) {
	if err := a.mediaUsers().RevokeDevice(r.Context(), r.PathValue("id"), r.PathValue("deviceId")); err != nil {
		mediaError(w, err)
		return
	}
	write(w, http.StatusOK, struct {
		Revoked bool `json:"revoked"`
	}{true})
}
