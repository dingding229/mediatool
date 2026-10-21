package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/platform/buildinfo"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/database"
)

// Restore uses the existing private bundle, clone verifier and PostgreSQL clients.
// It never edits Compose, the master key, downloads or an existing output file.
type WebRestore struct {
	BackupSchema  int64             `json:"backupSchema"`
	ProgramSchema int64             `json:"programSchema"`
	ID            string            `json:"id"`
	BackupID      string            `json:"backupId"`
	SnapshotID    *string           `json:"snapshotId"`
	State         string            `json:"state"`
	Digest        string            `json:"digest"`
	Protected     map[string]string `json:"-"`
	AppliedBackup *string           `json:"appliedBackup"`
	Message       string            `json:"message"`
	CreatedAt     time.Time         `json:"createdAt"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	CompletedAt   *time.Time        `json:"completedAt"`
}

var RestoreIncludes = []string{"PostgreSQL：通过保护校验的业务数据（旧待执行任务暂停）", "outputs.tar：仅补回缺失且摘要匹配的受管 STRM/NFO/字幕"}
var RestoreExcludes = []string{"主密钥、Compose、环境变量、其他 /data 配置", "115 授权、账号、媒体归属：不同则拒绝，不回退", "现存 STRM 正文与输出、115 原媒体、downloads、qbittorrent-downloads、logs", "Emby、CloudDrive2 及其他外部服务配置/卷", "插件实例、权限、配置、KV及工作流记录保留当前值，不回退", "管理端会话与恢复标记、当前连接和115授权表保留在线，不覆盖"}

const webRestoreColumns = `SELECT id::text,backup_id::text,snapshot_id::text,state,manifest_digest,protected,applied_backup::text,message,created_at,expires_at,completed_at,(SELECT coalesce((b.manifest->>'schema')::bigint,0)FROM ops_backups b WHERE b.id=backup_id) FROM ops_web_restores`

func scanWebRestore(row interface{ Scan(...any) error }) (WebRestore, error) {
	var r WebRestore
	var raw []byte
	err := row.Scan(&r.ID, &r.BackupID, &r.SnapshotID, &r.State, &r.Digest, &raw, &r.AppliedBackup, &r.Message, &r.CreatedAt, &r.ExpiresAt, &r.CompletedAt, &r.BackupSchema)
	if err == nil {
		err = json.Unmarshal(raw, &r.Protected)
		r.ProgramSchema = buildinfo.Schema
	}
	return r, err
}
func (e Engine) WebRestore(ctx context.Context, id string) (WebRestore, error) {
	if !UUID.MatchString(id) {
		return WebRestore{}, ErrUnsafe
	}
	return scanWebRestore(e.DB.QueryRow(ctx, webRestoreColumns+` WHERE id=$1`, id))
}
func (e Engine) WebRestores(ctx context.Context) ([]WebRestore, error) {
	rows, err := e.DB.Query(ctx, webRestoreColumns+` ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WebRestore{}
	for rows.Next() {
		r, err := scanWebRestore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (e Engine) PreviewWebRestore(ctx context.Context, id string) (WebRestore, error) {
	// Serialize expensive preview validation with backup/drill/restore admission.
	conn, err := e.DB.Acquire(ctx)
	if err != nil {
		return WebRestore{}, err
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(91220880)`).Scan(&locked); err != nil || !locked {
		return WebRestore{}, ErrConflict
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(c, `SELECT pg_advisory_unlock(91220880)`); err != nil {
			conn.Conn().Close(c)
		}
	}()
	var busy bool
	if err = conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ops_backups WHERE state IN('pending','running','deleting')) OR EXISTS(SELECT 1 FROM ops_drills WHERE state IN('pending','running')) OR EXISTS(SELECT 1 FROM ops_web_restores WHERE state IN('pending','running','unknown'))`).Scan(&busy); err != nil {
		return WebRestore{}, err
	}
	if busy {
		return WebRestore{}, ErrConflict
	}
	if err = e.restoreSchema(ctx); err != nil {
		return WebRestore{}, err
	}
	m, dir, err := e.VerifyBundle(ctx, id)
	if err != nil {
		return WebRestore{}, err
	}
	if m.Schema > buildinfo.Schema {
		return WebRestore{}, errors.New("备份 schema 高于当前程序，禁止恢复；请先升级程序")
	}
	current, err := Protected(ctx, e.DB)
	if err != nil {
		return WebRestore{}, err
	}
	if !sameProtected(current, m.Protected) {
		return WebRestore{}, errors.New("账号、115授权或媒体归属与当前不同，禁止回退保护数据")
	}
	// Existing clone verifier validates every ciphertext, schema migration and inventory.
	if e.validateRestore != nil {
		err = e.validateRestore(ctx, m, dir)
	} else {
		_, clean, checkErr := e.restoreClone(ctx, "media_tool_restore_"+stripID(newID())[:12], dir, m)
		err = checkErr
		if err == nil {
			err = clean()
		}
	}
	if err != nil {
		return WebRestore{}, err
	}
	if err = e.applyOutputs(ctx, filepath.Join(dir, "outputs.tar"), m.Files, false); err != nil {
		return WebRestore{}, err
	}
	r := WebRestore{ID: newID(), BackupID: id}
	raw, _ := json.Marshal(current)
	var digest string
	if err = e.DB.QueryRow(ctx, `SELECT digest FROM ops_backups WHERE id=$1`, id).Scan(&digest); err != nil {
		return r, err
	}
	_, err = e.DB.Exec(ctx, `INSERT INTO ops_web_restores(id,backup_id,manifest_digest,protected)VALUES($1,$2,$3,$4)`, r.ID, id, digest, raw)
	if err != nil {
		return r, err
	}
	return e.WebRestore(ctx, r.ID)
}

// Queue is serialized with backup/drill/delete admission. The same preview ID is
// the idempotency key; repeat submissions return its receipt, never a new job.
func (e Engine) QueueWebRestore(ctx context.Context, id, digest, text string) (WebRestore, error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return WebRestore{}, err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(91220880)`); err != nil {
		return WebRestore{}, err
	}
	r, err := scanWebRestore(tx.QueryRow(ctx, webRestoreColumns+` WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return r, err
	}
	if r.Digest != digest || text != "恢复 "+r.BackupID {
		return r, ErrUnsafe
	}
	if r.State != "preview" {
		return r, tx.Commit(ctx)
	}
	if time.Now().After(r.ExpiresAt) {
		return r, ErrConflict
	}
	var busy bool
	if err = tx.QueryRow(ctx, `SELECT (SELECT paused FROM ops_mode WHERE singleton) OR EXISTS(SELECT 1 FROM ops_backups WHERE state IN('pending','running','deleting')) OR EXISTS(SELECT 1 FROM ops_drills WHERE state IN('pending','running')) OR EXISTS(SELECT 1 FROM ops_web_restores WHERE state IN('pending','running','unknown'))`).Scan(&busy); err != nil {
		return r, err
	}
	if busy {
		return r, ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE ops_web_restores SET state='pending',message='已排队，尚未恢复' WHERE id=$1;`, id); err != nil {
		return r, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','ops.web_restore.queued',jsonb_build_object('id',$1::text))`, id); err != nil {
		return r, err
	}
	if err = tx.Commit(ctx); err != nil {
		return r, err
	}
	return e.WebRestore(ctx, id)
}

// RunWebRestore is driven by a dedicated app loop, not jobs: restoring a jobs
// snapshot cannot destroy the executor's own lease. A session advisory lock
// survives the transactional table replacement and fences every app instance.
func (e Engine) RunWebRestore(ctx context.Context) error {
	conn, err := e.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(91220882)`).Scan(&locked); err != nil || !locked {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(c, `SELECT pg_advisory_unlock(91220882)`); err != nil {
			conn.Conn().Close(c)
		}
	}()
	// A released executor lock with a running ledger is an interrupted operation.
	if _, err = conn.Exec(ctx, `UPDATE ops_web_restores SET state='unknown',message='执行进程中断；请核对提交标记，禁止盲目重跑' WHERE state='running'`); err != nil {
		return err
	}
	if _, err = conn.Exec(ctx, `UPDATE ops_mode SET paused=true,reason='网页恢复 '||(SELECT id::text FROM ops_web_restores WHERE state='unknown' LIMIT 1),revision=revision+1 WHERE singleton AND EXISTS(SELECT 1 FROM ops_web_restores WHERE state='unknown') AND (NOT paused OR reason IS DISTINCT FROM '网页恢复 '||(SELECT id::text FROM ops_web_restores WHERE state='unknown' LIMIT 1))`); err != nil {
		return err
	}
	var id string
	err = conn.QueryRow(ctx, `UPDATE ops_web_restores SET state='running',message='等待业务任务结束' WHERE id=(SELECT id FROM ops_web_restores WHERE state='pending' ORDER BY created_at LIMIT 1) RETURNING id::text`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return e.executeWebRestore(ctx, id)
}
func (e Engine) executeWebRestore(ctx context.Context, id string) (result error) {
	r, err := e.WebRestore(ctx, id)
	if err != nil {
		return err
	}
	if _, err = e.DB.Exec(ctx, `UPDATE ops_mode SET paused=true,reason=$1,revision=revision+1 WHERE singleton`, "网页恢复 "+id); err != nil {
		return err
	}
	// Unknown outcomes deliberately keep maintenance enabled. Only a readback or
	// verified automatic rollback may release it.
	markUnknown := func() {
		if result != nil {
			c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, _ = e.DB.Exec(c, `WITH paused AS(UPDATE ops_mode SET paused=true,reason=$2,revision=revision+1 WHERE singleton)UPDATE ops_web_restores SET state='unknown',message=$3,completed_at=NULL WHERE id=$1`, id, "网页恢复 "+id, result.Error())
		}
	}
	gate, err := e.DB.Acquire(ctx)
	if err != nil {
		result = err
		markUnknown()
		return err
	}
	defer gate.Release()
	gateLocked := false
	defer func() {
		if gateLocked {
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := gate.Exec(c, `SELECT pg_advisory_unlock(91220883)`); err != nil {
				gate.Conn().Close(c)
			}
		}
	}()
	// On uncertain commit, re-pause before releasing the writer gate.
	defer markUnknown()
	drain, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		var n int
		if err = e.DB.QueryRow(drain, `SELECT (SELECT count(*) FROM jobs WHERE state='running')+(SELECT count(*) FROM plugin_v2_instances WHERE generation IS NOT NULL)`).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if err = gate.QueryRow(drain, `SELECT pg_try_advisory_lock(91220883)`).Scan(&gateLocked); err != nil {
				return err
			}
			if gateLocked {
				break
			}
		}
		select {
		case <-drain.Done():
			return errors.New("业务任务未在120秒内结束，未恢复；保持维护等待核对")
		case <-ticker.C:
		}
	}
	if err = e.restoreSchema(ctx); err != nil {
		return e.finishUnchanged(ctx, id, err)
	}
	m, dir, err := e.VerifyBundle(ctx, r.BackupID)
	if err != nil {
		return e.finishUnchanged(ctx, id, err)
	}
	var digest string
	if err = e.DB.QueryRow(ctx, `SELECT digest FROM ops_backups WHERE id=$1`, r.BackupID).Scan(&digest); err != nil {
		return err
	}
	current, err := Protected(ctx, e.DB)
	if err != nil {
		return err
	}
	if digest != r.Digest || !sameProtected(current, r.Protected) || !sameProtected(current, m.Protected) {
		return e.finishUnchanged(ctx, id, ErrConflict)
	}
	// Reuse Backup synchronously; the latest ready snapshot is protected by retention.
	snapshot := newID()
	if _, err = e.DB.Exec(ctx, `INSERT INTO ops_backups(id,origin,pinned)VALUES($1,'pre-restore',false);`, snapshot); err != nil {
		return err
	}
	if _, err = e.DB.Exec(ctx, `UPDATE ops_web_restores SET snapshot_id=$2,message='正在创建恢复前快照' WHERE id=$1`, id, snapshot); err != nil {
		return err
	}
	if err = e.Backup(ctx, snapshot); err != nil {
		return e.finishUnchanged(ctx, id, fmt.Errorf("恢复前快照失败，未执行恢复：%w", err))
	}
	if err = e.restoreStage(ctx, id, "恢复前快照已就绪，正在校验候选数据库并向上迁移"); err != nil {
		return err
	}
	// Candidate preparation validates all ciphertexts and migrates older backups.
	file, cleanup, err := e.webDump(ctx, r, m, dir)
	if cleanup != nil {
		defer e.cleanupWebCandidate(id, cleanup)
	}
	if err != nil {
		return e.finishUnchanged(ctx, id, err)
	}
	if err = e.restoreOutputs(ctx, filepath.Join(dir, "outputs.tar"), m.Files); err != nil {
		return e.finishUnchanged(ctx, id, err)
	}
	if err = e.restoreStage(ctx, id, "正在事务导入业务数据库；管理会话与执行记录保持在线"); err != nil {
		return err
	}
	if err = e.importWebDump(ctx, file); err == nil {
		if err = e.verifyApplied(ctx, id, r.Protected); err == nil {
			return e.finishWebRestore(ctx, id, "completed", "恢复成功；恢复前快照已保留，旧任务不会自动重放，请重新登录")
		}
	}
	// pg_restore is one transaction. Still restore the preflight snapshot after a
	// reported import/readback failure, rather than assuming a timeout rolled back.
	rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer rollbackCancel()
	_ = e.restoreStage(rollbackCtx, id, "恢复失败，正在自动回滚到恢复前快照："+err.Error())
	sm, sd, se := e.VerifyBundle(rollbackCtx, snapshot)
	if se == nil {
		var f string
		var clean func() error
		f, clean, se = e.webDump(rollbackCtx, r, sm, sd)
		if clean != nil {
			defer e.cleanupWebCandidate(id, clean)
		}
		if se == nil {
			se = e.importWebDump(rollbackCtx, f)
		}
	}
	if se == nil {
		var committed string
		se = e.DB.QueryRow(rollbackCtx, `SELECT backup_id::text FROM ops_restore_commit WHERE request_id=$1`, id).Scan(&committed)
		if se == nil && committed != snapshot {
			se = errors.New("回滚事务提交标记不符，保持维护等待核对")
		}
	}
	if se == nil {
		var p map[string]string
		p, se = Protected(rollbackCtx, e.DB)
		if se == nil && !sameProtected(p, r.Protected) {
			se = errors.New("回滚保护数据核验失败")
		}
	}
	if se != nil {
		return fmt.Errorf("恢复失败：%v；自动回滚失败：%v；保持维护，请按快照核对", err, se)
	}
	return e.finishWebRestore(rollbackCtx, id, "rolled_back", "恢复失败，已自动回滚到恢复前快照："+err.Error())
}
func (e Engine) finishUnchanged(ctx context.Context, id string, cause error) error {
	return e.finishWebRestore(ctx, id, "failed", "未执行数据库恢复："+cause.Error())
}
func (e Engine) finishWebRestore(ctx context.Context, id, state, message string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `UPDATE ops_web_restores SET state=$2,message=$3,completed_at=now(),applied_backup=(SELECT backup_id FROM ops_restore_commit WHERE request_id=$1) WHERE id=$1`, id, state, message); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE ops_mode SET paused=false,reason='',revision=revision+1 WHERE singleton AND reason=$1`, "网页恢复 "+id); err != nil {
		return err
	}
	if state == "completed" || state == "rolled_back" {
		if _, err = tx.Exec(ctx, `UPDATE administrator_sessions SET revoked_at=now()`); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('system','ops.web_restore.finished',jsonb_build_object('id',$1::text,'state',$2::text))`, id, state); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (e Engine) verifyApplied(ctx context.Context, id string, expected map[string]string) error {
	r, err := e.WebRestore(ctx, id)
	if err != nil {
		return err
	}
	var committed string
	if err = e.DB.QueryRow(ctx, `SELECT backup_id::text FROM ops_restore_commit WHERE request_id=$1`, id).Scan(&committed); err != nil {
		return err
	}
	if committed != r.BackupID {
		return errors.New("事务提交标记不存在")
	}
	p, err := Protected(ctx, e.DB)
	if err != nil {
		return err
	}
	if !sameProtected(p, expected) {
		return errors.New("恢复后保护数据核验失败")
	}
	return nil
}

// Reconciliation never reruns pg_restore. applied is imported atomically with
// every business table, so absence/presence distinguishes pre/post-commit crashes.
func (e Engine) ReconcileWebRestore(ctx context.Context, id string) (WebRestore, error) {
	conn, err := e.DB.Acquire(ctx)
	if err != nil {
		return WebRestore{}, err
	}
	defer conn.Release()
	var ok bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(91220882)`).Scan(&ok); err != nil || !ok {
		return WebRestore{}, ErrConflict
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(c, `SELECT pg_advisory_unlock(91220882)`); err != nil {
			conn.Conn().Close(c)
		}
	}()
	r, err := e.WebRestore(ctx, id)
	if err != nil {
		return r, err
	}
	var importing bool
	if err = conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND application_name='mediatool-maintenance' AND pid<>pg_backend_pid())`).Scan(&importing); err != nil {
		return r, err
	}
	if importing {
		return r, errors.New("PostgreSQL维护客户端仍在运行，请等待退出后核对")
	}
	if r.State != "unknown" {
		return r, ErrConflict
	}
	var committed *string
	if err = conn.QueryRow(ctx, `SELECT(SELECT backup_id::text FROM ops_restore_commit WHERE request_id=$1)`, id).Scan(&committed); err != nil {
		return r, err
	}
	r.AppliedBackup = committed
	if r.AppliedBackup != nil {
		p, checkErr := Protected(ctx, e.DB)
		if checkErr != nil {
			return r, checkErr
		}
		if !sameProtected(p, r.Protected) {
			return r, ErrConflict
		}
		state := "completed"
		if *r.AppliedBackup != r.BackupID {
			if r.SnapshotID == nil || *r.AppliedBackup != *r.SnapshotID {
				return r, ErrConflict
			}
			state = "rolled_back"
		}
		err = e.finishWebRestore(ctx, id, state, "已核对事务提交标记与保护数据；未重新执行恢复")
	} else {
		err = e.finishUnchanged(ctx, id, errors.New("已核对数据库导入事务未提交"))
	}
	if err != nil {
		return r, err
	}
	return e.WebRestore(ctx, id)
}
func (e Engine) webDump(ctx context.Context, r WebRestore, m Manifest, dir string) (string, func() error, error) {
	if e.prepareRestore != nil {
		return e.prepareRestore(ctx, r, m, dir)
	}
	return e.prepareWebDump(ctx, r, m, dir)
}
func (e Engine) prepareWebDump(ctx context.Context, r WebRestore, m Manifest, dir string) (string, func() error, error) {
	name := "media_tool_restore_" + stripID(newID())[:12]
	dsn, clean, err := e.restoreClone(ctx, name, dir, m)
	if err != nil {
		return "", clean, err
	}
	clone, err := database.Open(ctx, dsn)
	if err != nil {
		return "", clean, err
	}
	defer clone.Close()
	if err = e.copyCurrentPlugins(ctx, clone); err != nil {
		return "", clean, err
	}
	if err = e.copyCurrentMetadata(ctx, clone); err != nil {
		return "", clean, err
	}
	// Sessions and pending work are not replayed. The admin may log in again using
	// the unchanged protected accounts. Metadata, including snapshots, stays current.
	_, err = clone.Exec(ctx, `UPDATE plugin_v2_runs SET state='unknown',reason='worker_lost',finished_at=now() WHERE state IN('pending','running');UPDATE administrator_sessions SET revoked_at=now();UPDATE media_user_sessions SET revoked_at=now();UPDATE jobs SET state='paused',lease_until=NULL,lease_token=NULL,locked_by=NULL,last_error='网页恢复后需人工核对，不自动重放' WHERE state IN('pending','retry','running');UPDATE outbox_events SET dispatched_at=now() WHERE dispatched_at IS NULL;UPDATE notification_deliveries SET state='outcome_unknown',last_error='恢复后需人工核对，不自动重发' WHERE state IN('pending','retry','sending');`)
	if err != nil {
		return "", clean, err
	}
	if _, err = clone.Exec(ctx, `INSERT INTO ops_restore_commit(request_id,backup_id)VALUES($1,$2)ON CONFLICT(request_id)DO UPDATE SET backup_id=excluded.backup_id`, r.ID, m.ID); err != nil {
		return "", clean, err
	}
	// Dump the verified, current-schema candidate in the existing custom format.
	temp, err := os.CreateTemp(e.Root, ".restore-*.dump")
	if err != nil {
		return "", clean, err
	}
	file := temp.Name()
	cleanup := func() error { return errors.Join(os.Remove(file), clean()) }
	err = e.pg(ctx, "pg_dump", dsn, []string{"--format=custom", "--schema=public", "--no-owner", "--no-acl"}, nil, temp)
	err = errors.Join(err, temp.Sync(), temp.Close())
	return file, cleanup, err
}
func (e Engine) importWebDump(ctx context.Context, file string) error {
	f, err := os.OpenFile(file, os.O_RDONLY|noFollowFlag(), 0)
	if err != nil {
		return err
	}
	defer deferutil.Close(f, "private restore dump")
	// No DROP DATABASE, no pool/DSN switch. PostgreSQL rolls back every DROP/COPY
	// together on error. Progress/auth stay live; the commit stamp is in the transaction.
	list, clean, err := e.restoreList(ctx, file)
	if err != nil {
		return err
	}
	defer clean()
	return e.pg(ctx, "pg_restore", e.DSN, []string{"--schema=public", "--use-list=" + list, "--clean", "--if-exists", "--single-transaction", "--exit-on-error", "--no-owner", "--no-acl", "--dbname=" + databaseName(e.DSN)}, f, io.Discard)
}
func databaseName(dsn string) string { // pg env carries secrets; argv contains name only.
	u, err := url.Parse(dsn)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}
func (e Engine) WebRestoreActive(ctx context.Context) (bool, error) {
	if e.DB == nil {
		return false, nil
	}
	var active bool
	err := e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.ops_web_restores WHERE state IN('pending','running','unknown'))`).Scan(&active)
	return active, err
}

// The loop is part of app, cancellable on shutdown, and has no automatic replay.
func (e Engine) ServeWebRestores(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bounded, cancel := context.WithTimeout(ctx, 30*time.Minute)
			if err := e.RunWebRestore(bounded); err != nil {
				deferutil.Report(err, "web restore executor")
			}
			cancel()
		}
	}
}

// Candidate cleanup failure does not turn an already verified database commit
// into unknown, and must never briefly resume jobs before re-pausing them.
func (e Engine) cleanupWebCandidate(id string, clean func() error) {
	if err := clean(); err != nil {
		deferutil.Report(err, "web restore candidate cleanup")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = e.DB.Exec(ctx, `UPDATE ops_web_restores SET message=message||$2 WHERE id=$1`, id, "；候选库清理失败，请由所有者核对："+err.Error())
	}
}

func (e Engine) restoreStage(ctx context.Context, id, message string) error {
	_, err := e.DB.Exec(ctx, `UPDATE ops_web_restores SET message=$2 WHERE id=$1`, id, message)
	return err
}
func (e Engine) restoreSchema(ctx context.Context) error {
	var actual int64
	if err := e.DB.QueryRow(ctx, SchemaSQL).Scan(&actual); err != nil {
		return err
	}
	if actual != buildinfo.Schema {
		return errors.New("程序与当前数据库 schema 不一致，禁止恢复；请先核对运行版本")
	}
	return nil
}
