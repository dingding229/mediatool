package ops

import (
	"strings"
	"testing"
)

func TestClassificationRepairRequiresBoundedExplicitRootIdentities(t *testing.T) {
	got, e := classificationTargets("movie:700,series:701,movie:702")
	if e != nil || len(got) != 3 {
		t.Fatal(got, e)
	}
	for _, raw := range []string{"", "*", "movie:0", "movie:-1", "movie:01", "movie:1,movie:1", "season:1", "episode:1", "https://upstream.invalid", "movie:1:other", strings.Repeat("movie:1,", 20) + "movie:21"} {
		if _, e := classificationTargets(raw); e == nil {
			t.Fatal("unsafe scope accepted", raw)
		}
	}
}
