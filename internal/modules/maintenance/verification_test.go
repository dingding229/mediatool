package maintenance

import (
	"context"
	"encoding/json"
	"media-tool/internal/platform/buildinfo"
	"testing"
	"time"
)

func TestOpsFlowIsNotCompletionAtApprovalOrExecution(t *testing.T) {
	for _, state := range []string{"preview", "approved", "executing", "failed"} {
		f := RequestFlow(state, false, false)
		if f.Phase == "complete" || f.Retryable {
			t.Fatal(state, f)
		}
	}
	if f := RequestFlow("completed", false, false); f.Phase != "verify" {
		t.Fatal(f)
	}
	if f := RequestFlow("completed", true, false); f.Phase != "complete" {
		t.Fatal(f)
	}
	if f := RequestFlow("approved", false, true); f.NextAction != "create_preview" {
		t.Fatal(f)
	}
}
func TestOpsExecutionVerificationAndResumeRecheck(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	protected, err := Protected(ctx, e.DB)
	if err != nil {
		t.Fatal(err)
	}
	id := newID()
	payload, _ := json.Marshal(Payload{Schema: buildinfo.Schema, CurrentRevision: e.Revision, Protected: protected})
	digest := hash(payload)
	if _, err = e.DB.Exec(ctx, `INSERT INTO ops_requests(id,kind,state,payload,digest,expires_at)VALUES($1,'restore','approved',$2,$3,$4)`, id, payload, digest, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = e.VerifyExecution(ctx, id); err == nil {
		t.Fatal("approval treated as execution")
	}
	if _, err = e.DB.Exec(ctx, `UPDATE ops_requests SET state='completed',completed_at=now() WHERE id=$1;`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = e.DB.Exec(ctx, `UPDATE ops_mode SET paused=true,reason=$1 WHERE singleton`, "维护单 "+id); err != nil {
		t.Fatal(err)
	}
	if err = e.Resume(ctx, 1); err == nil {
		t.Fatal("unverified execution resumed")
	}
	if _, err = e.VerifyExecution(ctx, id); err == nil {
		t.Fatal("a manual finish without the shared commit receipt was accepted")
	}
	backup := newID()
	proof, _ := json.Marshal(protected)
	if _, err = e.DB.Exec(ctx, `INSERT INTO ops_backups(id,state)VALUES($1,'ready')`, backup); err != nil {
		t.Fatal(err)
	}
	if _, err = e.DB.Exec(ctx, `INSERT INTO ops_web_restores(id,backup_id,state,manifest_digest,protected,applied_backup,completed_at)VALUES($1,$2,'completed','fixture',$3,$2,now());`, id, backup, proof); err != nil {
		t.Fatal(err)
	}
	if _, err = e.DB.Exec(ctx, `INSERT INTO ops_restore_commit(request_id,backup_id)VALUES($1,$2)`, id, backup); err != nil {
		t.Fatal(err)
	}
	r, err := e.VerifyExecution(ctx, id)
	if err != nil || r.VerifiedAt == nil {
		t.Fatal(r, err)
	}
	// The proof is invalidated by subsequent protected business edits at resume.
	if _, err = e.DB.Exec(ctx, `UPDATE application_settings SET revision=revision+1 WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	if err = e.Resume(ctx, 1); err == nil {
		t.Fatal("changed protected data passed stale proof")
	}
	if _, err = e.VerifyExecution(ctx, id); err == nil {
		t.Fatal("changed protected data reverified")
	}
	if _, err = e.DB.Exec(ctx, `UPDATE application_settings SET revision=revision-1 WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	if err = e.Resume(ctx, 1); err != nil {
		t.Fatal(err)
	}
}

func TestJournalCapacityAlertRecoversIndependently(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	p := Defaults()
	p.Consecutive = 1
	if err := e.evaluate(ctx, p, map[string]any{"pluginJournalUsed": 100000}); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := e.DB.QueryRow(ctx, `SELECT state FROM ops_alerts WHERE code='plugin_journal'`).Scan(&state); err != nil || state != "active" {
		t.Fatal(state, err)
	}
	if err := e.evaluate(ctx, p, map[string]any{"pluginJournalUsed": 0}); err != nil {
		t.Fatal(err)
	}
	if err := e.DB.QueryRow(ctx, `SELECT state FROM ops_alerts WHERE code='plugin_journal'`).Scan(&state); err != nil || state != "normal" {
		t.Fatal(state, err)
	}
	var recovered int
	if err := e.DB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type='ops.alert.recovered'`).Scan(&recovered); err != nil || recovered != 1 {
		t.Fatal(recovered, err)
	}
}
