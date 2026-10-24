package httpapi

import (
	"media-tool/internal/modules/ipgeo"
	"net/http"
	"strings"
)

func (a *API) ipLocations(w http.ResponseWriter, r *http.Request) {
	values := strings.Split(r.URL.Query().Get("ips"), ",")
	if len(values) > 50 || len(r.URL.RawQuery) > 4096 {
		fail(w, 400, "invalid_ip_query", "每次最多查询50个 IP")
		return
	}
	out := []ipgeo.Location{}
	for _, v := range values {
		if a.Deps.IPGeo != nil {
			out = append(out, a.Deps.IPGeo.Lookup(v))
		} else {
			out = append(out, (*ipgeo.Service)(nil).Lookup(v))
		}
	}
	write(w, 200, out)
}
func (a *API) ipGeoSettings(w http.ResponseWriter, r *http.Request) {
	if a.Deps.IPGeo == nil {
		write(w, 200, (*ipgeo.Service)(nil).Status(r.Context()))
		return
	}
	write(w, 200, a.Deps.IPGeo.Status(r.Context()))
}
