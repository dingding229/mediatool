package messaging

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"time"
)

type DeliverySummary struct {
	ID, EventID, State string
	Attempts           int
	CreatedAt          time.Time
	NextAttemptAt      *time.Time
}

const ownedDelivery = ` FROM notification_deliveries d JOIN outbox_events o ON o.id=d.event_id WHERE o.event_type='plugin.notice'AND(o.aggregate_id::text=$1 OR o.payload->>'instanceId'=$1)`

func (d Dispatcher) PluginDeliveries(ctx context.Context, instance string, start, limit int) ([]DeliverySummary, error) {
	if start < 0 || limit < 1 || limit > 200 {
		return nil, errors.New("invalid notification page")
	}
	rows, e := d.DB.Query(ctx, `SELECT d.id::text,d.event_id::text,d.state,d.attempts,o.created_at,d.next_attempt_at`+ownedDelivery+` ORDER BY o.created_at DESC,d.id LIMIT $2 OFFSET $3`, instance, limit+1, start)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []DeliverySummary{}
	for rows.Next() {
		var v DeliverySummary
		if e = rows.Scan(&v.ID, &v.EventID, &v.State, &v.Attempts, &v.CreatedAt, &v.NextAttemptAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (d Dispatcher) PluginPublish(ctx context.Context, instance, operation, title, text string) (string, error) {
	tx, e := d.DB.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer deferutil.Rollback(ctx, tx)
	if e = outbox.EmitTx(ctx, tx, "plugin.notice", operation, map[string]string{"instanceId": instance, "title": title, "message": text}); e != nil {
		return "", e
	}
	var id string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM outbox_events WHERE event_type='plugin.notice'AND aggregate_id=$1`, operation).Scan(&id); e != nil {
		return "", e
	}
	return id, dbutil.Commit(ctx, tx)
}
func (d Dispatcher) PluginRetry(ctx context.Context, instance, id string, duplicate bool) error {
	// Ownership is rechecked while the existing retry flow holds its row lock.
	ctx = context.WithValue(ctx, pluginOwnerKey{}, instance)
	return d.Retry(ctx, id, duplicate)
}

type pluginOwnerKey struct{}

func pluginOwnership(ctx context.Context, tx pgx.Tx, id string) error {
	instance, ok := ctx.Value(pluginOwnerKey{}).(string)
	if !ok {
		return nil
	}
	var owned bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1`+ownedDelivery+` AND d.id=$2)`, instance, id).Scan(&owned)
	if e != nil {
		return e
	}
	if !owned {
		return pgx.ErrNoRows
	}
	return nil
}
