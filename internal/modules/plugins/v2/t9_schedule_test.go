package v2

import (
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"testing"
	"time"
)

func TestT9CronServiceExhaustionDoesNotDisableConfig(t *testing.T) {
	d := contracts.Descriptor{Trigger: "cron", Cron: "0 0 0 1 1 * 2020", ScheduleSetting: "active"}
	next, enabled, err := nextService(d, json.RawMessage(`{"active":true}`), time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC))
	if err != nil || next != nil || !enabled {
		t.Fatal(next, enabled, err)
	}
	d.Cron = "0 0 0 1 1 * 2199"
	next, enabled, err = nextService(d, json.RawMessage(`{"active":true}`), time.Now())
	if err != nil || next == nil || next.Year() != 2199 || !enabled {
		t.Fatal(next, enabled, err)
	}
}

func TestT9ExplicitCronConfigCannotBypassPreviewWhileDisabled(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	for _, raw := range []string{`{}`, `{"cron":"0 8 * * *"}`, `{"cron":"0 0 8 * * *"}`, `{"cron":"0 0 8 * * * 2199"}`} {
		if err := validateCronConfig("site-checkin", json.RawMessage(raw), now); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, expr := range []string{"61 * * * *", "*/15 * * * * *", "0 0 0 1 1 * 2020", "0 0 0 * * * 2200"} {
		raw, _ := json.Marshal(map[string]any{"active": false, "cron": expr})
		if validateCronConfig("site-checkin", raw, now) == nil {
			t.Fatal(expr)
		}
	}
	// Unmarked plugin configuration remains opaque and is not guessed by name.
	if err := validateCronConfig("hello-go", json.RawMessage(`{"cron":"not a scheduling field"}`), now); err != nil {
		t.Fatal(err)
	}
}
