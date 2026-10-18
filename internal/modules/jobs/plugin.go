package jobs

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"time"
)

var ErrUnsupportedKind = errors.New("job kind not safely cancellable")

type Owned struct {
	ID, Kind, State        string
	Attempts               int
	CreatedAt, AvailableAt time.Time
	Cancelable             bool
}

// Only F2's explicitly owned manual service wakes have complete cancellation
// evidence. No arbitrary kind/payload, worker owner string, Claim or Finish.
const ownedPluginJob = ` FROM jobs j WHERE j.payload->>'instanceId'=$1 AND EXISTS(SELECT 1 FROM plugin_v2_manual_requests m WHERE m.job_id=j.id AND m.instance_id::text=$1)`

func (r Repository) PluginPage(ctx context.Context, instance string, start, limit int, kind, state, id string) ([]Owned, error) {
	if start < 0 || limit < 1 || limit > 200 {
		return nil, ErrOperatorRequired
	}
	rows, e := r.DB.Query(ctx, `SELECT j.id::text,j.kind,j.state,j.attempts,j.created_at,j.available_at,j.kind='plugin.v2.service'AND j.state IN('pending','retry','paused')`+ownedPluginJob+` AND($2=''OR j.kind=$2)AND($3=''OR j.state=$3)AND($4=''OR j.id::text=$4)ORDER BY j.created_at DESC,j.id LIMIT $5 OFFSET $6`, instance, kind, state, id, limit+1, start)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Owned{}
	for rows.Next() {
		var v Owned
		if e = rows.Scan(&v.ID, &v.Kind, &v.State, &v.Attempts, &v.CreatedAt, &v.AvailableAt, &v.Cancelable); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r Repository) PluginCancel(ctx context.Context, instance, id string) error {
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	var kind, state string
	e = tx.QueryRow(ctx, `SELECT j.kind,j.state`+ownedPluginJob+` AND j.id=$2 FOR UPDATE OF j`, instance, id).Scan(&kind, &state)
	if e != nil {
		return e
	}
	if kind != "plugin.v2.service" {
		return ErrUnsupportedKind
	}
	if state == "cancelled" {
		return dbutil.Commit(ctx, tx)
	}
	if state != "pending" && state != "retry" && state != "paused" {
		return ErrOperatorRequired
	}
	// A worker may already have made a durable run before its wake was retried.
	var runState string
	e = tx.QueryRow(ctx, `SELECT state FROM plugin_v2_runs WHERE instance_id::text=$1 AND kind='service'AND dedupe_key='service-job:'||$2 FOR UPDATE`, instance, id).Scan(&runState)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if runState != "" && runState != "pending" && runState != "canceled" {
		return ErrOperatorRequired
	}
	if _, e = tx.Exec(ctx, `UPDATE plugin_v2_runs SET state='canceled',reason='revoked',finished_at=clock_timestamp()WHERE instance_id::text=$1 AND dedupe_key='service-job:'||$2 AND state='pending'`, instance, id); e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `UPDATE jobs SET state='cancelled',last_error=NULL,lease_token=NULL,lease_until=NULL,locked_by=NULL,updated_at=now()WHERE id=$1 AND state IN('pending','retry','paused')`, id)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return dbutil.Commit(ctx, tx)
}
