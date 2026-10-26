package integrations

import (
	"testing"
	"time"
)

func TestReadinessDoesNotAutoEnableOrPromisePlayback(t *testing.T) {
	now := time.Now()
	checked := now.Add(-time.Minute)
	r := Record{Kind: "open115", LastCheckState: "ready", LastCheckedAt: &checked, UpdatedAt: now.Add(-time.Hour)}
	if f := ConnectionReadiness(r, now); f.State != "authorization_required" {
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
	if f := ConnectionReadiness(r, now.Add(48*time.Hour)); f.State != "check_required" {
		t.Fatal(f)
	}
	r.LastCheckState = "failed"
	if f := ConnectionReadiness(r, now); f.State != "check_failed" {
		t.Fatal(f)
	}
}
