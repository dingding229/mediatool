package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/storage"
	"strings"
)

type UploadControl struct {
	Action                string `json:"action"`
	AllowRestartIfAbsent  bool   `json:"allowRestartIfAbsent"`
	SessionID             string `json:"sessionId,omitempty"`
	ConfirmSessionBinding bool   `json:"confirmSessionBinding"`
}

func (e Engine) Control(ctx context.Context, id string, in UploadControl) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,714))`, id); err != nil {
		return err
	}
	var connectionID, destination, session, hash, endpoint, state string
	var expectedSize int64
	err = tx.QueryRow(ctx, `SELECT t.integration_id::text,t.destination,coalesce(t.upload_id,''),coalesce(t.source_sha1,''),coalesce(t.endpoint_snapshot,''),t.state,f.size_bytes FROM transfer_tasks t JOIN download_files f ON f.id=t.download_file_id WHERE t.id=$1 FOR UPDATE OF t`, id).Scan(&connectionID, &destination, &session, &hash, &endpoint, &state, &expectedSize)
	if err != nil {
		return errors.New("transfer unavailable")
	}
	if state == "needs_output_reconciliation" || state == outputApprovedState {
		return errors.New("输出布局等待明确迁移，不是活动上传会话；请核对云盘索引后恢复入库，不能重建上传")
	}
	connection, err := e.Connections.Repository.Get(ctx, connectionID)
	if err != nil {
		return err
	}
	if endpoint != "" && endpoint != connection.String("endpoint") {
		return errors.New("upload endpoint changed; restore original endpoint before reconciliation")
	}
	client, err := e.Connections.CloudByID(ctx, connectionID)
	if err != nil {
		return err
	}
	if in.Action == "reconcile" {
		var running bool
		if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE idempotency_key=$1 AND state='running'AND lease_until>now())`, "transfer:"+id).Scan(&running); err != nil {
			return err
		}
		if running {
			return errors.New("a live upload worker owns this session; pause or wait before reconciliation")
		}

		remote, findErr := client.Find(ctx, destination)
		if findErr == nil {
			if remote.Size != expectedSize || hash == "" || !strings.EqualFold(remote.Hashes[2], hash) {
				return errors.New("remote file does not prove this upload completed")
			}
			if _, err = tx.Exec(ctx, `UPDATE transfer_tasks SET control_revision=control_revision+1 WHERE id=$1`, id); err != nil {
				return err
			}
			if err = tx.Commit(ctx); err != nil {
				return err
			}
			return e.markVerified(ctx, id, remote)
		}
		if !errors.Is(findErr, storage.ErrNotFound) {
			return findErr
		}
		active, listErr := client.ActiveUploadAt(ctx, destination)
		if listErr != nil {
			return errors.New("cannot prove remote upload is absent; no automatic restart")
		}
		if active {
			return errors.New("CloudDrive2 still has a matching upload; do not create another")
		}
		if !in.AllowRestartIfAbsent {
			return errors.New("no matching remote task/file; explicitly confirm a restart before creating a new session")
		}
		if _, err = tx.Exec(ctx, `UPDATE transfer_tasks SET state='pending',control_revision=control_revision+1,upload_id=NULL,last_error='',phase='pending',updated_at=now()WHERE id=$1`, id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE jobs SET state='pending',attempts=0,available_at=now(),last_error=NULL WHERE idempotency_key=$1 AND state IN ('failed','retry','paused','pending')`, "transfer:"+id); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if state == "imported" || state == "verified" || state == "artifacts_pending" {
		return errors.New("verified media cannot be controlled as a live upload")
	}
	if in.Action == "bind" {
		var live bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE idempotency_key=$1 AND state='running'AND lease_until>now())`, "transfer:"+id).Scan(&live); err != nil || live {
			return errors.New("live upload worker owns session; wait before binding")
		}
		if session != "" || !in.ConfirmSessionBinding || in.SessionID == "" || len(in.SessionID) > 256 || strings.ContainsAny(in.SessionID, "\r\n\x00") {
			return errors.New("explicit session ID and acknowledgement required; existing session cannot be replaced")
		}
		sessions, listErr := client.UploadsAt(ctx, destination)
		if listErr != nil {
			return listErr
		}
		match := false
		for _, candidate := range sessions {
			if candidate.ID == in.SessionID && candidate.Size == uint64(expectedSize) {
				match = true
			}
		}
		if !match {
			return errors.New("session key/path/size did not match a listed remote upload; cannot bind")
		}
		// key is not specified as upload_id by every CloudDrive release. The caller
		// explicitly supplies it and the control RPC must acknowledge it first.
		session = in.SessionID
		if err = client.ControlUpload(ctx, session, "pause"); err != nil {
			return errors.New("session control was not acknowledged; binding rejected")
		}
		if _, err = tx.Exec(ctx, `UPDATE transfer_tasks SET upload_id=$2,state='paused',control_revision=control_revision+1,updated_at=now()WHERE id=$1`, id, session); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE jobs SET state='paused',lease_until=NULL,lease_token=NULL,locked_by=NULL WHERE idempotency_key=$1`, "transfer:"+id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','transfer.session_bound',jsonb_build_object('transferId',$1::text))`, id); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if in.Action == "resume" && state != "paused" {
		return errors.New("only an explicitly paused upload can resume")
	}
	if in.Action != "pause" && in.Action != "resume" && in.Action != "cancel" {
		return errors.New("unsupported control")
	}
	if err = client.ControlUpload(ctx, session, in.Action); err != nil {
		if _, err = tx.Exec(ctx, `UPDATE transfer_tasks SET state='uncertain',control_revision=control_revision+1,phase='control_outcome_unknown',updated_at=now()WHERE id=$1`, id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE jobs SET state='paused',lease_until=NULL,lease_token=NULL,locked_by=NULL WHERE idempotency_key=$1`, "transfer:"+id); err != nil {
			return err
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return commitErr
		}
		return errors.New("remote control outcome unknown; reconcile before retrying")
	}
	next := "paused"
	switch in.Action {
	case "resume":
		next = "uploading"
	case "cancel":
		next = "cancelled"
	case "pause":
	default:
		return errors.New("unsupported control")
	}
	if _, err = tx.Exec(ctx, `UPDATE transfer_tasks SET state=$2,control_revision=control_revision+1,updated_at=now()WHERE id=$1`, id, next); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE jobs SET state=$2,lease_until=NULL,lease_token=NULL,locked_by=NULL,updated_at=now()WHERE idempotency_key=$1`, "transfer:"+id, map[string]string{"paused": "paused", "cancelled": "cancelled", "uploading": "pending"}[next]); err != nil {
		return err
	}
	if in.Action == "resume" {
		if _, err = tx.Exec(ctx, `UPDATE jobs SET state='pending',attempts=0,available_at=now(),last_error=NULL WHERE idempotency_key=$1 AND state IN ('failed','retry','paused','pending')`, "transfer:"+id); err != nil {
			return err
		}
	}
	audit, _ := json.Marshal(map[string]string{"transferId": id, "action": in.Action})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','transfer.control',$1)`, audit); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
