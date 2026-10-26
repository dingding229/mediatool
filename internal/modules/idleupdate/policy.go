package idleupdate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"media-tool/internal/modules/accounts"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/outbox"
)

func newID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}

// Save is called only after HTTP password reauthentication for every enabled save.
// Neither the password nor a login token is part of the stored grant or audit.
func (s *Service) Save(ctx context.Context, v SaveRequest, a accounts.Account) (Policy, error) {
	if e := v.Validate(); e != nil {
		return Policy{}, e
	}
	s.grantMu.Lock()
	defer s.grantMu.Unlock()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return Policy{}, e
	}
	defer deferutil.Rollback(ctx, tx)
	grant := ""
	if v.Enabled {
		grant = newID()
	}
	tag, e := tx.Exec(ctx, `UPDATE idle_update_policy p SET enabled=$1,start_time=$2,end_time=$3,quiet_minutes=$4,revision=revision+1,grant_id=$5,account_id=$6,credential_version=$7,granted_at=now(),reason='',updated_at=now() WHERE singleton AND revision=$8 AND EXISTS(SELECT 1 FROM administrator_accounts a WHERE a.id=$6 AND a.credential_version=$7 AND NOT a.must_change_password AND a.temporary_password_expires_at IS NULL)`, v.Enabled, v.StartTime, v.EndTime, v.QuietMinutes, grant, a.ID, a.Version, v.Revision)
	if e != nil {
		return Policy{}, e
	}
	if tag.RowsAffected() != 1 {
		return Policy{}, ErrConflict
	}
	details, _ := json.Marshal(map[string]any{"enabled": v.Enabled, "startTime": v.StartTime, "endTime": v.EndTime, "quietMinutes": v.QuietMinutes, "grantId": grant, "accountId": a.ID})
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,'idle_update.configured',$2)`, a.Username, details); e != nil {
		return Policy{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Policy{}, e
	}
	return s.Get(ctx)
}
func (s *Service) disable(ctx context.Context, grant, reason string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	tag, e := tx.Exec(ctx, `UPDATE idle_update_policy SET enabled=false,reason=$2,revision=revision+1,updated_at=now() WHERE singleton AND enabled AND ($1='' OR grant_id=$1)`, grant, reason)
	if e != nil {
		return e
	}
	if tag.RowsAffected() > 0 {
		raw, _ := json.Marshal(map[string]string{"grantId": grant, "message": reason})
		if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('system:idle-update','idle_update.disabled',$1)`, raw); e != nil {
			return e
		}
		if e = outbox.EmitVersionTx(ctx, tx, "idle_update.disabled", grant, "disabled", map[string]string{"title": "闲时自动更新已关闭", "message": reason + "；请核对结果并重新验密授权"}); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
