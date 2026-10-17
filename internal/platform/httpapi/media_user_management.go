package httpapi

import (
	"net/http"
	"strconv"
)

// The same bounded pagination contract is used by devices/history/sessions.
func managementPagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	start, limit := 0, 50
	for _, field := range []struct {
		name  string
		value *int
	}{{"startIndex", &start}, {"limit", &limit}} {
		if values, ok := r.URL.Query()[field.name]; ok {
			if len(values) != 1 {
				fail(w, 400, "invalid_management_page", "页码或条数无效")
				return 0, 0, false
			}
			n, err := strconv.Atoi(values[0])
			if err != nil {
				fail(w, 400, "invalid_management_page", "页码或条数无效")
				return 0, 0, false
			}
			*field.value = n
		}
	}
	if start < 0 || start > 1000000 || limit < 1 || limit > 200 {
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
