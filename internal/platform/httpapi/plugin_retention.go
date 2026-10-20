package httpapi

import "net/http"

func (a *API) pluginJournalCapacity(w http.ResponseWriter, r *http.Request) {
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := s.JournalCapacity(r.Context())
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, http.StatusOK, out)
}
