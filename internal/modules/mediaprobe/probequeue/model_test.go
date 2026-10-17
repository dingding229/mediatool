package probequeue

import (
	"encoding/json"
	"math"
	"media-tool/internal/modules/mediaprobe/linksource"
	"strings"
	"testing"
	"time"
)

func TestBackoffAndUpstreamPriority(t *testing.T) {
	for i, want := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour, 24 * time.Hour, 24 * time.Hour} {
		if got := Backoff(i+1, 0); got != want {
			t.Fatal(i, got, want)
		}
	}
	for _, i := range []int{math.MinInt, 0, -1} {
		if Backoff(i, 0) != time.Minute {
			t.Fatal("invalid attempt bounds")
		}
	}
	if Backoff(100, 10*time.Second) != 10*time.Second || Backoff(1, 48*time.Hour) != 24*time.Hour || Backoff(1, -time.Second) != time.Minute {
		t.Fatal("suggested interval contract")
	}
}
func TestLeaseBoundsAndWhitelistedResult(t *testing.T) {
	for _, tt := range []struct{ resolve, probe, want time.Duration }{{0, 0, 75 * time.Second}, {time.Minute, 2 * time.Minute, 195 * time.Second}, {10 * time.Second, 20 * time.Second, 45 * time.Second}} {
		got, err := LeaseDuration(tt.resolve, tt.probe)
		if err != nil || got != tt.want {
			t.Fatal(got, err)
		}
	}
	for _, bad := range []time.Duration{-time.Second, time.Hour} {
		if _, err := LeaseDuration(bad, 0); err == nil {
			t.Fatal("resolve bound")
		}
		if _, err := LeaseDuration(0, bad); err == nil {
			t.Fatal("probe bound")
		}
	}
	var r Result
	if err := json.Unmarshal([]byte(`{"durationTicks":42,"filename":"https://synthetic.example/?SECRET=never_persist","format":{"token":"private"}}`), &r); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(r)
	if err != nil || string(raw) != `{"durationTicks":42}` || strings.Contains(string(raw), "SECRET") {
		t.Fatal("typed result leaked raw data")
	}
}
func TestFailureWhitelist(t *testing.T) {
	classes := []linksource.Class{linksource.Permanent, linksource.Retryable, linksource.Manual}
	reasons := []string{string(linksource.NotExposable), string(linksource.FileMissing), string(linksource.Mismatch), string(linksource.Authorization), string(linksource.Limited), string(linksource.Timeout), string(linksource.Unavailable), string(linksource.UnsafeTarget), string(linksource.UAMismatch), string(linksource.Used), string(LeaseExpired), string(ProcessFailed), string(InvalidResult)}
	for _, class := range classes {
		for _, reason := range reasons {
			if !(Failure{Class: class, Reason: reason}).valid() {
				t.Fatal("constant rejected")
			}
		}
	}
	for _, f := range []Failure{{Class: "arbitrary", Reason: string(linksource.Timeout)}, {Class: linksource.Manual, Reason: "https://synthetic.example/?secret=bad"}, {}} {
		if f.valid() {
			t.Fatal("untrusted error persisted")
		}
	}

}
func TestInvalidRepositoryInputs(t *testing.T) {
	r := Repository{}
	if _, err := r.Enqueue(t.Context(), "invalid"); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Retry(t.Context(), "invalid"); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Claim(t.Context(), "invalid", time.Minute); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Succeed(t.Context(), Lease{}, Result{}); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Fail(t.Context(), Lease{}, Failure{}); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.Current(t.Context(), "invalid"); err != ErrInvalid {
		t.Fatal(err)
	}
	if _, err := r.RecoverExpired(t.Context()); err != ErrInvalid {
		t.Fatal(err)
	}
}
