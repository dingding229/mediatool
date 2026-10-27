package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
)

// Only the standard sizes and the endpoint's existing default are accepted.
func paginationSize(r *http.Request, key string, initial int) (int, error) {
	values, present := r.URL.Query()[key]
	if !present {
		return initial, nil
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("invalid page size")
	}
	n, err := strconv.Atoi(values[0])
	if err != nil || (n != initial && n != 10 && n != 25 && n != 50 && n != 100) {
		return 0, fmt.Errorf("invalid page size")
	}
	return n, nil
}
