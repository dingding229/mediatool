package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestT9CronPreviewAuthWhitelistAndNoWrite(t *testing.T) {
	h, _ := testHandler(nil)
	for _, v := range []struct {
		body, key string
		status    int
	}{
		{`{"expression":"0 0 * * *","purpose":"generic"}`, "", 401},
		{`{"expression":"0 0 * * *","purpose":"generic"}`, cfg().SessionKey, 200},
		{`{"expression":"0 0 9 * * * 2199","purpose":"backup"}`, cfg().SessionKey, 200},
		{`{"expression":"0 0 * * *","purpose":"untrusted"}`, cfg().SessionKey, 400},
		{`{"expression":"* * * *","purpose":"generic"}`, cfg().SessionKey, 400},
		{`{"expression":"` + strings.Repeat("x", 129) + `","purpose":"generic"}`, cfg().SessionKey, 400},
		{`{"expression":"0 0 * * *",` + strings.Repeat(" ", 4096) + `"purpose":"generic"}`, cfg().SessionKey, 400},
		{`{"expression":"*/15 * * * * *","purpose":"site-metrics"}`, cfg().SessionKey, 400},
	} {
		w := invoke(h, "POST", "/api/v1/scheduling/cron/preview", v.body, v.key)
		if w.Code != v.status {
			t.Fatal(v.status, w.Code, w.Body.String())
		}
		if w.Code == 200 {
			var out struct {
				Data struct {
					Next     []string `json:"next"`
					Timezone string   `json:"timezone"`
				}
			}
			if json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Data.Next) == 0 || out.Data.Timezone != "Asia/Shanghai" {
				t.Fatal(w.Body.String())
			}
		}
	}
}
func TestT9FixedSchedulesDoNotLookExhausted(t *testing.T) {
	h, _ := testHandler(nil)
	w := invoke(h, "GET", "/api/v1/system/operations", "", cfg().SessionKey)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var out struct {
		Data struct {
			Schedules []struct {
				ID   string  `json:"id"`
				Next *string `json:"nextRunAt"`
			}
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Data.Schedules) == 0 {
		t.Fatal("missing core")
	}
	if out.Data.Schedules[0].ID != "core" || out.Data.Schedules[0].Next == nil {
		t.Fatal("core is not exhausted")
	}
}
