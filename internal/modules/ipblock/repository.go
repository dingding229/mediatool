package ipblock

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	"time"
)

type Store interface {
	Load(context.Context) (Policy, error)
	Change(context.Context, Mutation) (Policy, error)
	AddHits(context.Context, string, map[string]Delta) error
}
type Repository struct{ DB *pgxpool.Pool }

func read(ctx context.Context, tx pgx.Tx) (Policy, error) {
	p := Policy{Rules: []Rule{}}
	if e := tx.QueryRow(ctx, `SELECT revision FROM player_ip_block_policy WHERE singleton`).Scan(&p.Revision); e != nil {
		return p, e
	}
	rows, e := tx.Query(ctx, `SELECT id::text,network::text,note,created_at,updated_at,hit_count,last_hit_at FROM player_ip_block_rules ORDER BY created_at DESC,id LIMIT 10001`)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		var r Rule
		if e = rows.Scan(&r.ID, &r.Network, &r.Note, &r.CreatedAt, &r.UpdatedAt, &r.HitCount, &r.LastHitAt); e != nil {
			return p, e
		}
		p.Rules = append(p.Rules, r)
	}
	if len(p.Rules) > MaxRules {
		return p, ErrLimit
	}
	return p, rows.Err()
}
func (r Repository) Load(ctx context.Context) (Policy, error) {
	if r.DB == nil {
		return Policy{}, ErrUnavailable
	}
	tx, e := r.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return Policy{}, e
	}
	defer deferutil.Rollback(ctx, tx)
	p, e := read(ctx, tx)
	if e == nil {
		e = tx.Commit(ctx)
	}
	return p, e
}
func (r Repository) Change(ctx context.Context, m Mutation) (Policy, error) {
	if e := m.Validate(); e != nil {
		return Policy{}, e
	}
	if r.DB == nil {
		return Policy{}, ErrUnavailable
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return Policy{}, e
	}
	defer deferutil.Rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SET LOCAL lock_timeout='1s'`); e != nil {
		return Policy{}, e
	}
	var rev int64
	if e = tx.QueryRow(ctx, `SELECT revision FROM player_ip_block_policy WHERE singleton FOR UPDATE`).Scan(&rev); e != nil {
		return Policy{}, e
	}
	if rev != m.Revision {
		return Policy{}, ErrConflict
	}
	if m.Delete {
		e := tx.QueryRow(ctx, `DELETE FROM player_ip_block_rules WHERE id=$1 RETURNING network::text,note`, m.ID).Scan(&m.Network, &m.Note)
		if errors.Is(e, pgx.ErrNoRows) {
			return Policy{}, ErrNotFound
		}
		if e != nil {
			return Policy{}, e
		}
	} else {
		var duplicate bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM player_ip_block_rules WHERE network=$1::cidr AND id::text<>$2)`, m.Network, m.ID).Scan(&duplicate); e != nil {
			return Policy{}, e
		}
		if duplicate {
			return Policy{}, ErrDuplicate
		}
		if m.ID == "" {
			var n int
			if e = tx.QueryRow(ctx, `SELECT count(*) FROM player_ip_block_rules`).Scan(&n); e != nil {
				return Policy{}, e
			}
			if n >= MaxRules {
				return Policy{}, ErrLimit
			}
			m.ID = jobs.UUID()
			_, e = tx.Exec(ctx, `INSERT INTO player_ip_block_rules(id,network,note) VALUES($1,$2::cidr,$3)`, m.ID, m.Network, m.Note)
		} else {
			t, err := tx.Exec(ctx, `UPDATE player_ip_block_rules SET network=$2::cidr,note=$3,updated_at=now() WHERE id=$1`, m.ID, m.Network, m.Note)
			e = err
			if e == nil && t.RowsAffected() == 0 {
				e = ErrNotFound
			}
		}
		if e != nil {
			return Policy{}, e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE player_ip_block_policy SET revision=revision+1 WHERE singleton`); e != nil {
		return Policy{}, e
	}
	details, _ := json.Marshal(map[string]any{"id": m.ID, "network": m.Network, "note": m.Note, "delete": m.Delete, "confirmSelf": m.ConfirmSelf, "revision": m.Revision})
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details) VALUES('administrator','player_ip.rules_changed',$1)`, details); e != nil {
		return Policy{}, e
	}
	p, e := read(ctx, tx)
	if e == nil {
		e = tx.Commit(ctx)
		if e != nil && !errors.Is(e, pgx.ErrTxCommitRollback) {
			e = ErrCommitUnknown
		}
	}
	return p, e
}

// A batch updates existing IDs only: deleting a rule can never be undone by a late flush.
func (r Repository) AddHits(ctx context.Context, batchID string, d map[string]Delta) error {
	if len(d) == 0 {
		return nil
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	// One serialized in-flight batch retains its ID until acknowledged. An ambiguous
	// commit can be retried without counting the same requests twice.
	tag, e := tx.Exec(ctx, `UPDATE player_ip_block_policy SET last_counter_batch=$1 WHERE singleton AND last_counter_batch IS DISTINCT FROM $1::uuid`, batchID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		var existing string
		if e = tx.QueryRow(ctx, `SELECT COALESCE(last_counter_batch::text,'') FROM player_ip_block_policy WHERE singleton`).Scan(&existing); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	ids := make([]string, 0, len(d))
	counts := make([]int64, 0, len(d))
	times := make([]time.Time, 0, len(d))
	for id, v := range d {
		ids = append(ids, id)
		counts = append(counts, v.Count)
		times = append(times, v.Last)
	}
	_, e = tx.Exec(ctx, `UPDATE player_ip_block_rules r SET hit_count=r.hit_count+d.count,last_hit_at=GREATEST(r.last_hit_at,d.last) FROM unnest($1::uuid[],$2::bigint[],$3::timestamptz[]) d(id,count,last) WHERE r.id=d.id`, ids, counts, times)
	if e == nil {
		e = tx.Commit(ctx)
	}
	return e
}
