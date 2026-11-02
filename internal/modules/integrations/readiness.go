package integrations

import (
	"time"
)

// A fresh saved probe is a configuration observation, not real player/CDN proof.
type Readiness struct {
	State         string     `json:"state"`
	NextAction    string     `json:"nextAction"`
	Reason        string     `json:"reason"`
	ConnectionID  string     `json:"connectionId,omitempty"`
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty"`
	Message       string     `json:"message,omitempty"`
}

func ConnectionReadiness(r Record, now time.Time) Readiness {
	if heartbeatKind(r.Kind) {
		freshFor := 90 * time.Second
		if r.Kind == "open115" {
			freshFor = 80 * time.Minute
		}
		p := Readiness{State: "unavailable", NextAction: "test_connection", Reason: "last_probe_failed", ConnectionID: r.ID, LastCheckedAt: r.LastCheckedAt, Message: safeHealthMessage(r.LastCheckMessage)}
		switch {
		case !r.Enabled:
			p.State, p.NextAction, p.Reason, p.Message = "disabled", "enable_connection", "administrator_enable_required", "已停用"
		case r.LastCheckedAt == nil || r.LastCheckedAt.Before(r.UpdatedAt):
			p.Reason, p.Message = "probe_missing_or_stale", "尚无有效检测记录，已安排检测"
			p.LastCheckedAt = nil
		case r.LastCheckState == "ready" && r.LastCheckedAt.Add(freshFor).After(now):
			p.State, p.NextAction, p.Reason, p.Message = "ready", "none", "fresh_saved_probe", "连接成功"
		}
		if r.Enabled && r.LastCheckState == "ready" && r.LastCheckedAt != nil && r.LastCheckedAt.Add(freshFor).Before(now) {
			p.Reason, p.Message = "probe_missing_or_stale", "检测记录已过期，已安排检测"
		}
		if p.State == "unavailable" && p.Message == reauthorizeMessage {
			p.NextAction, p.Reason = "authorize_115", "official_grant_missing"
		}
		return p
	}

	p := Readiness{LastCheckedAt: r.LastCheckedAt}
	if !r.Enabled {
		p.State, p.NextAction, p.Reason = "disabled", "enable_connection", "administrator_enable_required"
	} else if r.LastCheckedAt == nil || r.LastCheckState == "unchecked" || r.LastCheckedAt.Before(r.UpdatedAt) || r.LastCheckedAt.Add(24*time.Hour).Before(now) {
		p.State, p.NextAction, p.Reason = "check_required", "test_connection", "probe_missing_or_stale"
	} else if r.LastCheckState != "ready" {
		p.State, p.NextAction, p.Reason = "check_failed", "inspect_connection", "last_probe_failed"
	} else {
		p.State, p.NextAction, p.Reason = "ready", "none", "fresh_saved_probe"
	}
	return p
}
