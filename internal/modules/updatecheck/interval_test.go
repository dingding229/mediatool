package updatecheck

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestConfigurableCheckWindow(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	for _, hours := range []int{1, 12, 168} {
		for _, offset := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
			v := State{Enabled: true, IntervalHours: hours, Result: Result{CheckedAt: now.Add(-time.Duration(hours)*time.Hour - offset)}}
			if got := shouldCheck(v, now); got != (offset >= 0) {
				t.Fatalf("hours=%d offset=%v got=%v", hours, offset, got)
			}
			v.Enabled = false
			if shouldCheck(v, now) {
				t.Fatal("disabled check ran")
			}
		}
	}
	for _, hours := range []int{-1, 169} {
		if shouldCheck(State{Enabled: true, IntervalHours: hours}, now) {
			t.Fatal("invalid check interval ran")
		}
	}
}
func TestUpdateSettingsValidationAndPersistence(t *testing.T) {
	for _, hours := range []int{0, -1, 169} {
		if _, e := (&Service{}).SetSettings(context.Background(), true, hours); !errors.Is(e, ErrInvalidSettings) {
			t.Fatal(hours, e)
		}
	}
	s := isolatedService(t)
	ctx := t.Context()
	if e := s.store(ctx, Result{Status: "no_release", CheckedAt: time.Now().UTC()}); e != nil {
		t.Fatal(e)
	}
	for _, hours := range []int{1, 12, 168} {
		v, e := s.SetSettings(ctx, false, hours)
		if e != nil || v.Enabled || v.IntervalHours != hours || v.Result.Status != "no_release" {
			t.Fatal(v, e)
		}
		if e = s.store(ctx, Result{Status: "unavailable", CheckedAt: time.Now().UTC()}); e != nil {
			t.Fatal(e)
		}
		v, e = s.Get(ctx)
		if e != nil || v.Enabled || v.IntervalHours != hours {
			t.Fatal("check overwrote settings", v, e)
		}
		if e = s.store(ctx, Result{Status: "no_release", CheckedAt: time.Now().UTC()}); e != nil {
			t.Fatal(e)
		}
	}
}
