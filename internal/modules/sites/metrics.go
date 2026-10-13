package sites

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
)

type Metrics struct {
	Name            string   `json:"username,omitempty"`
	Uploaded        *int64   `json:"uploaded"`
	Downloaded      *int64   `json:"downloaded"`
	Ratio           *float64 `json:"ratio"`
	Bonus           *float64 `json:"bonus"`
	Level           string   `json:"level,omitempty"`
	Seeding         *int64   `json:"seeding"`
	SeedingBytes    *int64   `json:"seedingBytes"`
	SeedingPages    int      `json:"seedingPages"`
	SeedingComplete bool     `json:"seedingComplete"`
	Partial         bool     `json:"partial"`
	Notes           []string `json:"notes"`
}

func metricNumber(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	s := strings.ReplaceAll(strings.TrimSpace(text(v)), ",", "")
	n, e := strconv.ParseFloat(s, 64)
	return n, e == nil && !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0
}
func metricBytes(v any) *int64 {
	n, ok := metricNumber(v)
	if !ok || n >= math.Exp2(63) || math.Trunc(n) != n {
		return nil
	}
	i := int64(n)
	return &i
}
func (c Client) readMetricFields(ctx context.Context, rule Map, keys []string, vars map[string]string) (Map, error) {
	if err := validateRequestExecution(rule); err != nil {
		return nil, err
	}
	raw, mode, err := c.call(ctx, rule, vars, nil)
	if err != nil {
		return nil, err
	}
	doc, err := parseDocument(raw, mode)
	if err != nil {
		return nil, err
	}
	out := Map{}
	for _, k := range keys {
		field, ok := object(rule["fields"])[k]
		if !ok {
			continue
		}
		v, err := mapFields(doc, Map{k: field}, nil, vars)
		if err == nil && !blank(v[k]) {
			out[k] = v[k]
		}
	}
	return out, nil
}
func (c Client) Metrics(ctx context.Context) (Metrics, error) {
	m := Metrics{Notes: []string{}}
	if err := c.Test(ctx); err != nil {
		return m, err
	}
	vars := c.vars("", 1)
	rule := c.Config.Requests["user_basic_info"]
	if c.Config.Schema == "mTorrent" {
		rule = c.Config.Requests["profile"]
	}
	values, err := c.readMetricFields(ctx, rule, []string{"id", "name", "username", "uploaded", "downloaded", "ratio", "bonus", "seedbonus", "level", "seeding", "seeding_size"}, vars)
	if err != nil {
		return m, err
	}
	m.Name = text(values["name"])
	if m.Name == "" {
		m.Name = text(values["username"])
	}
	if id := text(values["id"]); id != "" {
		vars["userId"] = id
	}
	m.Uploaded = metricBytes(values["uploaded"])
	m.Downloaded = metricBytes(values["downloaded"])
	if n, ok := metricNumber(values["ratio"]); ok {
		m.Ratio = &n
	}
	if n, ok := metricNumber(values["bonus"]); ok {
		m.Bonus = &n
	}
	m.Level = text(values["level"])
	m.Seeding = metricBytes(values["seeding"])
	m.SeedingBytes = metricBytes(values["seeding_size"])
	if details := c.Config.Requests["user_details"]; details != nil && vars["userId"] != "" {
		v, e := c.readMetricFields(ctx, details, []string{"level"}, vars)
		if e == nil {
			m.Level = text(v["level"])
		} else {
			m.Notes = append(m.Notes, "等级读取失败")
		}
	}
	if m.Seeding != nil && *m.Seeding == 0 && rule["seeding_zero_is_complete"] == true {
		zero := int64(0)
		m.SeedingBytes = &zero
		m.SeedingComplete = true
		m.SeedingPages = 1
	} else if seed := c.Config.Requests["seeding_statistics"]; seed != nil && vars["userId"] != "" {
		count, size, pages, complete, note := c.collectSeeding(ctx, seed, vars)
		m.SeedingPages = pages
		m.SeedingComplete = complete
		if complete {
			m.Seeding = &count
			m.SeedingBytes = &size
		} else {
			m.Notes = append(m.Notes, note)
		}
	}

	m.Partial = m.Uploaded == nil || m.Downloaded == nil || m.Ratio == nil || m.Bonus == nil || m.Level == "" || m.Seeding == nil || m.SeedingBytes == nil || len(m.Notes) > 0
	if m.Uploaded == nil && m.Downloaded == nil && m.Bonus == nil && m.Name == "" {
		return m, errors.New("站点指标未取得；站点定义或挑战需要适配")
	}
	return m, nil
}
func MetricsState(m Metrics) string {
	if m.Partial {
		return "partial"
	}
	return "succeeded"
}
