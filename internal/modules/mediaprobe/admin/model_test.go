package admin

import "testing"

func TestBoundedKindsStatesAndPagination(t *testing.T) {
	for _, kind := range []string{"probe", "still"} {
		for _, state := range append(States(kind), "") {
			for _, size := range []int{1, 50, 200} {
				f := Filter{Kind: kind, State: state, Page: 1, PageSize: size}
				if !f.Valid() {
					t.Fatal(f)
				}
			}
		}
	}
	for _, f := range []Filter{{Kind: "probe", State: "grabbing"}, {Kind: "still", State: "probing"}, {Kind: "probe", State: "skipped"}, {Kind: "sql"}, {Kind: "probe", PageSize: 201}, {Kind: "still", Page: -1}, {Kind: "probe", Page: 100001, PageSize: 200}, {Kind: "probe", State: "' OR true"}} {
		if f.Valid() {
			t.Fatal("unbounded/invalid", f)
		}
	}
	if MaxRetry != 500 {
		t.Fatal("bulk limit changed")
	}
}
