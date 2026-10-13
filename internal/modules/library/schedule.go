package library

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/scheduling"
	"path"
	"strings"
	"time"
)

type ScheduleConfig struct {
	ID              string     `json:"id"`
	Root            string     `json:"root"`
	Enabled         bool       `json:"enabled"`
	Interval        int        `json:"-"`
	Cron            string     `json:"cron"`
	Revision        int64      `json:"revision"`
	NextRun         time.Time  `json:"nextRunAt"`
	LastError       string     `json:"lastError"`
	WatchChanges    bool       `json:"watchChanges"`
	ChangeScanAfter *time.Time `json:"changeScanAfter,omitempty"`
	LastChangeAt    *time.Time `json:"lastChangeAt,omitempty"`
	WatchState      string     `json:"watchState"`
	WatchMessage    string     `json:"watchMessage"`
}

func (e Engine) SaveSchedule(ctx context.Context, in ScheduleConfig) (ScheduleConfig, error) {
	next, cronErr := scheduling.Next(in.Cron, time.Now(), 5*time.Minute)
	if cronErr != nil {
		return in, cronErr
	}
	c, err := e.Connections.Repository.Default(ctx, "clouddrive2")
	if err != nil {
		return in, errors.New("请先在服务连接中配置并启用 CloudDrive2")
	}
	root := c.String("cloudRoot")
	if root == "" {
		if len(e.Connections.Config.STRMAllowedRoots) == 0 {
			return in, errors.New("configure an allowed scan root")
		}
		root = e.Connections.Config.STRMAllowedRoots[0]
	}
	if in.Root == "" {
		in.Root = root
	}
	if path.Clean(in.Root) != in.Root || !strings.HasPrefix(in.Root, "/") || strings.ContainsAny(in.Root, "\\\x00") || !inside(root, in.Root) {
		return in, errors.New("scan root outside configured cloud directory")
	}
	in.LastError = ""
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return in, err
	}
	defer tx.Rollback(ctx)
	if in.ID == "" {
		in.ID = jobs.UUID()
		err = tx.QueryRow(ctx, `INSERT INTO library_scan_schedules(id,integration_id,root,enabled,cron_expression,watch_changes,watch_state)VALUES($1,$2,$3,$4,$5,$6,CASE WHEN $4 AND $6 THEN 'waiting' ELSE 'disabled' END)RETURNING revision,next_run_at,change_scan_after,last_change_at,watch_state,watch_message`, in.ID, c.Record.ID, in.Root, in.Enabled, in.Cron, in.WatchChanges).Scan(&in.Revision, &in.NextRun, &in.ChangeScanAfter, &in.LastChangeAt, &in.WatchState, &in.WatchMessage)
	} else {
		err = tx.QueryRow(ctx, `UPDATE library_scan_schedules SET enabled=$3,cron_expression=$4,revision=revision+1,next_run_at=now(),watch_changes=$7,change_scan_after=CASE WHEN $3 AND $7 THEN change_scan_after ELSE NULL END,watch_state=CASE WHEN $3 AND $7 THEN 'waiting' ELSE 'disabled' END,watch_message='',updated_at=now()WHERE id=$1 AND revision=$2 AND integration_id=$5 AND root=$6 RETURNING revision,next_run_at,change_scan_after,last_change_at,watch_state,watch_message`, in.ID, in.Revision, in.Enabled, in.Cron, c.Record.ID, in.Root, in.WatchChanges).Scan(&in.Revision, &in.NextRun, &in.ChangeScanAfter, &in.LastChangeAt, &in.WatchState, &in.WatchMessage)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return in, errors.New("schedule changed; reload before editing")
	}
	if err != nil {
		return in, err
	}
	raw, _ := json.Marshal(map[string]string{"scheduleId": in.ID})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','library.schedule',$1)`, raw); err != nil {
		return in, err
	}
	if _, err = tx.Exec(ctx, `UPDATE library_scan_schedules SET next_run_at=$2 WHERE id=$1`, in.ID, next); err != nil {
		return in, err
	}
	in.NextRun = next
	return in, tx.Commit(ctx)
}
func (e Engine) Schedules(ctx context.Context) ([]ScheduleConfig, error) {
	rows, err := e.DB.Query(ctx, `SELECT id::text,root,enabled,cron_expression,revision,next_run_at,last_error,watch_changes,change_scan_after,last_change_at,watch_state,watch_message FROM library_scan_schedules ORDER BY root`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScheduleConfig{}
	for rows.Next() {
		var in ScheduleConfig
		if err = rows.Scan(&in.ID, &in.Root, &in.Enabled, &in.Cron, &in.Revision, &in.NextRun, &in.LastError, &in.WatchChanges, &in.ChangeScanAfter, &in.LastChangeAt, &in.WatchState, &in.WatchMessage); err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// Scheduled subdirectory scans reconcile only that subtree. A partial/failed
// traversal never casts a missing-file vote outside its coverage.
func (e Engine) Schedule(ctx context.Context) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT s.id::text,s.integration_id::text,s.root,s.cron_expression,coalesce(s.watch_changes AND s.change_scan_after<=now(),false) FROM library_scan_schedules s JOIN integrations i ON i.id=s.integration_id WHERE s.enabled AND i.enabled AND (s.next_run_at<=now() OR (s.watch_changes AND s.change_scan_after<=now())) ORDER BY s.next_run_at LIMIT 10 FOR UPDATE OF s SKIP LOCKED`)
	if err != nil {
		return err
	}
	type due struct {
		id, connection, root string
		cron                 string
		dirty                bool
	}
	list := []due{}
	for rows.Next() {
		var s due
		if err = rows.Scan(&s.id, &s.connection, &s.root, &s.cron, &s.dirty); err != nil {
			rows.Close()
			return err
		}
		list = append(list, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, s := range list {
		nextRun, cronErr := scheduling.Next(s.cron, time.Now(), 5*time.Minute)
		if cronErr != nil {
			return cronErr
		}
		c, loadErr := e.Connections.Repository.Default(ctx, "clouddrive2")
		message := ""
		var scan *string
		if loadErr != nil || c.Record.ID != s.connection {
			message = "主连接已改变，请核对定时扫描配置"
		} else {
			allowed := c.String("cloudRoot")
			if allowed == "" && len(e.Connections.Config.STRMAllowedRoots) > 0 {
				allowed = e.Connections.Config.STRMAllowedRoots[0]
			}
			var id string
			var created bool
			var startErr error
			if allowed == "" || !inside(allowed, s.root) {
				startErr = errors.New("scan root changed")
			} else {
				id, created, startErr = startScanTx(ctx, tx, s.connection, s.root)
			}
			if startErr != nil {
				message = "扫描创建失败，请核对根目录和连接"
			} else {
				scan = &id
				if s.dirty && !created {
					// The active traversal may have passed the changed path.
					// Retain the dirty hint until a genuinely new scan is admitted.
					if _, err = tx.Exec(ctx, `UPDATE library_scan_schedules SET change_scan_after=now()+interval '20 seconds',next_run_at=greatest(next_run_at,now()+interval '20 seconds'),last_scan_id=$2,updated_at=now() WHERE id=$1`, s.id, id); err != nil {
						return err
					}
					continue
				}
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE library_scan_schedules SET next_run_at=$2,last_scan_id=coalesce($3,last_scan_id),last_error=$4,change_scan_after=CASE WHEN $4='' THEN NULL ELSE change_scan_after END,updated_at=now()WHERE id=$1`, s.id, nextRun, scan, message); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
