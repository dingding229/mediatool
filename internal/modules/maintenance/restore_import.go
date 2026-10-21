package maintenance

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"sort"
	"strings"
)

// These runtime leases must never resurrect external side effects from a backup.
// Configuration, credentials, grants, KV, media and settings are not preserved.
const restoreRuntimeSQL = `
UPDATE administrator_sessions SET revoked_at=clock_timestamp();
UPDATE media_user_sessions SET revoked_at=clock_timestamp();
UPDATE jobs SET state='paused',lease_until=NULL,lease_token=NULL,locked_by=NULL,last_error='恢复后需人工核对，不自动重放' WHERE state IN('pending','retry','running');
UPDATE outbox_events SET dispatched_at=now() WHERE dispatched_at IS NULL;
UPDATE notification_deliveries SET state='outcome_unknown',last_error='恢复后需人工核对，不自动重发' WHERE state IN('pending','retry','sending');
UPDATE plugin_v2_instances SET data_operation=NULL,generation=NULL,owner_id=NULL,lease_until=NULL,worker_pid=0,worker_start='',runtime_state=CASE WHEN reason IN('permission_review_required','definition_missing','definition_unsupported') THEN 'needs_action' ELSE 'stopped' END;
UPDATE plugin_v2_subscriptions SET cursor=(SELECT coalesce(max(cursor),0)FROM plugin_v2_event_journal),source_since=clock_timestamp(),polled_at=clock_timestamp();
UPDATE plugin_v2_state_tasks SET state='failed',cleaned=true,result='{"error":"恢复后需核对磁盘状态，旧任务不自动重放"}' WHERE state IN('writing','prepared');
UPDATE plugin_v2_state_snapshots SET state='failed' WHERE state='writing';
UPDATE plugin_v2_schedules SET wake_job=NULL;
UPDATE plugin_v2_runs SET state='unknown',reason='worker_lost',finished_at=now() WHERE state IN('pending','running');
`

func (e Engine) importWebDump(ctx context.Context, file, id, backup string) error {
	if !UUID.MatchString(id) || !UUID.MatchString(backup) {
		return ErrUnsafe
	}
	order, err := e.restoreOrder(ctx)
	if err != nil {
		return err
	}
	list, clean, err := e.restoreList(ctx, file, order)
	if err != nil {
		return err
	}
	defer clean()
	f, err := os.OpenFile(file, os.O_RDONLY|noFollowFlag(), 0)
	if err != nil {
		return err
	}
	defer deferutil.Close(f, "private restore archive")
	sql, err := os.CreateTemp(e.Root, ".restore-*.sql")
	if err != nil {
		return err
	}
	defer func() { deferutil.Close(sql, "private restore SQL"); _ = os.Remove(sql.Name()) }()
	// pg_restore generates COPY data only; no archive DDL, triggers disabled, or
	// sequence rewind is accepted. psql's single transaction covers every table.
	err = e.pg(ctx, "pg_restore", e.DSN, []string{"--file=-", "--data-only", "--schema=public", "--use-list=" + list, "--no-owner", "--no-acl"}, f, sql)
	if err != nil {
		return err
	}
	if _, err = sql.Seek(0, 0); err != nil {
		return err
	}
	names := append([]string(nil), order...)
	sort.Strings(names)
	qualified := []string{}
	for _, n := range names {
		qualified = append(qualified, pgx.Identifier{"public", n}.Sanitize())
	}
	var prefix strings.Builder
	prefix.WriteString("SET LOCAL lock_timeout='5s';\nLOCK TABLE " + strings.Join(qualified, ",") + " IN SHARE ROW EXCLUSIVE MODE;\n")
	for i := len(order) - 1; i >= 0; i-- {
		prefix.WriteString("DELETE FROM " + pgx.Identifier{"public", order[i]}.Sanitize() + ";\n")
	}
	// Preserve monotonic counters (setval itself is not transactional). Advancing
	// a counter may leave gaps after failure, but never duplicates existing IDs.
	backupSequences, err := e.backupSequenceValues(ctx, file)
	if err != nil {
		return err
	}
	seq, err := e.sequenceAdvanceSQL(ctx, order, backupSequences)
	if err != nil {
		return err
	}
	suffix := "SET LOCAL search_path=pg_catalog,public;\n" + restoreRuntimeSQL + seq + fmt.Sprintf("UPDATE ops_requests SET state='failed',message='恢复后旧批准无效' WHERE state IN('preview','approved','executing') AND id<>'%s';\n", id) + fmt.Sprintf("\nINSERT INTO public.ops_restore_commit(request_id,backup_id)VALUES('%s','%s') ON CONFLICT(request_id)DO UPDATE SET backup_id=excluded.backup_id;\n", id, backup)
	return e.pg(ctx, "psql", e.DSN, []string{"-X", "--set=ON_ERROR_STOP=1", "--single-transaction", "--file=-", "--dbname=" + databaseName(e.DSN)}, io.MultiReader(strings.NewReader(prefix.String()), sql, strings.NewReader(suffix)), io.Discard)
}
func (e Engine) sequenceAdvanceSQL(ctx context.Context, tables []string, backup map[string]int64) (string, error) {
	rows, err := e.DB.Query(ctx, `SELECT t.relname,a.attname,s.relname FROM pg_class t JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum>0 AND NOT a.attisdropped JOIN pg_depend d ON d.refobjid=t.oid AND d.refobjsubid=a.attnum AND d.deptype IN('a','i') JOIN pg_class s ON s.oid=d.objid AND s.relkind='S' WHERE t.relnamespace='public'::regnamespace AND t.relname=ANY($1)`, tables)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var t, c, s string
		if err = rows.Scan(&t, &c, &s); err != nil {
			return "", err
		}
		qi := pgx.Identifier{"public", s}.Sanitize()
		literal := "'" + strings.ReplaceAll(qi, "'", "''") + "'"
		b.WriteString(fmt.Sprintf("SELECT pg_catalog.setval(%s,greatest((SELECT last_value FROM %s),%d,coalesce((SELECT max(%s) FROM %s),1)),true);\n", literal, qi, backup[s], pgx.Identifier{c}.Sanitize(), pgx.Identifier{"public", t}.Sanitize()))
	}
	return b.String(), errors.Join(rows.Err())
}
