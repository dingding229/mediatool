// Package diagnosticpolicy keeps owner CLI probes away from this deployment's
// service endpoints without embedding any deployment address in source.
package diagnosticpolicy

import (
	"media-tool/internal/modules/playback"
	"net/url"
)

// New accepts configured public origins and the selected cloud endpoint. Only
// hostnames are retained; credentials, query values and paths never enter policy.
func New(origins ...string) playback.Policy {
	hosts := []string{"localhost", "server", "clouddrive2"}
	for _, raw := range origins {
		u, err := url.Parse(raw)
		if err == nil && u.Hostname() != "" {
			hosts = append(hosts, u.Hostname())
		}
	}
	return playback.Policy{BlockedHosts: hosts}
}
