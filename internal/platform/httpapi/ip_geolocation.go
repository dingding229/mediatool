package httpapi

import (
	"media-tool/internal/modules/ipgeo"
	"media-tool/internal/platform/clientip"
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
			out = append(out, ipgeo.Location{IP: clientip.Sanitize(v)})
		}
	}
	write(w, 200, out)
}
func (a *API) ipGeoSettings(w http.ResponseWriter, r *http.Request) {
	if a.Deps.IPGeo == nil {
		write(w, 200, ipgeo.Status{Attribution: ipgeo.Attribution})
		return
	}
	write(w, 200, a.Deps.IPGeo.Status(r.Context()))
}
func (a *API) saveIPGeoSettings(w http.ResponseWriter, r *http.Request) {
	var in ipgeo.Credentials
	if !decode(w, r, &in) {
		return
	}
	if a.Deps.IPGeo == nil || a.Deps.IPGeo.Save(r.Context(), in) != nil {
		fail(w, 400, "invalid_ip_geo_settings", "请检查下载授权 key 和 Base64 解密密钥；旧库不会被替换")
		return
	}
	write(w, 200, a.Deps.IPGeo.Status(r.Context()))
}
