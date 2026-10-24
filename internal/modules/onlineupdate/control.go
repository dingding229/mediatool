package onlineupdate

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

var ErrDisabled = errors.New("在线更新执行开关关闭")

const executionKey = "online_update.execution"
const recoveryPrefix = "online_update.recovery."

type Control struct {
	Enabled  bool     `json:"executionEnabled"`
	Recovery []string `json:"recoveryRequired"`
}

// Both the latch and audit survive task-file removal. Old manual-recovery audit
// records also interlock the first startup after upgrading from the old runner.
func (s *Service) Control(ctx context.Context) (Control, error) {
	c := Control{Recovery: []string{}}
	if s.DB == nil {
		return c, ErrUnsafe
	}
	var enabled bool
	e := s.DB.QueryRow(ctx, `SELECT COALESCE((SELECT value='true'::jsonb FROM app_settings WHERE key=$1),false)`, executionKey).Scan(&enabled)
	if e != nil {
		return c, e
	}
	c.Enabled = enabled
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
	key, value, auditAction := executionKey, false, "online_update.execution_disabled"
	details := map[string]any{}
	switch action {
	case "enable":
		if len(c.Recovery) > 0 {
			return ErrBusy
		}
		value = true
		auditAction = "online_update.execution_enabled"
	case "disable":
	case "confirm_recovery":
		found := false
		for _, v := range c.Recovery {
			if v == id {
				found = true
			}
		}
		if !found || !idPattern.MatchString(id) {
			return ErrUnsafe
		}
		key = recoveryPrefix + id
		auditAction = "online_update.recovery_confirmed"
		details["id"] = id
	default:
		return ErrUnsafe
	}
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
