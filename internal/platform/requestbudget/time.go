package requestbudget

import "time"

// Earlier preserves zero-time semantics: a zero deadline sorts before real time.
func Earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
