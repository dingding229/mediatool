package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestI1PaginationSizeWhitelist(t *testing.T) {
	for _, initial := range []int{20, 30, 50} {
		for _, value := range []string{"10", "25", "50", "100"} {
			n, e := paginationSize(httptest.NewRequest("GET", "/?pageSize="+value, nil), "pageSize", initial)
			if e != nil || n == 0 {
				t.Fatal(value, e)
			}
		}
		n, e := paginationSize(httptest.NewRequest("GET", "/", nil), "pageSize", initial)
		if e != nil || n != initial {
			t.Fatal(n, e)
		}
		for _, value := range []string{"", "0", "-1", "11", "200", "no", "10.5", "10&pageSize=25"} {
			if _, e := paginationSize(httptest.NewRequest("GET", "/?pageSize="+value, nil), "pageSize", initial); e == nil {
				t.Fatal("accepted", value)
			}
		}
	}
}
