package acquisition

import (
	"context"
	"github.com/jackc/pgx/v5/pgconn"
)

type subscriptionTaskExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Called only inside task creation's transaction. A concurrent/manual task can
// be adopted when unbound, but another subscription's task is never reparented.
func assignDownloadSubscription(ctx context.Context, tx subscriptionTaskExecer, taskID, subscriptionID string) error {
	tag, err := tx.Exec(ctx, `UPDATE download_tasks SET subscription_id=$2 WHERE id=$1 AND (subscription_id IS NULL OR subscription_id=$2)`, taskID, subscriptionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrDownloadIdentityConflict
	}
	return nil
}
