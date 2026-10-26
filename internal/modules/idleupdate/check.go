package idleupdate

import (
	"context"
	"media-tool/internal/platform/buildinfo"
	"strings"
	"time"
)

// Check is read-only: no attempt claim, image pull, configuration, grant or audit writes.
func (s *Service) Check(ctx context.Context) (Evaluation, error) { return s.evaluate(ctx, "") }
func (s *Service) evaluate(ctx context.Context, ownAttempt string) (Evaluation, error) {
	now := s.Now().In(time.Local)
	zone := now.Location().String()
	v := Evaluation{CheckedAt: now, Timezone: zone, Conditions: []Condition{}}
	p, e := s.Get(ctx)
	if e != nil {
		return v, e
	}
	v.add("enabled", "自动更新授权", p.Enabled, "闲时自动更新已关闭")
	v.add("window", "服务器时间段", InWindow(now, p.StartTime, p.EndTime), "当前不在执行时间段内")
	var authorized, sessions, quiet, tasks bool
	e = s.DB.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM idle_update_policy p JOIN administrator_accounts a ON a.id=p.account_id WHERE p.singleton AND p.grant_id<>'' AND a.credential_version=p.credential_version AND NOT a.must_change_password AND a.temporary_password_expires_at IS NULL),
 NOT EXISTS(SELECT 1 FROM native_playback_sessions WHERE state<>'stopped' AND last_progress_at >= clock_timestamp()-interval '5 minutes'),
 EXISTS(SELECT 1 FROM idle_update_activity WHERE singleton AND last_issued_at <= clock_timestamp()-($1*interval '1 minute')),
 NOT (EXISTS(SELECT 1 FROM jobs WHERE state IN('running','unknown','checking') AND kind<>'plugin.v2.service')
 OR EXISTS(SELECT 1 FROM download_tasks WHERE download_state IN('added','downloading'))
 OR EXISTS(SELECT 1 FROM manual_download_tasks WHERE state IN('submitting','downloading','uncertain'))
 OR EXISTS(SELECT 1 FROM transfer_tasks WHERE state IN('starting','uploading','uncertain','output_resume_approved'))
 OR EXISTS(SELECT 1 FROM file_batches WHERE state='running')
 OR EXISTS(SELECT 1 FROM ops_backups WHERE state IN('pending','running'))
 OR EXISTS(SELECT 1 FROM ops_drills WHERE state IN('pending','running'))
 OR EXISTS(SELECT 1 FROM ops_web_restores WHERE state IN('pending','running','unknown'))
 OR EXISTS(SELECT 1 FROM ops_mode WHERE singleton AND paused))`, p.QuietMinutes).Scan(&authorized, &sessions, &quiet, &tasks)
	if e != nil {
		return v, e
	}
	v.add("authorization", "管理员授权", authorized, "授权缺失或管理员凭据已变化，需重新验密")
	release, verifyErr := s.Checker.VerifyFresh(ctx)
	current := s.Current()
	cmp, stable := buildinfo.Compare(strings.Split(current.Version, "-dev+")[0], release.Version)
	trusted := verifyErr == nil && stable && cmp < 0 && current.Schema >= release.MinimumSchema
	v.add("release", "已校验稳定版", trusted, "没有高于当前版本且与 Hub 一致的可用稳定版")
	if trusted {
		v.Target = &release
	}
	v.add("sessions", "原生播放会话", sessions, "存在5分钟心跳有效期内的播放或暂停会话")
	v.add("quiet", "近期播放请求", quiet && activityReliable(now, p.QuietMinutes), "尚未达到配置的无302/PlaybackInfo安静时长")
	v.add("tasks", "关键任务", tasks, "存在关键任务、状态未知任务或维护窗口")
	status, statusErr := s.Updater.IdleStatus(ctx)
	v.add("recovery", "人工恢复锁", statusErr == nil && len(status.Recovery) == 0, "人工恢复未确认或更新状态不可读取")
	v.add("environment", "更新环境", statusErr == nil && status.Available && (status.Job == nil || status.Job.Done()), "Docker环境不可用或已有更新正在执行")
	var daily bool
	if trusted {
		e = s.DB.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM idle_update_attempts WHERE version=$1 AND local_day=$2::date AND id<>$3)`, release.Version, now.Format("2006-01-02"), ownAttempt).Scan(&daily)
		if e != nil {
			return v, e
		}
	}
	entries, journalErr := s.entries()
	if journalErr != nil {
		daily = false
	}
	for _, entry := range entries {
		if entry.Version == release.Version && entry.Day == now.Format("2006-01-02") && entry.ID != ownAttempt {
			daily = false
		}
	}
	v.add("daily", "每日尝试限制", trusted && daily, "该版本今天已尝试，或尚无可信目标版本")
	var preflightErr error
	if trusted && statusErr == nil && status.Available {
		preflightErr = s.Updater.IdleTargetPreflight(ctx, release, false)
	} else {
		preflightErr = verifyErr
		if preflightErr == nil {
			preflightErr = errNoTarget
		}
	}
	reason := failedReason(preflightErr)
	if !trusted {
		reason = "尚无可用稳定版，无法预检"
	}
	v.add("startup", "镜像启动配置", preflightErr == nil, reason)
	v.Ready = v.passingExcept()
	return v, nil
}

var errNoTarget = &noTarget{}

type noTarget struct{}

func (*noTarget) Error() string { return "没有可预检的目标镜像" }
