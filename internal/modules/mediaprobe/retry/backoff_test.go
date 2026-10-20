package retry

import (
	"testing"
	"time"
)

func TestExactMediaRetryIntervals(t *testing.T) {
	want := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour, 24 * time.Hour}
	for n, w := range want {
		if Backoff(n+1, 0) != w {
			t.Fatal(n)
		}
	}
	if Backoff(-1, 0) != time.Minute || Backoff(999, 0) != 24*time.Hour || Backoff(1, 48*time.Hour) != 24*time.Hour || Backoff(999, 3*time.Second) != 3*time.Second {
		t.Fatal("retry policy drift")
	}
}
