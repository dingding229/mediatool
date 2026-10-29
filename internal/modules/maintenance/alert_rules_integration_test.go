package maintenance

import (
	"context"
	"testing"
)

func TestAlertMetadataMatchesEvaluationAndTracksPolicy(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	p := Defaults()
	p.Consecutive = 1
	for generation := 0; generation < 2; generation++ {
		if generation == 1 {
			p.MemoryPercent = 75
			p.DiskFreeGB = 12
			p.LoadPerCPU = 1.5
			p.FailedJobs = 7
		}
		r, err := e.Policy(ctx)
		if err != nil {
			t.Fatal(err)
		}
		r.Value = p
		if _, err = e.SavePolicy(ctx, r); err != nil {
			t.Fatal(err)
		}
		boundary := map[string]any{}
		safe := map[string]any{}
		for _, rule := range alertRules(p) {
			boundary[rule.key] = rule.threshold
			safe[rule.key] = rule.threshold - 1
			if rule.below {
				safe[rule.key] = rule.threshold + 1
			}
		}
		if err = e.evaluate(ctx, p, boundary); err != nil {
			t.Fatal(err)
		}
		alerts, err := e.Alerts(ctx)
		if err != nil || len(alerts) != 6 {
			t.Fatal(alerts, err)
		}
		expected := map[string]struct {
			threshold        float64
			unit, comparison string
		}{
			"memory": {p.MemoryPercent, "percent", ">="}, "disk": {p.DiskFreeGB, "GiB", "<="},
			"load": {p.LoadPerCPU, "load_per_cpu", ">="}, "failures": {float64(p.FailedJobs), "jobs_per_10m", ">="},
			"workers": {OfflineQueueAlertThreshold, "queues", ">="}, "plugin_journal": {PluginJournalAlertThreshold, "entries", ">="},
		}
		for _, a := range alerts {
			want, ok := expected[a.Code]
			if !ok || a.Threshold == nil || *a.Threshold != want.threshold || a.Unit != want.unit || a.Comparison != want.comparison || a.State != "active" {
				t.Fatalf("generation %d: %+v, want %+v at active boundary", generation, a, want)
			}
		}
		if err = e.evaluate(ctx, p, safe); err != nil {
			t.Fatal(err)
		}
		alerts, err = e.Alerts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range alerts {
			if a.State != "normal" || a.Message != "指标已恢复或告警已关闭" {
				t.Fatalf("safe side changed: %+v", a)
			}
		}
	}
	// A new policy is reflected without needing the monitor to sample again.
	r, err := e.Policy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r.Value.MemoryPercent = 81
	if _, err = e.SavePolicy(ctx, r); err != nil {
		t.Fatal(err)
	}
	alerts, err := e.Alerts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range alerts {
		if a.Code == "memory" && (a.Threshold == nil || *a.Threshold != 81) {
			t.Fatal(a)
		}
	}
	if _, err = e.DB.Exec(ctx, `INSERT INTO ops_alerts(code,message)VALUES('future-rule','unknown original')`); err != nil {
		t.Fatal(err)
	}
	alerts, err = e.Alerts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range alerts {
		if a.Code == "future-rule" && (a.Threshold != nil || a.Unit != "" || a.Comparison != "" || a.Message != "unknown original") {
			t.Fatal(a)
		}
	}
}
