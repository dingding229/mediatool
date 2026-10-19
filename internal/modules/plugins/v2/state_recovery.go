package v2

import (
	"context"
	"errors"

	"os"
	"strings"
)

// RecoverState runs before supervisor claims. No unfinished exchange permits an
// enable: data_operation remains set until filesystem and PG agree again.
func (s Store) RecoverState(ctx context.Context) error {
	rows, e := s.DB.Query(ctx, `SELECT id::text FROM plugin_v2_state_tasks WHERE (state IN('writing','prepared') AND lease_until<=clock_timestamp()) OR (state IN('done','failed') AND NOT cleaned) ORDER BY created_at,id LIMIT 32`)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	for _, id := range ids {
		if e = s.recoverStateTask(ctx, id); e != nil {
			return e
		}
		if e = s.cleanStateTask(ctx, id); e != nil {
			return e
		}
	}
	_, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_state_tasks WHERE id IN(SELECT id FROM plugin_v2_state_tasks WHERE cleaned AND state IN('done','failed') AND created_at<now()-interval '7 days' ORDER BY created_at LIMIT 100)`)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(ctx, `DELETE FROM plugin_v2_state_snapshots WHERE id IN(SELECT s.id FROM plugin_v2_state_snapshots s WHERE state IN('failed','deleted') AND created_at<now()-interval '7 days' AND NOT EXISTS(SELECT 1 FROM plugin_v2_state_tasks t WHERE t.snapshot_id=s.id) ORDER BY created_at LIMIT 100)`)
	return e
}
func (s Store) recoverStateTask(ctx context.Context, task string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	var id, snapshot, kind, state, ino string
	var expired bool
	e = tx.QueryRow(ctx, `SELECT t.instance_id::text,t.snapshot_id::text,t.kind,t.state,t.stage_identity,t.lease_until<=clock_timestamp() FROM plugin_v2_state_tasks t JOIN plugin_v2_instances i ON i.id=t.instance_id WHERE t.id=$1 FOR UPDATE OF i,t`, task).Scan(&id, &snapshot, &kind, &state, &ino, &expired)
	if e != nil {
		return e
	}
	if state == "done" || state == "failed" {
		return nil
	}
	if !expired {
		return nil
	}
	r, e := s.stateRoot(id)
	if e != nil {
		return e
	}
	defer closeState(r)
	if kind == "restore" && state == "prepared" {
		current, e := identity(r, "data")
		if e != nil {
			return e
		}
		stage := "snapshots/" + task + "/stage"
		expected := strings.Split(ino, "|")
		if len(expected) != 2 {
			return ErrInvalid
		}
		staged, e := identity(r, stage)
		if e != nil {
			return e
		}
		if current == expected[1] && staged == expected[0] {
			if e = exchangeState(r, "data", stage); e != nil {
				return e
			}
			a, e := directory(r, "snapshots/"+task)
			if e != nil {
				return e
			}
			e = syncRoot(a)
			closeState(a)
			if e != nil {
				return e
			}
		} else if current != expected[0] || staged != expected[1] {
			return ErrConflict
		}

	}
	if kind == "create" {
		if _, e = tx.Exec(ctx, `UPDATE plugin_v2_state_tasks SET reserved_bytes=$2 WHERE id=$1`, task, stateReserve); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE plugin_v2_state_snapshots SET state='failed',bytes=0 WHERE id=$1 AND state='writing'`, snapshot); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `DELETE FROM plugin_v2_state_values WHERE snapshot_id=$1`, snapshot); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE plugin_v2_state_tasks SET state='failed' WHERE id=$1`, task); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET data_operation=NULL WHERE id=$1 AND data_operation=$2`, id, task); e != nil {
		return e
	}
	if e = stateAudit(ctx, tx, "system", "recovered", id, snapshot); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s Store) cleanStateTask(ctx context.Context, task string) error {
	var id, state, kind, path string
	var cleaned bool
	e := s.DB.QueryRow(ctx, `SELECT t.instance_id::text,t.state,t.kind,s.data_path,t.cleaned FROM plugin_v2_state_tasks t JOIN plugin_v2_state_snapshots s ON s.id=t.snapshot_id WHERE t.id=$1`, task).Scan(&id, &state, &kind, &path, &cleaned)
	if e != nil {
		return e
	}
	if cleaned || (state != "done" && state != "failed") {
		return nil
	}
	r, e := s.stateRoot(id)
	if e != nil {
		return e
	}
	defer closeState(r)
	snap, e := directory(r, "snapshots")
	if errors.Is(e, os.ErrNotExist) {
		_, e = s.DB.Exec(ctx, `UPDATE plugin_v2_state_tasks SET cleaned=true WHERE id=$1 AND state IN('done','failed')`, task)
		return e
	}
	if e != nil {
		return e
	}
	defer closeState(snap)
	remove := task
	if kind == "create" && state == "done" {
		remove = ""
	}
	if kind == "delete" && state == "done" {
		p := strings.Split(path, "/")
		if len(p) != 3 || !validUUID(p[1]) {
			return ErrInvalid
		}
		if e = snap.RemoveAll(p[1]); e != nil {
			return e
		}
	}
	if remove != "" {
		if !validUUID(remove) {
			return ErrInvalid
		}
		if e = snap.RemoveAll(remove); e != nil {
			return e
		}
	}
	if e = syncRoot(snap); e != nil {
		return e
	}
	_, e = s.DB.Exec(ctx, `UPDATE plugin_v2_state_tasks SET cleaned=true WHERE id=$1 AND state IN('done','failed')`, task)
	return e
}
