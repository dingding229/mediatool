package v2

import (
	"context"
)

// Only the worker owning this generation may yield its own instance for human
// intervention. No new privilege or arbitrary state-write RPC is exposed.
// The finished run is durable before this CAS; no effect is automatically replayed.
func (s Store) pauseForAction(ctx context.Context, l Lease) error {
	defer invalidateNameHooks(s.DB)
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	tag, e := tx.Exec(ctx, `UPDATE plugin_v2_instances SET desired_state='disabled',runtime_state='needs_action',reason='',revision=revision+1,grant_revision=grant_revision+1,updated_at=now() WHERE id=$1 AND generation=$2 AND owner_id=$3 AND grant_revision=$4 AND desired_state='enabled' AND lease_until>clock_timestamp()`, l.ID, l.Token, l.Owner, l.GrantRevision)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('plugin','plugins.needs_action',jsonb_build_object('instanceId',$1::text))`, l.ID); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	s.logInfo(ctx, l.ID, "Plugin requires operator action", "result", "needs_action")
	return nil
}
