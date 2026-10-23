package buildinfo

import (
	"regexp"
	"strconv"
	"strings"
)

var stable = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func StableVersion(s string) bool {
	if !stable.MatchString(s) {
		return false
	}
	for _, n := range strings.Split(s, ".") {
		if _, e := strconv.ParseUint(n, 10, 64); e != nil {
			return false
		}
	}
	return true
}

// Compare accepts stable versions and our explicitly marked development builds.
func Compare(a, b string) (int, bool) {
	parse := func(v string) ([3]uint64, bool, bool) {
		var out [3]uint64
		parts := strings.SplitN(v, "-dev+", 2)
		if len(parts) == 2 && parts[1] == "" || !StableVersion(parts[0]) {
			return out, false, false
		}
		for i, n := range strings.Split(parts[0], ".") {
			out[i], _ = strconv.ParseUint(n, 10, 64)
		}
		return out, len(parts) == 2, true
	}
	x, xd, ok := parse(a)
	y, yd, bok := parse(b)
	if !ok || !bok {
		return 0, false
	}
	for i := range x {
		if x[i] < y[i] {
			return -1, true
		}
		if x[i] > y[i] {
			return 1, true
		}
	}
	if xd && !yd {
		return -1, true
	}
	if !xd && yd {
		return 1, true
	}
	return 0, true
}
