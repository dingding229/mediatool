package metadata

import (
	"reflect"
	"testing"
)

func TestEpisodeTagRoundTripPreservesOnlyDeclaredCoverage(t *testing.T) {
	for _, tc := range []struct {
		season int
		eps    []int
		want   string
	}{{1, []int{1}, "S01E01"}, {2, []int{3, 1, 2}, "S02E01-E03"}, {0, []int{3, 1}, "S00E01E03"}, {123, []int{1000, 1002}, "S123E1000E1002"}} {
		tag, err := EpisodeTag(tc.season, tc.eps)
		if err != nil || tag != tc.want {
			t.Fatal(tag, err)
		}
		season, got, ok := Episodes(tag + ".strm")
		want := tc.eps
		if len(want) == 3 {
			want = []int{1, 2, 3}
		} else if len(want) == 2 && want[0] == 3 {
			want = []int{1, 3}
		}
		if !ok || season != tc.season || !reflect.DeepEqual(got, want) {
			t.Fatal("coverage expanded or dropped", tag, got, want)
		}
	}
	if _, err := EpisodeTag(1, []int{1, 1}); err == nil {
		t.Fatal("duplicate coverage accepted")
	}
}
