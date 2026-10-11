package requestbudget

import (
	"testing"
	"time"
)

func TestResolutionAndWriteBudgetsRemainFinite(t *testing.T) {
	if EffectiveResolve(0) != 30*time.Second || EffectiveResolve(-time.Second) != 30*time.Second || EffectiveResolve(2*time.Minute) != time.Minute {
		t.Fatal("unsafe effective resolution budget")
	}
	if FixedSTRMWrite(time.Minute) != 77*time.Second || InspectWrite(time.Minute) != 82*time.Second {
		t.Fatal("response budget is unbounded or omits bounded validation")
	}
	if FixedSTRMWrite(0) <= 30*time.Second {
		t.Fatal("management deadline would truncate resolution plus validation")
	}
}
