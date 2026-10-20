package v2

import (
	"context"
	"testing"
)

func TestCapabilityClassificationKeepsUnsupportedAuthAndFailuresDistinct(t *testing.T) {
	in := Instance{Desired: Enabled, State: Running, Availability: Available}
	if state, _, _ := capabilityState(in, false, true, true, ""); state != "unsupported" {
		t.Fatal(state)
	}
	if state, _, _ := capabilityState(in, true, false, true, ""); state != "authorization_required" {
		t.Fatal(state)
	}
	if state, _, _ := capabilityState(in, true, true, true, "unknown"); state != "runtime_failed" {
		t.Fatal(state)
	}
	if state, _, _ := capabilityState(in, true, true, true, ""); state != "ready" {
		t.Fatal(state)
	}
	s, l, _ := snapshotFixture(t)
	before := countRetention(t, s, `SELECT count(*) FROM plugin_v2_runs`)
	capabilities, e := s.Capabilities(context.Background(), l.ID)
	if e != nil {
		t.Fatal(e)
	}
	ready, starting := false, false
	for _, c := range capabilities {
		ready = ready || c.State == "ready"
		starting = starting || c.State == "starting"
	}
	if !ready || !starting {
		t.Fatal(capabilities)
	}
	if after := countRetention(t, s, `SELECT count(*) FROM plugin_v2_runs`); after != before {
		t.Fatal("metadata caused execution")
	}
	execRetention(t, s, `DELETE FROM plugin_v2_grants WHERE scope='services.register'`)
	capabilities, e = s.Capabilities(context.Background(), l.ID)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, c := range capabilities {
		if c.Kind == "service" {
			found = true
			if c.State != "authorization_required" {
				t.Fatal(c)
			}
		}
	}
	if !found {
		t.Fatal("service declaration lost while unauthorized")
	}
}
