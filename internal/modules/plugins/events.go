package plugins

import (
	"context"
	"encoding/json"
	"time"
)

const (
	extensionDeliveryPending   = "pending"
	extensionDeliverySucceeded = "succeeded"
	extensionDeliveryUnknown   = "unknown"
)

func (r Registry) DispatchEvents(ctx context.Context) error {
	list, e := r.List(ctx)
	if e != nil {
		return e
	}
	for _, p := range list {
		if !p.Enabled || len(p.Manifest.Events) == 0 {
			continue
		}
		// A pending row means the process claimed the event but may have stopped
		// before or during invocation. It is no longer safe to replay blindly;
		// converge stale claims to the explicit unknown state instead of leaving
		// them permanently invisible to the dispatcher.
		if _, e = r.DB.Exec(ctx, `UPDATE extension_deliveries SET state='unknown',message='事件投递中断，结果未知；不自动重复发送',updated_at=now() WHERE plugin_id=$1 AND state='pending' AND updated_at<now()-interval '2 minutes'`, p.Manifest.ID); e != nil {
			return e
		}
		rows, e := r.DB.Query(ctx, `SELECT e.id::text,e.event_type,e.payload FROM outbox_events e WHERE e.event_type=ANY($1)AND NOT EXISTS(SELECT 1 FROM extension_deliveries d WHERE d.event_id=e.id AND d.plugin_id=$2)AND e.created_at>(SELECT updated_at FROM extension_plugins WHERE id=$2)ORDER BY e.created_at LIMIT 10`, p.Manifest.Events, p.Manifest.ID)
		if e != nil {
			return e
		}
		type event struct {
			id, kind string
			raw      []byte
		}
		events := []event{}
		for rows.Next() {
			var x event
			if rows.Scan(&x.id, &x.kind, &x.raw) != nil {
				rows.Close()
				return ErrInvalid
			}
			events = append(events, x)
		}
		rowErr := rows.Err()
		rows.Close()
		if rowErr != nil {
			return rowErr
		}
		for _, x := range events {
			tag, e := r.DB.Exec(ctx, `INSERT INTO extension_deliveries(event_id,plugin_id,state)VALUES($1,$2,'pending')ON CONFLICT DO NOTHING`, x.id, p.Manifest.ID)
			if e != nil {
				return e
			}
			if tag.RowsAffected() == 0 {
				continue
			}
			data := map[string]any{}
			if json.Unmarshal(x.raw, &data) != nil || data == nil {
				return ErrInvalid
			}
			data["eventId"] = x.id
			_, e = r.Invoke(ctx, p.Manifest.ID, x.kind, data, false)
			state := extensionDeliverySucceeded
			message := "事件执行完成"
			if e != nil {
				state = extensionDeliveryUnknown
				message = "事件执行结果未知，不自动重试"
			}
			finish, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			_, finishErr := r.DB.Exec(finish, `UPDATE extension_deliveries SET state=$3,message=$4,updated_at=now()WHERE event_id=$1 AND plugin_id=$2 AND state='pending'`, x.id, p.Manifest.ID, state, message)
			stop()
			if finishErr != nil {
				return finishErr
			}
		}
	}
	return nil
}
