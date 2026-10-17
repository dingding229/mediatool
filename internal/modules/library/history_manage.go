package library

import (
	"context"
	"encoding/json"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	"time"
)

type HistorySelection struct {
	Kind      string    `json:"kind"`
	ID        string    `json:"id"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (e Engine) HideHistory(ctx context.Context, items []HistorySelection, confirm bool) error {
	if !confirm || len(items) == 0 || len(items) > 50 {
		return errors.New("confirm removal of 1–50 records")
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	seen := map[string]bool{}
	for _, item := range items {
		key := item.Kind + ":" + item.ID
		if seen[key] {
			return errors.New("duplicate history selection")
		}
		seen[key] = true
		table := ""
		switch item.Kind {
		case "file":
			table = "file_operations"
		case "batch":
			table = "file_batches"
		case "scan":
			table = "library_scans"
		case "transfer":
			table = "transfer_tasks"
		default:
			return errors.New("only ended organizing/scanning/transfer history can be removed")
		}
		var state string
		var updated time.Time
		if err = tx.QueryRow(ctx, `SELECT state,updated_at FROM `+table+` WHERE id=$1 FOR UPDATE`, item.ID).Scan(&state, &updated); err != nil {
			return err
		}
		if item.UpdatedAt.IsZero() || !updated.Equal(item.UpdatedAt) {
			return errors.New("history changed; refresh before removal")
		}
		allowed := state == "succeeded" || state == "cancelled" || state == "expired" || state == "partial" || state == "failed" || item.Kind == "transfer" && state == "imported"
		if !allowed {
			return errors.New("active, unknown or unreconciled records cannot be removed")
		}
		// Failed transfer/source reads may still require an external-outcome audit.
		if item.Kind == "transfer" && state != "imported" && state != "cancelled" {
			return errors.New("only finished imported/cancelled transfers may be hidden")
		}
		if item.Kind == "batch" {
			var unresolved bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_batch_items WHERE batch_id=$1 AND state IN ('running','needs_reconcile'))`, item.ID).Scan(&unresolved); err != nil {
				return err
			}
			if unresolved {
				return errors.New("reconcile batch before removal")
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO library_hidden_history(kind,record_id,observed_at)VALUES($1,$2,$3)ON CONFLICT(kind,record_id)DO UPDATE SET observed_at=excluded.observed_at,hidden_at=now()`, item.Kind, item.ID, updated); err != nil {
			return err
		}
	}
	details, _ := json.Marshal(map[string]any{"items": items, "filesystemDeleted": false, "recoveryEvidenceRetained": true})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','library.history_removed',$1)`, details); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
