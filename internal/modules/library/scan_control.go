package library

import (
	"context"
	"errors"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"strconv"
)

// Resume creates a new generation, not a replay of incomplete coverage. Already
// indexed stable files remain; missing votes require a new complete traversal.
func (e Engine) ControlScan(ctx context.Context, id, action string) error {
	if action != "pause" && action != "resume" && action != "cancel" {
		return errors.New("invalid scan action")
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var state, connection string
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT state,integration_id::text,generation FROM library_scans WHERE id=$1 FOR UPDATE`, id).Scan(&state, &connection, &generation); err != nil {
		return err
	}
	next := state
	switch action {
	case "pause":
		if state == "paused" {
			return nil
		}
		if state != "running" && state != "pending" {
			return errors.New("only active scans can be paused")
		}
		next = "paused"
	case "cancel":
		if state == "cancelled" {
			return nil
		}
		if state == "succeeded" {
			return errors.New("completed scan cannot be cancelled")
		}
		next = "cancelled"
	case "resume":
		if state != "paused" && state != "failed" {
			return errors.New("only paused or failed scans can restart")
		}
		next = "pending"
	}
	// Read current connection under the same admission guard used by file writes.
	var revision int64
	var enabled bool
	if err = tx.QueryRow(ctx, `SELECT revision,enabled FROM integrations WHERE id=$1 FOR SHARE`, connection).Scan(&revision, &enabled); err != nil {
		return err
	}
	if action == "resume" {
		if !enabled {
			return errors.New("connection disabled")
		}
		var busy bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_operations WHERE integration_id=$1 AND state IN ('running','needs_reconcile'))`, connection).Scan(&busy); err != nil {
			return err
		}
		if busy {
			return errors.New("reconcile file operations before restarting scan")
		}
	}
	generation++
	if _, err = tx.Exec(ctx, `UPDATE library_scans SET state=$2,generation=$3,updated_at=now(),message=$4,integration_revision=CASE WHEN $2='pending' THEN $5 ELSE integration_revision END WHERE id=$1`, id, next, generation, "扫描控制："+action+"；不完整覆盖不用于缺失判断", revision); err != nil {
		return err
	}
	if action == "resume" {
		if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,queue,payload,idempotency_key)VALUES($1,'library.scan','library',jsonb_build_object('scanId',$2::text,'generation',$3::bigint),$4)`, jobs.UUID(), id, generation, "library.scan:"+id+":"+action+":"+strconv.FormatInt(generation, 10)); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','library.scan_control',jsonb_build_object('scanId',$1::text,'action',$2::text,'generation',$3::bigint))`, id, action, generation); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}

func (e Engine) ScanByID(ctx context.Context, id string) (Scan, error) {
	var s Scan
	err := e.DB.QueryRow(ctx, `SELECT id::text,integration_id::text,root,state,(SELECT count(*) FROM library_scan_seen s WHERE s.scan_id=library_scans.id),(SELECT count(*) FROM library_scan_directories d WHERE d.scan_id=library_scans.id),message,created_at,updated_at,integration_revision,generation,current_directory,(SELECT count(*) FROM library_scan_directories d WHERE d.scan_id=library_scans.id AND state='completed') FROM library_scans WHERE id=$1`, id).Scan(&s.ID, &s.ConnectionID, &s.Root, &s.State, &s.Files, &s.Directories, &s.Message, &s.CreatedAt, &s.UpdatedAt, &s.Revision, &s.Generation, &s.CurrentDirectory, &s.CompletedDirectories)
	return s, err
}
