package scheduling

import (
	"errors"
	"github.com/robfig/cron/v3"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestT9LegacySequencesAndCoreUnchanged(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 1, 0, time.UTC)
	for _, expr := range []string{"*/30 * * * *", "0 3 * * *", "0 9 1 * MON", "0 0 29 FEB *", "? 1 * * MON", "*/15 * * * * *", "5 */7 0-20 * JAN,MAR *"} {
		opts := cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow
		if len(strings.Fields(expr)) == 6 {
			opts |= cron.Second
		}
		old, e := cron.NewParser(opts).Parse("CRON_TZ=" + Timezone + " " + expr)
		if e != nil {
			t.Fatal(e)
		}
		current, e := (Parser{}).Parse(expr)
		if e != nil {
			t.Fatal(e)
		}
		a, b := now, now
		for i := 0; i < 200; i++ {
			a = old.Next(a)
			b = current.Next(b)
			if !a.Equal(b) {
				t.Fatal(expr, i, a, b)
			}
		}
	}
}
func TestT9FiveSixSevenYearAndExhaustion(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, location)
	for _, expr := range []string{"0 9 * * *", "0 0 9 * * *", "0 0 9 * * * 2026"} {
		out, e := Preview(expr, "subscription", now)
		if e != nil || len(out.Next) != 5 || out.Next[0].Hour() != 9 {
			t.Fatal(out, e)
		}
	}
	for _, expr := range []string{"0 0 0 1 1 * 2199", "0 0 0 29 2 * 2104", "0 0 0 1 1 * 2035,2040", "0 0 0 1 1 * 2030-2040/2"} {
		out, e := Preview(expr, "generic", now)
		if e != nil || len(out.Next) == 0 || out.Next[0].Year() <= 2026 {
			t.Fatal(expr, out, e)
		}
	}
	lower, e := Preview("0 0 0 1 1 * 1970", "generic", time.Date(1969, 12, 31, 0, 0, 0, 0, location))
	if e != nil || len(lower.Next) != 1 || lower.Next[0].Year() != 1970 {
		t.Fatal(lower, e)
	}
	padded := "0 0 * * *" + strings.Repeat(" ", MaxExpressionBytes-len("0 0 * * *"))
	if _, e = Preview(padded, "generic", now); e != nil {
		t.Fatal(e)
	}
	if _, e = Preview(padded+" ", "generic", now); e == nil {
		t.Fatal("length boundary")
	}
	expr := "0 0 0 1 1 * 2027"
	out, e := Preview(expr, "backup", now)
	if e != nil || len(out.Next) != 1 {
		t.Fatal(out, e)
	}
	end := time.Date(2027, 1, 1, 0, 0, 0, 0, location)
	next, e := NextOptional(expr, end, time.Hour)
	if e != nil || next != nil {
		t.Fatal(next, e)
	}
	if _, e = Next(expr, end, time.Hour); !errors.Is(e, ErrExhausted) {
		t.Fatal(e)
	}
	out, e = Preview(expr, "backup", end)
	if e != nil || !out.Exhausted || len(out.Next) != 0 {
		t.Fatal(out, e)
	}
}
func TestT9InvalidAndBudget(t *testing.T) {
	now := time.Now()
	for _, expr := range []string{"* * * *", "* * * * * * * *", "0 0 0 * * * 1969", "0 0 0 * * * 2200", "0 0 0 * * * 2030/0", "0 0 0 * * * 2040-2030", "0 0 0 * * * ?", "@daily", "CRON_TZ=UTC 0 0 * * *", strings.Repeat("*", 129), "0 0 25 * * *"} {
		if _, e := Preview(expr, "generic", now); e == nil {
			t.Fatal(expr)
		}
	}
	if _, e := Preview("0 0 * * *", "unknown", now); e == nil {
		t.Fatal("purpose")
	}
	if _, e := Preview("*/10 * * * * *", "site-metrics", now); e == nil {
		t.Fatal("minimum")
	}
	s, _ := parse("0 0 0 31 2 * 1970-2199")
	if _, e := s.next(now, &budget{deadline: now.Add(-time.Hour)}); !errors.Is(e, ErrBudget) {
		t.Fatal(e)
	}
	if _, e := s.next(now, &budget{steps: MaxSteps, deadline: now.Add(time.Hour)}); !errors.Is(e, ErrBudget) {
		t.Fatal(e)
	}
	start := time.Now()
	_, _ = Preview("0 0 0 31 2 * 1970-2199", "generic", now)
	if time.Since(start) > time.Second {
		t.Fatal("unbounded calculation")
	}
}
func TestT9CoreRecoveryAndNonOverlapChain(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	job := cron.FuncJob(func() { calls.Add(1); close(entered); <-release })
	wrapped := cron.NewChain(cron.Recover(cron.DefaultLogger), cron.SkipIfStillRunning(cron.DefaultLogger)).Then(job)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); wrapped.Run() }()
	<-entered
	wrapped.Run()
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	cron.Recover(cron.DefaultLogger)(cron.FuncJob(func() { panic("test recovery") })).Run()
}

func TestT9CoreActualLegacyParserAndUnifiedFixedClock(t *testing.T) {
	old, err := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse("*/15 * * * * *")
	if err != nil {
		t.Fatal(err)
	}
	current, err := (Parser{}).Parse(CoreReconcileCron)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := current.(*cron.SpecSchedule); !ok {
		t.Fatal("core must retain the original executor")
	}
	a := time.Date(2026, 12, 31, 23, 59, 59, 123456789, time.UTC)
	b := a
	for i := 0; i < 200; i++ {
		a = old.Next(a)
		b = current.Next(b)
		if a.Format(time.RFC3339Nano) != b.Format(time.RFC3339Nano) {
			t.Fatal(i, a, b)
		}
	}
}
