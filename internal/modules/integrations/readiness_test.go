package integrations

import (
	"testing"
	"time"
)

func TestReadinessDoesNotAutoEnableOrPromisePlayback(t *testing.T) {
	now := time.Now()
	checked := now.Add(-time.Minute)
	r := Record{Kind: "open115", LastCheckState: "ready", LastCheckedAt: &checked, UpdatedAt: now.Add(-time.Hour)}
	if f := ConnectionReadiness(r, now); f.State != "disabled" {
		t.Fatal(f)
	}
	r.SecretFields = []string{"accessToken", "refreshToken"}
	if f := ConnectionReadiness(r, now); f.NextAction != "enable_connection" {
		t.Fatal(f)
	}
	r.Enabled = true
	if f := ConnectionReadiness(r, now); f.State != "ready" {
		t.Fatal(f)
	}
	if f := ConnectionReadiness(r, now.Add(48*time.Hour)); f.State != "unavailable" {
		t.Fatal(f)
	}
	r.LastCheckState = "failed"
	if f := ConnectionReadiness(r, now); f.State != "unavailable" {
		t.Fatal(f)
	}
}

func TestHealthReadinessNeverReturnsLegacyOrSecretMessage(t *testing.T) {
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	for _, kind := range []string{"open115", "clouddrive2", "qbittorrent"} {
		r := Record{ID: "fixture", Kind: kind, Enabled: true, LastCheckState: "unchecked", LastCheckMessage: "https://user:secret@host?token=PRIVATE"}
		p := ConnectionReadiness(r, now)
		if p.State != "unavailable" || p.LastCheckedAt != nil || p.Message != "尚无有效检测记录，已安排检测" {
			t.Fatal(p)
		}
		r.LastCheckedAt = &old
		r.LastCheckState = "unavailable"
		p = ConnectionReadiness(r, now)
		if p.Message != safeHealthMessage("") {
			t.Fatal("unsafe old message", p)
		}
		r.LastCheckState = "ready"
		p = ConnectionReadiness(r, now)
		if p.State != "unavailable" || p.Message != "检测记录已过期，已安排检测" {
			t.Fatal(p)
		}
		r.Enabled = false
		p = ConnectionReadiness(r, now)
		if p.State != "disabled" || p.Message != "已停用" {
			t.Fatal(p)
		}
	}
}

func TestNonHeartbeatReadinessRetainsActualCheckTime(t *testing.T) {
	now := time.Now()
	checked := now.Add(-time.Hour)
	for _, test := range []struct {
		name, state             string
		enabled, missing, stale bool
		want                    string
	}{
		{"ready", "ready", true, false, false, "ready"}, {"missing", "ready", true, true, false, "check_required"},
		{"failed", "failed", true, false, false, "check_failed"}, {"expired", "ready", true, false, true, "check_required"},
		{"disabled", "ready", false, false, false, "disabled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			at := checked
			if test.stale {
				at = now.Add(-48 * time.Hour)
			}
			r := Record{Kind: "tmdb", Enabled: test.enabled, LastCheckState: test.state, LastCheckedAt: &at, UpdatedAt: now.Add(-72 * time.Hour)}
			if test.missing {
				r.LastCheckedAt = nil
			}
			p := ConnectionReadiness(r, now)
			if p.State != test.want {
				t.Fatal(p)
			}
			if r.LastCheckedAt == nil {
				if p.LastCheckedAt != nil {
					t.Fatal("invented check time")
				}
			} else if p.LastCheckedAt == nil || !p.LastCheckedAt.Equal(*r.LastCheckedAt) {
				t.Fatal("actual check timestamp lost", p)
			}
		})
	}
}
