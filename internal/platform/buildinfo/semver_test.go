package buildinfo

import "testing"

func TestStableVersionsAndDevelopmentOrdering(t *testing.T) {
	for _, v := range []string{"0.28.0", "1.2.3", "0.0.0"} {
		if !StableVersion(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"v0.28.0", "01.2.3", "1.2", "1.2.3-rc.1", "1.2.3+data", "999999999999999999999999.0.0", "1.2.3\n"} {
		if StableVersion(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []struct {
		a, b string
		n    int
	}{{"0.28.0-dev+abcdef", "0.28.0", -1}, {"0.28.0", "0.28.0", 0}, {"0.29.0-dev+abcdef", "0.28.0", 1}, {"0.9.0", "0.28.0", -1}} {
		n, ok := Compare(v.a, v.b)
		if !ok || n != v.n {
			t.Fatal(v, n, ok)
		}
	}
	if _, ok := Compare("invalid", "0.28.0"); ok {
		t.Fatal("invalid current accepted")
	}
}
