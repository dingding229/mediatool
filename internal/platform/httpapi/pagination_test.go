package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestSharedPaginationPreservesDistinctProfiles(t *testing.T) {
	for _, tt := range []struct {
		raw    string
		strict bool
		want   int
		fail   bool
	}{{"page=2", false, 2, false}, {"page=2&page=3", false, 2, false}, {"page=", false, 1, false}, {"page=", true, 0, true}, {"page=2&page=3", true, 0, true}, {"page=-1", true, 0, true}, {"page=201", true, 0, true}, {"", true, 1, false}} {
		r := httptest.NewRequest("GET", "/?"+tt.raw, nil)
		v, e := queryInt(r.URL.Query(), "page", 1, 1, 200, tt.strict)
		if (e != nil) != tt.fail || (!tt.fail && v != tt.want) {
			t.Fatal(tt, v, e)
		}
	}
}
