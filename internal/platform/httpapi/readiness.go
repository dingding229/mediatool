package httpapi

import (
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"net/http"
	"path"
)

func (a *API) workflowReadiness(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	records, e := a.Deps.Integrations.Repository.List(r.Context())
	if e != nil {
		integrationError(w, e)
		return
	}
	out := map[string]integrations.Readiness{}
	for _, kind := range []string{"open115", "clouddrive2", "emby"} {
		current := integrations.Readiness{State: "not_configured", NextAction: "configure_" + kind, Reason: "connection_missing"}
		// An enabled connection wins over disabled drafts; never auto-enable one.
		for _, record := range records {
			if record.Kind == kind {
				current = record.Readiness
				if record.Enabled {
					break
				}
			}
		}
		out[kind] = current
	}
	out["strm"] = integrations.Readiness{State: "check_required", NextAction: "configure_strm", Reason: "resolver_configuration_invalid"}
	builder := a.builder(r.Context())
	if len(builder.AllowedRoots) > 0 {
		_, e = builder.Build(storage.FileRef{Path: path.Join(builder.AllowedRoots[0], "readiness-fixture.mkv")})
		if e == nil {
			out["strm"] = integrations.Readiness{State: "configured", NextAction: "verify_player_302", Reason: "stable_resolver_configured_not_player_verified"}
		}
	}
	write(w, 200, map[string]any{"scope": "configuration_and_last_probe", "delivery": "original_strm_302", "resolverPath": strm.Endpoint, "components": out})
}
