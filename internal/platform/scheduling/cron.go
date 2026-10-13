// Package scheduling owns the application's business scheduling contract.
package scheduling

import (
	"errors"
	"github.com/robfig/cron/v3"
	"strings"
	"time"
	_ "time/tzdata"
)

const Timezone = "Asia/Shanghai"

func Next(expression string, now time.Time, minimum time.Duration) (time.Time, error) {
	if len(expression) > 120 || len(strings.Fields(expression)) != 5 || strings.Contains(expression, "@") {
		return time.Time{}, errors.New("请输入五字段 cron 表达式（分 时 日 月 周），时区 Asia/Shanghai")
	}
	loc, err := time.LoadLocation(Timezone)
	if err != nil {
		return time.Time{}, err
	}
	schedule, err := cron.ParseStandard("CRON_TZ=" + Timezone + " " + expression)
	if err != nil {
		return time.Time{}, err
	}
	next := schedule.Next(now.In(loc))
	if next.IsZero() {
		return time.Time{}, errors.New("cron 无可执行日期")
	}
	previous := next
	for i := 0; i < 128; i++ {
		following := schedule.Next(previous)
		if following.IsZero() || following.Sub(previous) < minimum {
			return time.Time{}, errors.New("cron 执行过于频繁或无后续执行日期")
		}
		previous = following
	}
	return next, nil
}
