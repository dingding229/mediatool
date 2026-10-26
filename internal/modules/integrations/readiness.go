package integrations

import (
	"slices"
	"time"
)

// A fresh saved probe is a configuration observation, not real player/CDN proof.
type Readiness struct {
	State      string `json:"state"`
	NextAction string `json:"nextAction"`
	Reason     string `json:"reason"`
}

func ConnectionReadiness(r Record, now time.Time) Readiness {
	if r.Kind == "open115" && (!slices.Contains(r.SecretFields, "accessToken") || !slices.Contains(r.SecretFields, "refreshToken")) {
		return Readiness{"authorization_required", "authorize_115", "official_grant_missing"}
	}

	if !r.Enabled {
		return Readiness{"disabled", "enable_connection", "administrator_enable_required"}
	}
	if r.LastCheckedAt == nil || r.LastCheckState == "unchecked" || r.LastCheckedAt.Before(r.UpdatedAt) || r.LastCheckedAt.Add(24*time.Hour).Before(now) {
		return Readiness{"check_required", "test_connection", "probe_missing_or_stale"}
	}
	if r.LastCheckState != "ready" {
		return Readiness{"check_failed", "inspect_connection", "last_probe_failed"}
	}
	return Readiness{"ready", "none", "fresh_saved_probe"}
}
