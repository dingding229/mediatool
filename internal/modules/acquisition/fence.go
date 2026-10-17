package acquisition

import (
	"context"
	"errors"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	"time"
)

var ErrUploadStopped = errors.New("upload control revision changed; worker stopped without restarting")

// Each mutating RPC and its durable callback are fenced by the same PG lock as
// operator controls. The stream itself is not locked, so pause remains usable.
func (e Engine) uploadFence(ctx context.Context, id string, revision int64, leases ...jobs.Job) (func(), error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,714))`, id); err != nil {
		deferutil.Rollback(ctx, tx)
		return nil, err
	}
	var current int64
	var state string
	if err = tx.QueryRow(ctx, `SELECT control_revision,state FROM transfer_tasks WHERE id=$1`, id).Scan(&current, &state); err != nil || current != revision || (state != "pending" && state != "starting" && state != "uploading") {
		deferutil.Rollback(ctx, tx)
		return nil, ErrUploadStopped
	}
	if len(leases) > 0 {
		var owned bool
		j := leases[0]
		if j.LeaseToken == "" {
			deferutil.Rollback(ctx, tx)
			return nil, jobs.ErrLeaseLost
		}
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1 AND lease_token=$2 AND state='running' AND lease_until>now())`, j.ID, j.LeaseToken).Scan(&owned); err != nil || !owned {
			deferutil.Rollback(ctx, tx)
			return nil, jobs.ErrLeaseLost
		}
	}
	return func() {
		bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		deferutil.Rollback(bounded, tx)
	}, nil
}
