package sitemetrics

import (
	"testing"
)

func TestT9LastDueSlotThenNullSkipsWithoutErrors(t *testing.T) {
	ctx, e, _ := fixture(t)
	if _, err := e.DB.Exec(ctx, `UPDATE site_metric_schedule SET enabled=true,cron_expression='0 0 0 1 1 * 2020',next_run_at=now()-interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	if err := e.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	s, err := e.GetSchedule(ctx)
	if err != nil || s.NextRunAt != nil || s.LastDispatchedAt == nil || !s.Enabled {
		t.Fatal(s, err)
	}
	var before, after int
	if err = e.DB.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err = e.Schedule(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err = e.DB.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&after); err != nil || before != after {
		t.Fatal(before, after, err)
	}
	s.Cron = "0 0 0 1 1 * 2199"
	s, err = e.SaveSchedule(ctx, s)
	if err != nil || s.NextRunAt == nil || s.NextRunAt.Year() != 2199 {
		t.Fatal(s, err)
	}
}
