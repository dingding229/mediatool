package httpapi

import (
	"context"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/clientip"
	"net/http"
	"time"
)

// Missing/unavailable trust configuration never enables forwarded headers.
func (a *API) observedAddress(r *http.Request) clientip.Address {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	prefs, err := (settings.Repository{DB: a.Deps.DB}).Get(ctx)
	evidence := ipEvidence{proxies: prefs.Value.TrustedProxyCIDRs, err: err}
	if err == nil {
		evidence.address = clientip.AdmissionAddress(r, evidence.proxies)
	}
	*r = *r.WithContext(context.WithValue(r.Context(), ipEvidenceKey{}, evidence))
	if err != nil {
		return clientip.ClientAddress(r, nil)
	}
	return clientip.ClientAddress(r, prefs.Value.TrustedProxyCIDRs)
}
