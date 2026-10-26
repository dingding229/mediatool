// Package idleupdate authorizes and coordinates idle updates; F8b owns execution.
package idleupdate

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"media-tool/internal/modules/onlineupdate"
	"media-tool/internal/modules/updatecheck"
	"media-tool/internal/platform/buildinfo"
	"sync"
	"time"
)

var ErrConflict = errors.New("闲时更新设置已变化，请刷新后重试")
var ErrInvalid = errors.New("时间段须为不同的合法HH:mm，安静时长须为15–360分钟整数")

type Policy struct {
	Enabled      bool   `json:"enabled"`
	StartTime    string `json:"startTime"`
	EndTime      string `json:"endTime"`
	QuietMinutes int    `json:"quietMinutes"`
	Revision     int64  `json:"revision"`
	Reason       string `json:"reason"`
	GrantID      string `json:"-"`
}
type SaveRequest struct {
	Enabled      bool   `json:"enabled"`
	StartTime    string `json:"startTime"`
	EndTime      string `json:"endTime"`
	QuietMinutes int    `json:"quietMinutes"`
	Revision     int64  `json:"revision"`
	Password     string `json:"password"`
	Confirm      bool   `json:"confirm"`
}
type Condition struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason"`
}
type Evaluation struct {
	Ready      bool                 `json:"ready"`
	Timezone   string               `json:"timezone"`
	CheckedAt  time.Time            `json:"checkedAt"`
	Conditions []Condition          `json:"conditions"`
	Target     *updatecheck.Release `json:"target,omitempty"`
}
type Updater interface {
	IdleStatus(context.Context) (onlineupdate.Status, error)
	Start(context.Context, string, string, string) (onlineupdate.Job, error)
	IdleTargetPreflight(context.Context, updatecheck.Release, bool) error
	JobsByActor(string) ([]onlineupdate.Job, error)
}
type Checker interface {
	VerifyFresh(context.Context) (updatecheck.Release, error)
}
type Service struct {
	DB      *pgxpool.Pool
	Updater Updater
	Checker Checker
	Root    string
	Logger  *slog.Logger
	Now     func() time.Time
	Current func() buildinfo.Info
	mu      sync.Mutex
	grantMu sync.Mutex
}

func New(db *pgxpool.Pool, u Updater, c Checker) *Service {
	return &Service{DB: db, Updater: u, Checker: c, Root: onlineupdate.Root + "/idle", Logger: slog.Default(), Now: time.Now, Current: buildinfo.Current}
}
func validTime(v string) bool {
	t, e := time.Parse("15:04", v)
	return e == nil && t.Format("15:04") == v
}
func (v SaveRequest) Validate() error {
	if !validTime(v.StartTime) || !validTime(v.EndTime) || v.StartTime == v.EndTime || v.QuietMinutes < 15 || v.QuietMinutes > 360 {
		return ErrInvalid
	}
	return nil
}
func InWindow(now time.Time, start, end string) bool {
	v := now.Format("15:04")
	if start < end {
		return v >= start && v < end
	}
	return v >= start || v < end
}
func (v Evaluation) passingExcept(keys ...string) bool {
	for _, c := range v.Conditions {
		skip := false
		for _, k := range keys {
			skip = skip || c.Key == k
		}
		if !skip && !c.Passed {
			return false
		}
	}
	return true
}
func (v *Evaluation) add(key, label string, pass bool, reason string) {
	if pass {
		reason = "已通过"
	}
	v.Conditions = append(v.Conditions, Condition{key, label, pass, reason})
}
func failedReason(err error) string {
	if errors.Is(err, onlineupdate.ErrImageConfigChanged) {
		return "镜像启动配置已变化，本版需手动更新"
	}
	return "目标镜像未缓存或安全预检未通过；调度执行前须准备并再次校验"
}
func (s *Service) Get(ctx context.Context) (Policy, error) {
	var p Policy
	e := s.DB.QueryRow(ctx, `SELECT enabled,start_time,end_time,quiet_minutes,revision,reason,grant_id FROM idle_update_policy WHERE singleton`).Scan(&p.Enabled, &p.StartTime, &p.EndTime, &p.QuietMinutes, &p.Revision, &p.Reason, &p.GrantID)
	return p, e
}
func (s *Service) audit(ctx context.Context, action string, details any) error {
	_, e := s.DB.Exec(ctx, `INSERT INTO audit_logs(actor,action,details) VALUES('system:idle-update',$1,$2)`, action, details)
	return e
}
func (s *Service) log(err error) {
	if err != nil {
		s.Logger.Warn("闲时自动更新暂缓，未启动新的更新", "module", "idleupdate", "reason", "条件读取或对账未完成，请检查私有更新记录")
	}
}
