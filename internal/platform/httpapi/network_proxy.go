package httpapi

import (
	"errors"
	"media-tool/internal/modules/networkproxy"
	"net/http"
)

func (a *API) networkProxy(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	repo := networkproxy.Repository{DB: a.Deps.DB, Vault: a.Deps.Integrations.Repository.Vault}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "GET" {
		out, e := repo.Get(r.Context())
		if e != nil {
			fail(w, 503, "proxy_unavailable", "代理设置读取失败")
			return
		}
		write(w, 200, out)
		return
	}
	var in networkproxy.Record
	if !decode(w, r, &in) {
		return
	}
	out, e := repo.Save(r.Context(), in)
	if e != nil {
		code := 400
		if errors.Is(e, networkproxy.ErrRevision) {
			code = 409
		}
		fail(w, code, "proxy_save_failed", e.Error())
		return
	}
	write(w, 200, out)
}
