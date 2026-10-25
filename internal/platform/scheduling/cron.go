// Package scheduling owns the application's business scheduling contract.
package scheduling

import (
	"errors"
	"fmt"
	"github.com/robfig/cron/v3"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

const CoreReconcileCron = "*/15 * * * * *"
const Timezone = "Asia/Shanghai"
const MaxExpressionBytes = 128
const MaxSteps = 4096
const MaxComputeTime = 250 * time.Millisecond

var ErrExhausted = errors.New("计划已结束，没有后续执行时间")
var ErrBudget = errors.New("计划计算超过安全上限，请简化表达式")
var location, _ = time.LoadLocation(Timezone)
var baseParser = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

type Parser struct{}
type schedule struct {
	base   cron.Schedule
	years  []int
	fields int
}
type budget struct {
	steps    int
	deadline time.Time
}

func newBudget() *budget { return &budget{deadline: time.Now().Add(MaxComputeTime)} }
func (b *budget) check() error {
	b.steps++
	if b.steps > MaxSteps || time.Now().After(b.deadline) {
		return ErrBudget
	}
	return nil
}
func (Parser) Parse(expression string) (cron.Schedule, error) { return parse(expression) }
func parse(expression string) (*schedule, error) {
	if len(expression) > MaxExpressionBytes {
		return nil, errors.New("Cron 表达式最多128字节")
	}
	fields := strings.Fields(expression)
	if (len(fields) != 5 && len(fields) != 6 && len(fields) != 7) || strings.ContainsAny(expression, "@=") {
		return nil, errors.New("请输入最多128字节的5、6或7字段 Cron 表达式")
	}
	s := &schedule{fields: len(fields)}
	if len(fields) == 7 {
		var err error
		s.years, err = years(fields[6])
		if err != nil {
			return nil, err
		}
		fields = fields[:6]
	}
	base, err := baseParser.Parse("CRON_TZ=" + Timezone + " " + strings.Join(fields, " "))
	if err != nil {
		return nil, fmt.Errorf("Cron 字段格式或范围无效：%w", err)
	}
	s.base = base
	return s, nil
}
func years(field string) ([]int, error) {
	bad := errors.New("年字段须为1970～2199，支持星号、列表、范围和步长")
	values := map[int]bool{}
	for _, item := range strings.Split(field, ",") {
		p := strings.Split(item, "/")
		if len(p) > 2 {
			return nil, bad
		}
		step := 1
		var err error
		if len(p) == 2 {
			step, err = strconv.Atoi(p[1])
			if err != nil || step < 1 || step > 230 {
				return nil, bad
			}
		}
		lo, hi := 1970, 2199
		if p[0] != "*" {
			r := strings.Split(p[0], "-")
			if len(r) > 2 {
				return nil, bad
			}
			lo, err = strconv.Atoi(r[0])
			if err != nil {
				return nil, bad
			}
			hi = lo
			if len(r) == 2 {
				hi, err = strconv.Atoi(r[1])
				if err != nil {
					return nil, bad
				}
			} else if len(p) == 2 {
				hi = 2199
			}
		}
		if lo < 1970 || hi > 2199 || lo > hi {
			return nil, bad
		}
		for n := lo; n <= hi; n += step {
			values[n] = true
		}
	}
	out := []int{}
	for n := 1970; n <= 2199; n++ {
		if values[n] {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return nil, bad
	}
	return out, nil
}
func (s *schedule) Next(now time.Time) time.Time { next, _ := s.next(now, newBudget()); return next }
func (s *schedule) next(now time.Time, b *budget) (time.Time, error) {
	if err := b.check(); err != nil {
		return time.Time{}, err
	}
	now = now.In(location)
	if s.fields < 7 {
		return s.base.Next(now), nil
	}
	for _, year := range s.years {
		if year < now.Year() {
			continue
		}
		if err := b.check(); err != nil {
			return time.Time{}, err
		}
		from := now
		if year > now.Year() {
			from = time.Date(year, 1, 1, 0, 0, 0, 0, location).Add(-time.Nanosecond)
		}
		next := s.base.Next(from)
		if !next.IsZero() && next.In(location).Year() == year {
			return next, nil
		}
	}
	return time.Time{}, nil
}

// validate retains the existing 128-successor frequency check. Finite plans may
// end earlier; exhaustion is not an invalid calendar and must not lose the last run.
func (s *schedule) validate(now time.Time, minimum time.Duration, b *budget) (time.Time, error) {
	first, err := s.next(now, b)
	if err != nil {
		return first, err
	}
	if first.IsZero() {
		return first, ErrExhausted
	}
	previous := first
	for i := 0; i < 128; i++ {
		next, e := s.next(previous, b)
		if e != nil {
			return time.Time{}, e
		}
		if next.IsZero() {
			if s.fields == 7 {
				break
			}
			return time.Time{}, ErrExhausted
		}
		if next.Sub(previous) < minimum {
			return time.Time{}, errors.New("Cron 执行间隔小于此业务允许的最小间隔")
		}
		previous = next
	}
	return first, nil
}
func Next(expression string, now time.Time, minimum time.Duration) (time.Time, error) {
	s, err := parse(expression)
	if err != nil {
		return time.Time{}, err
	}
	return s.validate(now, minimum, newBudget())
}

// NextOptional is for dispatch only. Save/enable must use Next, rejecting plans
// without future runs; dispatch stores NULL after the final due slot is queued.
func NextOptional(expression string, now time.Time, minimum time.Duration) (*time.Time, error) {
	s, err := parse(expression)
	if err != nil {
		return nil, err
	}
	next, err := s.validate(now, minimum, newBudget())
	if errors.Is(err, ErrExhausted) && s.fields == 7 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &next, nil
}

type PreviewResult struct {
	Timezone               string      `json:"timezone"`
	FieldCount             int         `json:"fieldCount"`
	ServerNow              time.Time   `json:"serverNow"`
	Next                   []time.Time `json:"next"`
	Exhausted              bool        `json:"exhausted"`
	MinimumIntervalSeconds int64       `json:"minimumIntervalSeconds"`
}

func Preview(expression, purpose string, now time.Time) (PreviewResult, error) {
	minimum, ok := map[string]time.Duration{"generic": 0, "subscription": 5 * time.Minute, "scan": 5 * time.Minute, "site-metrics": 15 * time.Minute, "backup": time.Hour, "plugin": time.Minute}[purpose]
	out := PreviewResult{Timezone: Timezone, ServerNow: now, Next: []time.Time{}, MinimumIntervalSeconds: int64(minimum / time.Second)}
	if !ok {
		return out, errors.New("不支持的计划用途")
	}
	s, err := parse(expression)
	if err != nil {
		return out, err
	}
	out.FieldCount = s.fields
	b := newBudget()
	first, err := s.validate(now, minimum, b)
	if errors.Is(err, ErrExhausted) {
		out.Exhausted = true
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Next = append(out.Next, first)
	for len(out.Next) < 5 {
		next, e := s.next(out.Next[len(out.Next)-1], b)
		if e != nil {
			return out, e
		}
		if next.IsZero() {
			break
		}
		out.Next = append(out.Next, next)
	}
	return out, nil
}
