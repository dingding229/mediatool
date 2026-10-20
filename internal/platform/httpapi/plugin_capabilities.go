package httpapi

import "net/http"

func (a *API) pluginCapabilities(w http.ResponseWriter, r *http.Request) {
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := s.Capabilities(r.Context(), r.PathValue("id"))
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 200, out)
}
