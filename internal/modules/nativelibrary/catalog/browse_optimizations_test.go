package catalog

import "testing"

func TestIndexedNumericIDsPreserveReservedLibraries(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int64
	}{{"1", 1}, {"2", 2}, {"100000", 100000}, {"9223372036854775807", 9223372036854775807}, {"0", 0}, {"-1", 0}, {"01", 0}, {"+1", 0}, {" 1", 0}, {"9223372036854775808", 0}, {"movie:1", 0}, {"", 0}} {
		if got := numericID(tc.raw); got != tc.want {
			t.Fatalf("id %q=%d want%d", tc.raw, got, tc.want)
		}
	}
}
