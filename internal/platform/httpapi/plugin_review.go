package httpapi

import (
	"net/http"
)

func (a *API) pluginReviewPlan(w http.ResponseWriter, r *http.Request) {
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := s.ReviewPlan(r.Context(), r.PathValue("id"))
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
