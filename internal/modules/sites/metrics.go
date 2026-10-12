package sites

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
)

type Metrics struct {
	Name         string   `json:"username,omitempty"`
	Uploaded     *int64   `json:"uploaded"`
	Downloaded   *int64   `json:"downloaded"`
	Ratio        *float64 `json:"ratio"`
	Bonus        *float64 `json:"bonus"`
	Level        string   `json:"level,omitempty"`
	Seeding      *int64   `json:"seeding"`
	SeedingBytes *int64   `json:"seedingBytes"`
	Partial      bool     `json:"partial"`
	Notes        []string `json:"notes"`
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
	if seed := c.Config.Requests["seeding_statistics"]; seed != nil && vars["userId"] != "" {
		raw, mode, e := c.call(ctx, seed, vars, nil)
		if e == nil {
			doc, e := parseDocument(raw, mode)
			if e == nil {
				list := object(seed["list"])
				scopes, e := doc.values(text(list["selector"]), nil)
				if e == nil {
					count := int64(len(scopes))
					sum := int64(0)
					valid := true
					for _, scope := range scopes {
						v, e := mapFields(doc, Map{"size": object(seed["fields"])["size"]}, scope, vars)
						n := metricBytes(v["size"])
						if e != nil || n == nil || sum > math.MaxInt64-*n {
							valid = false
							break
						}
						sum += *n
					}
					next := object(list["next_page"])
					more := false
					if len(next) > 0 {
						v, e := mapFields(doc, Map{"next": next}, nil, vars)
						more = e != nil || !blank(v["next"])
					}
					if !more && count > 0 && valid {
						m.Seeding = &count
						if valid {
							m.SeedingBytes = &sum
						}
					} else {
						m.Notes = append(m.Notes, "做种列表可能分页或无法证明空列表；未将不完整数量当总数")
					}
				}
			}
		}
		if e != nil {
			m.Notes = append(m.Notes, "做种统计读取失败")
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
