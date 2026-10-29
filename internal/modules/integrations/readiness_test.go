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
