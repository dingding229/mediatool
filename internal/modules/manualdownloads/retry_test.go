package manualdownloads

import "testing"

func TestRetryEligibilityKeepsUncertainReadOnly(t *testing.T) {
	if err := retryEligibility("uncertain", true, true, true); err == nil {
		t.Fatal("uncertain outcome was allowed to retry without reconciliation")
	}
	if err := retryEligibility("blocked", false, true, false); err != nil {
		t.Fatal(err)
	}
	if err := retryEligibility("absent_confirmed", false, true, false); err == nil {
		t.Fatal("confirmed absence accepted without duplicate-risk acknowledgement")
	}
	if err := retryEligibility("absent_confirmed", false, true, true); err != nil {
		t.Fatal(err)
	}
}

func TestRetryEligibilityValidatesAllStatesAndConsent(t *testing.T) {
	for _, tc := range []struct {
		state                             string
		attempted, confirm, risk, allowed bool
	}{
		{"blocked", false, true, false, true}, {"failed", false, true, false, true}, {"blocked", true, true, true, false}, {"failed", true, true, true, false},
		{"uncertain", true, true, true, false}, {"submitting", true, true, true, false}, {"downloading", true, true, true, false}, {"pending", false, true, false, false}, {"completed", true, true, true, false},
		{"blocked", false, false, false, false}, {"absent_confirmed", true, true, false, false}, {"absent_confirmed", true, true, true, true},
	} {
		if got := retryEligibility(tc.state, tc.attempted, tc.confirm, tc.risk); (got == nil) != tc.allowed {
			t.Fatalf("%+v: %v", tc, got)
		}
	}
}
