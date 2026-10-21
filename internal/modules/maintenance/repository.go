package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	deferutil "media-tool/internal/platform/cleanup"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (e Engine) Backups(ctx context.Context) ([]Backup, error) {
	rows, err := e.DB.Query(ctx, `SELECT id::text,state,origin,pinned,bytes,digest,manifest,message,created_at,completed_at FROM ops_backups ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Backup{}
	for rows.Next() {
		var b Backup
		if err = rows.Scan(&b.ID, &b.State, &b.Origin, &b.Pinned, &b.Bytes, &b.Digest, &b.Manifest, &b.Message, &b.CreatedAt, &b.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (e Engine) Drills(ctx context.Context) ([]Drill, error) {
	rows, err := e.DB.Query(ctx, `SELECT id::text,backup_id::text,state,report,message,created_at,completed_at FROM ops_drills ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Drill{}
	for rows.Next() {
		var d Drill
		if err = rows.Scan(&d.ID, &d.BackupID, &d.State, &d.Report, &d.Message, &d.CreatedAt, &d.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (e Engine) Pin(ctx context.Context, id string, pin bool) error {
	tag, err := e.DB.Exec(ctx, `UPDATE ops_backups SET pinned=$2 WHERE id=$1 AND state='ready' AND ($2 OR id IS DISTINCT FROM(SELECT r.snapshot_id FROM ops_web_restores r JOIN ops_backups s ON s.id=r.snapshot_id WHERE s.state='ready' ORDER BY r.created_at DESC LIMIT 1))`, id, pin)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return err
}
func (e Engine) Delete(ctx context.Context, id string) error {
	if !UUID.MatchString(id) {
		return ErrUnsafe
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(91220880)`); err != nil {
		return err
	}
	var allowed bool
	err = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM ops_web_restores WHERE state IN('pending','running','unknown')) AND id IS DISTINCT FROM(SELECT r.snapshot_id FROM ops_web_restores r JOIN ops_backups s ON s.id=r.snapshot_id WHERE s.state='ready' ORDER BY r.created_at DESC LIMIT 1) AND NOT pinned AND state IN('ready','failed','interrupted') AND id NOT IN(SELECT id FROM ops_backups WHERE state='ready' ORDER BY created_at DESC LIMIT 2) AND NOT EXISTS(SELECT 1 FROM ops_drills WHERE backup_id=$1 AND state IN('pending','running'))AND NOT EXISTS(SELECT 1 FROM ops_requests WHERE payload->>'backupId'=$1::text AND state IN('preview','approved','executing')) FROM ops_backups WHERE id=$1 FOR UPDATE`, id).Scan(&allowed)
	if err != nil || !allowed {
		return errors.New("固定、最近两份成功备份或被维护任务引用的备份不可删除")
	}
	dir, err := e.dir(id)
	if err != nil {
		return err
	}
	tombstone := dir + ".deleting"
	paths, err := inspectBackupDeletion(dir, os.Lstat)
	if err != nil {
		return err
	}
	if paths.original.exists && paths.tombstone.exists {
		// Never replace an existing isolation directory with another backup.
		return ErrUnsafe
	}
	if (paths.original.exists && (!paths.original.info.IsDir() || paths.original.info.Mode()&os.ModeSymlink != 0)) || (paths.tombstone.exists && (!paths.tombstone.info.IsDir() || paths.tombstone.info.Mode()&os.ModeSymlink != 0)) {
		return ErrUnsafe
	}
	detached := paths.tombstone.exists
	filesExist := paths.original.exists || paths.tombstone.exists
	// Only known module-owned files; refuse an unexpected entry rather than recursive pruning.
	if filesExist {
		cleanupPath := dir
		if detached {
			cleanupPath = tombstone
		}
		entries, readErr := os.ReadDir(cleanupPath)
		if readErr != nil {
			return readErr
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || entry.Name() != "database.dump" && entry.Name() != "outputs.tar" && entry.Name() != "manifest.json" {
				return ErrUnsafe
			}
		}
	}
	var deletionStarted time.Time
	if err = tx.QueryRow(ctx, `UPDATE ops_backups SET state='deleting',deletion_started_at=clock_timestamp(),message='删除已确认，正在清理备份文件' WHERE id=$1 AND state IN('ready','failed','interrupted') RETURNING deletion_started_at`, id).Scan(&deletionStarted); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','ops.backup.delete_started',jsonb_build_object('id',$1::text))`, id); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}

	// Detach the directory first. Readers can no longer observe a ready backup
	// while the physical cleanup is in progress, and a later DB failure leaves
	// an explicit deleting/failed state rather than a false ready record.
	if filesExist {
		if !detached {
			if err = os.Rename(dir, tombstone); err != nil {
				return e.failBackupDeletion(ctx, id, deletionStarted, "备份目录隔离失败，文件未删除，可重新删除：", err)
			}
		}
		entries, readErr := os.ReadDir(tombstone)
		if readErr == nil {
			for _, entry := range entries {
				if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || entry.Name() != "database.dump" && entry.Name() != "outputs.tar" && entry.Name() != "manifest.json" {
					readErr = ErrUnsafe
					break
				}
			}
		}
		if readErr == nil {
			for _, entry := range entries {
				if removeErr := os.Remove(filepath.Join(tombstone, entry.Name())); removeErr != nil {
					readErr = removeErr
					break
				}
			}
		}
		if readErr == nil {
			readErr = os.Remove(tombstone)
		}
		if readErr != nil {
			return e.failBackupDeletion(ctx, id, deletionStarted, "备份文件清理失败，保留隔离目录，可重新删除：", readErr)
		}
	}

	finish, finishErr := e.DB.Begin(ctx)
	if finishErr != nil {
		return finishErr
	}
	defer deferutil.Rollback(ctx, finish)
	finishTag, finishErr := finish.Exec(ctx, `UPDATE ops_backups SET state='deleted',message='本模块备份已删除，业务媒体未删除',completed_at=now() WHERE id=$1 AND state='deleting' AND deletion_started_at=$2`, id, deletionStarted)
	if finishErr != nil {
		return finishErr
	}
	if finishTag.RowsAffected() != 1 {
		return ErrConflict
	}
	if _, finishErr = finish.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','ops.backup.delete',jsonb_build_object('id',$1::text))`, id); finishErr != nil {
		return finishErr
	}
	return finish.Commit(ctx)
}

func (e Engine) Retain(ctx context.Context, p Policy) error {
	recovered, err := e.recoverDeletingBackups(ctx, time.Now())
	if err != nil {
		return err
	}
	if _, checkedErr := e.DB.Exec(ctx, `UPDATE ops_requests SET state='expired',message='批准已过期' WHERE state IN('preview','approved')AND expires_at<now()`); checkedErr != nil {
		return checkedErr
	}
	if _, checkedErr := e.DB.Exec(ctx, `UPDATE ops_backups b SET state='interrupted',message='任务中断，不重放旧备份会话',completed_at=now() WHERE state IN('pending','running')AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.id=b.job_id AND j.state IN('pending','running','retry'))`); checkedErr != nil {
		return checkedErr
	}
	if _, checkedErr := e.DB.Exec(ctx, `UPDATE ops_drills d SET state='interrupted',message='演练中断；核对临时库后再清理',completed_at=now() WHERE state IN('pending','running')AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.id=d.job_id AND j.state IN('pending','running','retry'))`); checkedErr != nil {
		return checkedErr
	}

	// A deletion recovered in this cycle requires an administrator retry;
	// do not immediately start a second physical cleanup attempt.
	rows, err := e.DB.Query(ctx, `SELECT id::text FROM(SELECT id,pinned,created_at,row_number()OVER(ORDER BY created_at DESC)AS rank FROM ops_backups WHERE state='ready')b WHERE NOT pinned AND rank>2 AND (rank>$1 OR created_at<now()-make_interval(days=>$2)) UNION ALL SELECT id::text FROM ops_backups WHERE NOT pinned AND state IN('failed','interrupted')AND created_at<now()-make_interval(days=>$2) LIMIT 100`, p.KeepCount, p.KeepDays)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, ok := recovered[id]; ok {
			continue
		}
		_ = e.Delete(ctx, id)
	}
	return rows.Err()
}
func (e Engine) History(ctx context.Context, hours int) ([]map[string]any, error) {
	if hours < 1 || hours > 2160 {
		return nil, ErrUnsafe
	}
	rows, err := e.DB.Query(ctx, `SELECT to_timestamp(floor(extract(epoch FROM sampled_at)/($2*60))*($2*60)) AS bucket,avg((value->>'memoryPercent')::double precision),avg((value->>'diskFreeGB')::double precision),avg((value->>'load1')::double precision),avg((value->>'loadPerCPU')::double precision),avg((value->>'jobSeconds')::double precision),avg((value->>'processBytes')::double precision),max((value->>'pendingJobs')::integer),max((value->>'failedJobs10m')::integer),max((value->>'succeededJobs10m')::integer) FROM ops_samples WHERE sampled_at>now()-make_interval(hours=>$1) GROUP BY bucket ORDER BY bucket LIMIT 500`, hours, max(1, (hours*60+499)/500))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var at time.Time
		var mem, disk, load, ratio, seconds, process *float64
		var pending, failed, done *int
		if err = rows.Scan(&at, &mem, &disk, &load, &ratio, &seconds, &process, &pending, &failed, &done); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"at": at, "memoryPercent": mem, "diskFreeGB": disk, "load1": load, "loadPerCPU": ratio, "jobSeconds": seconds, "processBytes": process, "pendingJobs": pending, "failedJobs10m": failed, "succeededJobs10m": done})
	}
	return out, rows.Err()
}
func (e Engine) Alerts(ctx context.Context) ([]json.RawMessage, error) {
	rows, err := e.DB.Query(ctx, `SELECT to_jsonb(a) FROM ops_alerts a ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw json.RawMessage
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}

// A cleanup requires persisted CREATE evidence, not merely a matching database name.
func (e Engine) CleanupDrill(ctx context.Context, id string) error {
	if !UUID.MatchString(id) {
		return ErrUnsafe
	}
	var raw []byte
	var state string
	if err := e.DB.QueryRow(ctx, `SELECT state,report FROM ops_drills WHERE id=$1`, id).Scan(&state, &raw); err != nil {
		return err
	}
	if state != "failed" && state != "interrupted" {
		return ErrUnsafe
	}
	var report map[string]any
	if json.Unmarshal(raw, &report) != nil || report["created"] != true {
		return ErrUnsafe
	}
	name := "media_tool_restore_" + stripID(id)[:12]
	u, parseErr := url.Parse(e.DSN)
	if parseErr != nil || strings.TrimPrefix(u.Path, "/") == name {
		return ErrUnsafe
	}
	if report["createdDatabase"] != name {
		return ErrUnsafe
	}
	var active bool
	if err := e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs j JOIN ops_drills d ON d.job_id=j.id WHERE d.id=$1 AND j.state='running' AND j.lease_until>now())`, id).Scan(&active); err != nil {
		return err
	}
	if active {
		return ErrConflict
	}
	var owner string
	err := e.DB.QueryRow(ctx, `SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname=$1`, name).Scan(&owner)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil {
		var current string
		if checkedErr := e.DB.QueryRow(ctx, `SELECT current_user`).Scan(&current); checkedErr != nil {
			return checkedErr
		}
		if owner != current {
			return ErrUnsafe
		}
		if _, err = e.DB.Exec(ctx, `DROP DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
			return err
		}
	}
	report["databaseDeleted"] = true
	out, _ := json.Marshal(report)
	_, err = e.DB.Exec(ctx, `UPDATE ops_drills SET report=$2,message=message||'；本演练创建的临时库已清理' WHERE id=$1`, id, out)
	return err
}
