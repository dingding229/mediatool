package idleupdate

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/robfig/cron/v3"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/outbox"
	"media-tool/internal/platform/scheduling"
	"media-tool/internal/platform/updateidle"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

type attempt struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Day      string `json:"day"`
	Timezone string `json:"timezone"`
	Grant    string `json:"grant"`
	State    string `json:"state"`
	JobID    string `json:"jobId"`
	Message  string `json:"message"`
}

func (a attempt) actor() string { return "system:idle-update:" + a.ID }

var attemptName = regexp.MustCompile(`^[a-f0-9]{32}\.json$`)

func (s *Service) entries() ([]attempt, error) {
	list, e := os.ReadDir(s.Root)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	out := []attempt{}
	for _, f := range list {
		if !attemptName.MatchString(f.Name()) {
			continue
		}
		b, e := os.ReadFile(filepath.Join(s.Root, f.Name()))
		if e != nil {
			return nil, e
		}
		var a attempt
		if json.Unmarshal(b, &a) != nil || a.ID+".json" != f.Name() || a.Grant == "" || !validDay(a.Day) {
			return nil, errors.New("闲时更新持久标记损坏，需人工核对")
		}
		out = append(out, a)
	}
	return out, nil
}
func validDay(v string) bool {
	t, e := time.Parse("2006-01-02", v)
	return e == nil && t.Format("2006-01-02") == v
}
func (s *Service) persist(a attempt) error {
	if !attemptName.MatchString(a.ID + ".json") {
		return ErrInvalid
	}
	if e := os.MkdirAll(s.Root, 0700); e != nil {
		return e
	}
	b, e := json.Marshal(a)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.Root, ".attempt-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer func() {
		if e := os.Remove(name); e != nil && !os.IsNotExist(e) {
			s.log(e)
		}
	}()
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if e = os.Rename(name, filepath.Join(s.Root, a.ID+".json")); e != nil {
		return e
	}
	dir, e := os.Open(s.Root)
	if e != nil {
		return e
	}
	defer deferutil.Close(dir, "Close idle-update journal directory")
	return dir.Sync()
}
func (s *Service) complete(ctx context.Context, a attempt) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	_, e = tx.Exec(ctx, `INSERT INTO idle_update_attempts(id,version,local_day,timezone,grant_id,state,job_id,message)VALUES($1,$2,$3::date,$4,$5,$6,$7,$8) ON CONFLICT(version,local_day)DO UPDATE SET state=EXCLUDED.state,job_id=EXCLUDED.job_id,message=EXCLUDED.message,updated_at=now() WHERE idle_update_attempts.id=EXCLUDED.id`, a.ID, a.Version, a.Day, a.Timezone, a.Grant, a.State, a.JobID, a.Message)
	if e != nil {
		return e
	}
	b, _ := json.Marshal(a)
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details) SELECT $1,'idle_update.result',$2 WHERE NOT EXISTS(SELECT 1 FROM audit_logs WHERE actor=$1 AND action='idle_update.result')`, a.actor(), b); e != nil {
		return e
	}
	if e = outbox.EmitVersionTx(ctx, tx, "idle_update.result", a.ID, "terminal", map[string]string{"title": "闲时自动更新结果", "message": a.Version + "：" + resultText(a.State), "operationId": a.ID}); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	if a.State != "succeeded" && a.State != "deferred" {
		if e = s.disable(ctx, a.Grant, resultText(a.State)); e != nil {
			return e
		}
	}
	return s.persist(a)
}
func resultText(state string) string {
	switch state {
	case "succeeded":
		return "更新成功"
	case "deferred":
		return "最终复查不满足，今天不再尝试"
	case "rolled_back":
		return "更新失败，已回滚并关闭自动更新"
	case "manual_recovery":
		return "更新需人工恢复，已关闭自动更新"
	default:
		return "更新未确认成功，已关闭自动更新，请核对"
	}
}

// Reconcile runs before scheduling. Disk records deliberately survive an F8b database rollback.
func (s *Service) Reconcile(ctx context.Context) error {
	list, e := s.entries()
	if e != nil {
		_ = s.disable(ctx, "", "持久记录不可读取，已安全关闭")
		return e
	}
	for _, a := range list {
		if a.State == "preparing" || a.State == "queued" {
			jobs, e := s.Updater.JobsByActor(a.actor())
			if e != nil {
				return e
			}
			if len(jobs) == 1 {
				j := jobs[0]
				if !j.Done() {
					continue
				}
				a.State = j.Phase
				a.JobID = j.ID
			} else {
				a.State = "unknown"
				a.Message = "中断后未能唯一关联更新任务，禁止重试"
			}
		}
		if e = s.complete(ctx, a); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) Tick(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.Reconcile(ctx); e != nil {
		return e
	}
	p, e := s.Get(ctx)
	if e != nil || !p.Enabled {
		return e
	}
	v, e := s.evaluate(ctx, "")
	if e != nil {
		return e
	}
	for _, c := range v.Conditions {
		if !c.Passed && (c.Key == "authorization" || c.Key == "recovery") {
			return s.disable(ctx, p.GrantID, c.Reason)
		}
	}
	if !v.passingExcept("startup") {
		return nil
	}
	now := s.Now().In(time.Local)
	a := attempt{ID: newID(), Version: v.Target.Version, Day: now.Format("2006-01-02"), Timezone: now.Location().String(), Grant: p.GrantID, State: "preparing"}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	tag, e := tx.Exec(ctx, `INSERT INTO idle_update_attempts(id,version,local_day,timezone,grant_id,state)VALUES($1,$2,$3::date,$4,$5,$6)ON CONFLICT(version,local_day)DO NOTHING`, a.ID, a.Version, a.Day, a.Timezone, a.Grant, a.State)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	if e = s.persist(a); e != nil {
		_ = s.disable(ctx, a.Grant, "尝试标记写入失败")
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	fail := func(err error) error {
		a.State = "failed"
		a.Message = "准备或启动未确认成功"
		finish, done := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer done()
		return errors.Join(err, s.complete(finish, a))
	}
	prep, cancel := context.WithTimeout(ctx, 3*time.Minute)
	e = s.Updater.IdleTargetPreflight(prep, *v.Target, true)
	cancel()
	if e != nil {
		return fail(e)
	}
	s.grantMu.Lock()
	defer s.grantMu.Unlock()
	release, ok := updateidle.Default.CloseFor(5 * time.Minute)
	if !ok {
		a.State = "deferred"
		return s.complete(ctx, a)
	}
	defer release()
	final, stop := context.WithTimeout(ctx, 90*time.Second)
	defer stop()
	check, e := s.evaluate(final, a.ID)
	if e != nil {
		return fail(e)
	}
	latest, e := s.Get(final)
	if e != nil {
		return fail(e)
	}
	if !check.Ready || latest.GrantID != a.Grant || s.Now().In(time.Local).Format("2006-01-02") != a.Day || check.Target.Digest != v.Target.Digest {
		a.State = "deferred"
		return s.complete(final, a)
	}
	j, e := s.Updater.Start(final, a.actor(), a.Version, v.Target.Digest)
	if e != nil {
		// Start may have durably created a job before its response was lost. Never start twice.
		matches, readErr := s.Updater.JobsByActor(a.actor())
		if readErr == nil && len(matches) == 1 {
			j = matches[0]
		} else {
			return fail(e)
		}
	}
	a.JobID = j.ID
	a.State = "queued"
	if e = s.persist(a); e != nil {
		return e
	}
	_, e = s.DB.Exec(ctx, `UPDATE idle_update_attempts SET job_id=$2,state='queued',updated_at=now()WHERE id=$1`, a.ID, j.ID)
	return e
}
func (s *Service) Run(ctx context.Context) {
	if s.DB == nil {
		return
	}
	s.log(s.Reconcile(ctx))
	logger := scheduling.Logger{Log: s.Logger.With("module", "idleupdate")}
	scheduler := cron.New(cron.WithParser(scheduling.Parser{}), cron.WithLogger(logger), cron.WithChain(cron.Recover(logger), cron.SkipIfStillRunning(logger)))
	_, e := scheduler.AddFunc("0 * * * * *", func() { c, cancel := context.WithTimeout(ctx, 4*time.Minute); defer cancel(); s.log(s.Tick(c)) })
	if e != nil {
		s.log(e)
		return
	}
	scheduler.Start()
	<-ctx.Done()
	<-scheduler.Stop().Done()
}
