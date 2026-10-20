package requestbudget

import (
	"testing"
	"time"
)

func TestEarlierPreservesZeroAndEqualDeadline(t *testing.T) {
	a := time.Now()
	b := a.Add(time.Second)
	if Earlier(a, b) != a || Earlier(b, a) != a || Earlier(time.Time{}, a) != (time.Time{}) || Earlier(a, a) != a {
		t.Fatal("deadline drift")
	}
}
