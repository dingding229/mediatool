package scheduling

import (
	"testing"
	"time"
)

func TestCronCalendarTimezoneAndValidation(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 1, 0, 0, time.UTC)
	next, err := Next("0 9 * * *", now, time.Minute)
	if err != nil || !next.Equal(time.Date(2026, 10, 13, 1, 0, 0, 0, time.UTC)) {
		t.Fatal(next, err)
	}
	for _, expr := range []string{"@every 1h", "* * * * *", "0 0 31 2 *", "CRON_TZ=UTC 0 * * * *", "invalid"} {
		if _, err = Next(expr, now, 5*time.Minute); err == nil {
			t.Fatal(expr)
		}
	}
	next, _ = Next("*/30 * * * *", time.Date(2026, 10, 13, 1, 29, 59, 0, time.UTC), time.Minute)
	if next.Minute() != 30 {
		t.Fatal(next)
	}
}
