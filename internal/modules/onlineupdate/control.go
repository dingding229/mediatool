package onlineupdate

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

const recoveryPrefix = "online_update.recovery."

type Control struct {
	Recovery []string `json:"recoveryRequired"`
}

// Both the recovery latch and audit survive task-file removal. Old manual-recovery audit
// records also interlock the first startup after upgrading from the old runner.
func (s *Service) Control(ctx context.Context) (Control, error) {
	c := Control{Recovery: []string{}}
	if s.DB == nil {
		return c, ErrUnsafe
	}
	rows, e := s.DB.Query(ctx, `SELECT DISTINCT id FROM (
 SELECT substring(key from length($1)+1) id FROM app_settings WHERE left(key,length($1))=$1 AND value='true'::jsonb
 UNION SELECT details->>'id' FROM audit_logs a WHERE action='online_update.manual_recovery' AND NOT EXISTS
 (SELECT 1 FROM audit_logs b WHERE b.action='online_update.recovery_confirmed' AND b.details->>'id'=a.details->>'id')) q ORDER BY id`, recoveryPrefix)
	if e != nil {
		return c, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return c, e
		}
		if !idPattern.MatchString(id) {
			return c, ErrUnsafe
		}
		c.Recovery = append(c.Recovery, id)
	}
	return c, rows.Err()
}

// Caller must have reauthenticated the administrator. State and its audit are
// committed together; no endpoint implicitly acknowledges a recovery.
func (s *Service) SetControl(ctx context.Context, actor, action, id string) error {
	unlock, e := lock(s.Root, "admission")
	if e != nil {
		return e
	}
	defer unlock()
	if e = s.reconcileAudits(ctx); e != nil {
		return e
	}
	executionUnlock, e := lock(s.Root, "execution")
	if e != nil {
		return e
	}
	defer executionUnlock()
	c, e := s.Control(ctx)
	if e != nil {
		return e
	}
	if action != "confirm_recovery" {
		return ErrUnsafe
	}
	found := false
	for _, v := range c.Recovery {
		if v == id {
			found = true
		}
	}
	if !found || !idPattern.MatchString(id) {
		return ErrUnsafe
	}
	key, value, auditAction := recoveryPrefix+id, false, "online_update.recovery_confirmed"
	details := map[string]any{"id": id}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, e = tx.Exec(ctx, `INSERT INTO app_settings(key,value)VALUES($1,$2::jsonb) ON CONFLICT(key)DO UPDATE SET value=excluded.value,updated_at=now()`, key, value); e != nil {
		return e
	}
	b, _ := json.Marshal(details)
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,$2,$3::jsonb)`, actor, auditAction, b); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func latchRecovery(ctx context.Context, tx pgx.Tx, j Job) error {
	if j.Phase != "manual_recovery" {
		return nil
	}
	_, e := tx.Exec(ctx, `INSERT INTO app_settings(key,value) SELECT $1,'true'::jsonb WHERE NOT EXISTS
 (SELECT 1 FROM audit_logs WHERE action='online_update.recovery_confirmed' AND details->>'id'=$2)
 ON CONFLICT(key)DO NOTHING`, recoveryPrefix+strings.TrimSpace(j.ID), j.ID)
	return e
}
