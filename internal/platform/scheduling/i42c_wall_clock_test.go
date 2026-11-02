package scheduling

import (
	"testing"
	"time"
)

func TestI42cDailyBackupUsesShanghaiWallClock(t *testing.T) {
	instant := time.Date(2026, 11, 2, 6, 0, 0, 0, time.UTC)
	for _, zone := range []*time.Location{time.UTC, time.FixedZone("browser", -7*3600)} {
		p, e := Preview("0 2 * * *", "backup", instant.In(zone))
		if e != nil || len(p.Next) == 0 {
			t.Fatal(p, e)
		}
		scheduled, err := Next("0 2 * * *", instant.In(zone), time.Hour)
		if err != nil || !scheduled.Equal(p.Next[0]) {
			t.Fatal("preview and actual scheduler disagree", scheduled, p.Next[0], err)
		}
		at := p.Next[0].In(location)
		if at.Hour() != 2 || at.Minute() != 0 || at.Day() != 3 {
			t.Fatal(at)
		}
	}
}
