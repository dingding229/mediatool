package library

import (
	"context"
	"errors"
	"log/slog"
	"media-tool/internal/modules/maintenance"
	deferutil "media-tool/internal/platform/cleanup"
	"path"
	"strings"
	"time"

	"media-tool/internal/adapters/clouddrive2"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/storage"
)

// changeTouches checks both sides of a rename and ancestors of a configured
// subtree. No event is used as an authoritative deletion or identity assertion.
func changeTouches(root string, change clouddrive2.FileChange) bool {
	for _, p := range []string{change.Path, change.NewPath} {
		if p != "" && path.Clean(p) == p && strings.HasPrefix(p, "/") && !strings.ContainsAny(p, "\\\x00\r\n") && (inside(root, p) || inside(p, root)) {
			return true
		}
	}
	return false
}

// recordChange coalesces into one durable dirty bit per explicit scan scope.
// First event waits ten seconds; later events cannot postpone it indefinitely.
// Updates use a current connection revision and reject stopped/edited scopes.
func (e Engine) recordChange(ctx context.Context, connection string, revision int64, hint *clouddrive2.FileChange) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	rows, err := tx.Query(ctx, `SELECT s.id::text,s.root,coalesce(i.settings->>'cloudRoot','') FROM library_scan_schedules s JOIN integrations i ON i.id=s.integration_id WHERE s.integration_id=$1 AND s.enabled AND s.watch_changes AND i.enabled AND i.kind='clouddrive2' AND i.revision=$2 FOR UPDATE OF s`, connection, revision)
	if err != nil {
		return err
	}
	type scope struct{ id, root, allowed string }
	list := []scope{}
	for rows.Next() {
		var s scope
		if err = rows.Scan(&s.id, &s.root, &s.allowed); err != nil {
			rows.Close()
			return err
		}
		list = append(list, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, s := range list {
		if s.allowed == "" && len(e.Connections.Config.STRMAllowedRoots) > 0 {
			s.allowed = e.Connections.Config.STRMAllowedRoots[0]
		}
		if s.allowed == "" || !inside(s.allowed, s.root) {
			continue
		}
		if hint != nil && !changeTouches(s.root, *hint) {
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE library_scan_schedules SET change_scan_after=coalesce(change_scan_after,now()+interval '10 seconds'),last_change_at=CASE WHEN $2 THEN now() ELSE last_change_at END,updated_at=now() WHERE id=$1`, s.id, hint != nil); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (e Engine) watchStatus(ctx context.Context, connection string, revision int64, state, message string) error {
	_, err := e.DB.Exec(ctx, `UPDATE library_scan_schedules s SET watch_state=$3,watch_message=$4,updated_at=now() FROM integrations i WHERE i.id=s.integration_id AND s.integration_id=$1 AND i.enabled AND i.revision=$2 AND s.enabled AND s.watch_changes`, connection, revision, state, message)
	return err
}

// WatchWindow performs no RPC unless an enabled schedule explicitly opts in.
// It holds no database lock across stream reads. Adapter leases/timeout bound
// old credentials to one minute; each callback rechecks the current revision.
func (e Engine) WatchWindow(ctx context.Context) error {
	release, gateErr := maintenance.BeginWork(ctx, e.DB)
	if gateErr != nil {
		return nil
	}
	defer release()
	if e.DB == nil || e.Connections == nil {
		return nil
	}
	c, err := e.Connections.Repository.Default(ctx, "clouddrive2")
	if errors.Is(err, integrations.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var enabled bool
	if err = e.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM library_scan_schedules WHERE integration_id=$1 AND enabled AND watch_changes)`, c.Record.ID).Scan(&enabled); err != nil || !enabled {
		return err
	}
	client, err := e.Connections.CloudByID(ctx, c.Record.ID)
	if err != nil {
		return err
	}
	if err = e.watchStatus(ctx, c.Record.ID, c.Record.Revision, "connecting", ""); err != nil {
		return err
	}
	ready := func() error {
		// Protocol has no replay cursor: every reconnect schedules reconciliation.
		// This is a persisted hint, not a new scan on every PushMessage.
		bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := e.recordChange(bounded, c.Record.ID, c.Record.Revision, nil); err != nil {
			return err
		}
		return e.watchStatus(bounded, c.Record.ID, c.Record.Revision, "connected", "")
	}
	visit := func(hint clouddrive2.FileChange) error {
		bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		return e.recordChange(bounded, c.Record.ID, c.Record.Revision, &hint)
	}
	err = client.ObserveChanges(ctx, ready, visit)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	state, message := "waiting", "推送连接结束，将重连并对账；周期扫描仍保留"
	if errors.Is(err, storage.ErrUnsupported) {
		state, message = "unsupported", "当前CloudDrive2未提供PushMessage；继续周期对账"
	} else if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		state, message = "unavailable", "变化推送中断，将重连；不会据此确认文件缺失"
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if saveErr := e.watchStatus(bounded, c.Record.ID, c.Record.Revision, state, message); saveErr != nil {
		return saveErr
	}
	return err
}

// RunChangeWatcher backoff is finite and cancellation-aware. This loop runs
// only in the default coordinator worker; the library worker handles scans.
func (e Engine) RunChangeWatcher(ctx context.Context, logger *slog.Logger) {
	for ctx.Err() == nil {
		err := e.WatchWindow(ctx)
		if ctx.Err() != nil {
			return
		}
		wait := 15 * time.Second
		if errors.Is(err, storage.ErrUnsupported) {
			wait = 5 * time.Minute
		} else if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			wait = time.Minute
			if logger != nil {
				logger.Warn("cloud change stream unavailable; periodic reconciliation retained")
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
