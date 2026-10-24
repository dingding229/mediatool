package onlineupdate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var errTerminalAudit = errors.New("更新终态审计待对账，将由 app 数据库连接重试")

// Both helper and app use this transaction: restart/reconciliation must not
// duplicate an audit, and no runtime configuration is part of the payload.
func auditTerminal(ctx context.Context, db *pgxpool.Pool, j Job, source string) error {
	if !j.Done() {
		return nil
	}
	if db == nil {
		return errTerminalAudit
	}
	b, e := json.Marshal(struct {
		Job
		AuditSource string `json:"auditSource"`
	}{j, source})
	if e != nil {
		return errTerminalAudit
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		return errTerminalAudit
	}
	defer func() {
		clean, done := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer done()
		_ = tx.Rollback(clean)
	}()
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "online_update:"+j.ID); e != nil {
		return errTerminalAudit
	}
	_, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)
 SELECT $1,$2,$3::jsonb WHERE NOT EXISTS
 (SELECT 1 FROM audit_logs WHERE action=$2 AND details->>'id'=$4)`, j.Actor, "online_update."+j.Phase, b, j.ID)
	if e != nil {
		return errTerminalAudit
	}
	if e = tx.Commit(ctx); e != nil {
		return errTerminalAudit
	}
	return nil
}

// Reconcile only after helper releases the execution lock. The app's own pool
// is the fallback even when inspect/plan/DSN is unavailable to the helper.
// Scan all jobs, not just latest: an older requested-only record must be repaired.
func (s *Service) reconcileAudits(ctx context.Context) error {
	if _, e := os.Stat(s.Root); os.IsNotExist(e) {
		return nil
	}
	ctx, done := context.WithTimeout(ctx, 10*time.Second)
	defer done()
	unlock, e := lock(s.Root, "execution")
	if errors.Is(e, ErrBusy) {
		return nil
	}
	if e != nil {
		return errTerminalAudit
	}
	defer unlock()
	files, e := os.ReadDir(s.Root)
	if e != nil {
		return errTerminalAudit
	}
	for _, f := range files {
		id := strings.TrimSuffix(f.Name(), ".json")
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") || !idPattern.MatchString(id) {
			continue
		}
		j, e := load(s.Root, id)
		if e != nil {
			return errTerminalAudit
		}
		if e = auditTerminal(ctx, s.DB, j, "reconciliation"); e != nil {
			return e
		}
	}
	return nil
}
