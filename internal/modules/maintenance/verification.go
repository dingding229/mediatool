package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/platform/buildinfo"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/flow"
	"time"
)

func RequestFlow(state string, verified, expired bool) flow.Status {
	if expired && (state == "preview" || state == "approved") {
		return flow.Status{Phase: "preview", NextAction: "create_preview", Reason: "approval_expired"}
	}
	switch state {
	case "preview":
		return flow.Status{Phase: "preview", NextAction: "approve", Reason: "preview_only"}
	case "approved":
		return flow.Status{Phase: "approve", NextAction: "execute_on_owner_host", Reason: "approved_not_executed"}
	case "executing":
		return flow.Status{Phase: "execute", NextAction: "inspect_owner_receipt", Reason: "execution_outcome_pending"}
	case "completed":
		if verified {
			return flow.Status{Phase: "complete", NextAction: "resume_after_review", Reason: "protected_data_verified"}
		}
		return flow.Status{Phase: "verify", NextAction: "verify_execution", Reason: "execution_completed_not_verified"}
	case "failed":
		return flow.Status{Phase: "verify", NextAction: "inspect_owner_receipt", Reason: "do_not_replay_unknown_effects"}
	default:
		return flow.Status{Phase: "preview", NextAction: "create_preview", Reason: "request_not_executable"}
	}
}
func (r Request) MarshalJSON() ([]byte, error) {
	type plain Request
	return json.Marshal(struct {
		plain
		Flow flow.Status `json:"flow"`
	}{plain(r), RequestFlow(r.State, r.VerifiedAt != nil, time.Now().After(r.ExpiresAt))})
}
func protectedDigest(values map[string]string) string {
	raw, _ := json.Marshal(values)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Verification records schema/revision/protected-data readback, not approval,
// execution, business unpause, or proof of physical player/CDN behavior.
func (e Engine) VerifyExecution(ctx context.Context, id string) (Request, error) {
	r, err := e.Request(ctx, id)
	if err != nil {
		return r, err
	}
	if r.State != "completed" {
		return r, ErrConflict
	}
	tx, err := e.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return r, err
	}
	defer deferutil.Rollback(ctx, tx)
	var paused bool
	if err = tx.QueryRow(ctx, `SELECT paused FROM ops_mode WHERE singleton FOR UPDATE`).Scan(&paused); err != nil {
		return r, err
	}
	var digest, state string
	var payload []byte
	if err = tx.QueryRow(ctx, `SELECT digest,state,payload FROM ops_requests WHERE id=$1 FOR UPDATE`, id).Scan(&digest, &state, &payload); err != nil {
		return r, err
	}
	if state != "completed" || digest != r.Digest {
		return r, ErrConflict
	}
	if err = json.Unmarshal(payload, &r.Payload); err != nil {
		return r, err
	}
	raw, err := json.Marshal(r.Payload)
	if err != nil || hash(raw) != digest {
		return r, ErrUnsafe
	}
	var version int64
	if err = tx.QueryRow(ctx, SchemaSQL).Scan(&version); err != nil {
		return r, err
	}
	target := r.Payload.TargetRevision
	if target == "" {
		target = r.Payload.CurrentRevision
	}
	if version != buildinfo.Schema || version != r.Payload.Schema || target != e.Revision || !SHA.MatchString(target) {
		return r, ErrConflict
	}
	current, err := Protected(ctx, tx)
	if err != nil {
		return r, err
	}
	expected := r.Payload.Protected
	if r.Kind == "restore" {
		restored, err := e.WebRestore(ctx, id)
		if err != nil || restored.State != "completed" {
			return r, ErrConflict
		}
		var committed string
		if err = tx.QueryRow(ctx, `SELECT backup_id::text FROM ops_restore_commit WHERE request_id=$1`, id).Scan(&committed); err != nil || restored.AppliedBackup == nil || committed != restored.BackupID || *restored.AppliedBackup != committed {
			return r, ErrConflict
		}
		expected = restored.Protected
	}
	if !sameProtected(current, expected) {
		return r, ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO ops_request_verifications(request_id,request_digest,schema_version,revision,protected_digest)VALUES($1,$2,$3,$4,$5) ON CONFLICT(request_id)DO UPDATE SET request_digest=excluded.request_digest,schema_version=excluded.schema_version,revision=excluded.revision,protected_digest=excluded.protected_digest,verified_at=clock_timestamp()`, id, r.Digest, version, e.Revision, protectedDigest(current))
	if err != nil {
		return r, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','ops.execution.verify',jsonb_build_object('requestId',$1::text))`, id); err != nil {
		return r, err
	}
	if err = tx.Commit(ctx); err != nil {
		return r, err
	}
	return e.Request(ctx, id)
}
