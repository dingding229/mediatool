package retry

import "time"

// Backoff never decides whether an external effect is safe to retry.
func Backoff(attempt int, suggested time.Duration) time.Duration {
	if suggested > 0 {
		return min(suggested, 24*time.Hour)
	}
	delays := [...]time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour, 24 * time.Hour}
	return delays[min(max(attempt, 1)-1, len(delays)-1)]
}
