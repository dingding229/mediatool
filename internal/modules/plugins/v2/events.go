package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/dingding229/mediatool-plugin/sdk/events"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s Store) publishSubscriptions(ctx context.Context, tx pgx.Tx, l Lease, snapshot string, metadata contracts.Snapshot) error {
	for _, d := range metadata.Descriptors {
		if d.Kind != "event" {
			continue
		}
		_, e := tx.Exec(ctx, `INSERT INTO plugin_v2_subscriptions(instance_id,handler,event_type,snapshot_id,cursor)VALUES($1,$2,$3,$4,coalesce((SELECT max(cursor) FROM plugin_v2_event_journal),0)) ON CONFLICT(instance_id,handler)DO UPDATE SET snapshot_id=excluded.snapshot_id,source_since=CASE WHEN (SELECT grant_revision FROM plugin_v2_snapshots WHERE id=plugin_v2_subscriptions.snapshot_id)=$5 THEN plugin_v2_subscriptions.source_since ELSE clock_timestamp() END,cursor=CASE WHEN (SELECT grant_revision FROM plugin_v2_snapshots WHERE id=plugin_v2_subscriptions.snapshot_id)=$5 THEN plugin_v2_subscriptions.cursor ELSE excluded.cursor END`, l.ID, d.Handler, d.ID, snapshot, l.GrantRevision)
		if e != nil {
			return e
		}
	}
	return nil
}
func (s Store) Bridge(ctx context.Context) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7063001)`); e != nil {
		return e
	}
	var stored int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_event_journal`).Scan(&stored); e != nil {
		return e
	}
	if stored >= 100000 {
		return contracts.NewError(contracts.Busy)
	}
	// NOT EXISTS avoids a commit-order watermark bug: late transactions are still
	// ingested. The separate durable journal cursor is assigned under this lock.
	_, e = tx.Exec(ctx, `INSERT INTO plugin_v2_event_journal(event_id,event_type,payload,created_at) SELECT o.id,CASE o.event_type WHEN 'files.succeeded' THEN 'library.organized' WHEN 'download.started' THEN 'download.added' WHEN 'files.failed' THEN 'library.organize.failed' ELSE o.event_type END,CASE WHEN o.event_type='download.started' THEN jsonb_build_object('taskId',o.aggregate_id::text,'state','downloading') WHEN o.event_type IN('site.account.updated','site.account.deleted') THEN jsonb_build_object('accountId',o.payload->'accountId','revision',o.payload->'revision','enabled',o.payload->'enabled') WHEN o.event_type='site.metrics.refreshed' THEN jsonb_build_object('accountId',o.payload->'accountId','accountRevision',o.payload->'accountRevision','state',o.payload->'state','attemptedAt',o.payload->'attemptedAt') WHEN o.event_type IN('subscription.created','subscription.updated','subscription.deleted') THEN jsonb_build_object('subscriptionId',o.payload->'subscriptionId','revision',o.payload->'revision','enabled',o.payload->'enabled','mediaType',o.payload->'mediaType','tmdbId',o.payload->'tmdbId') ELSE jsonb_build_object('operationId',o.aggregate_id::text,'state',CASE o.event_type WHEN 'files.succeeded' THEN 'succeeded' ELSE 'failed' END) END,o.created_at FROM outbox_events o WHERE o.event_type IN('files.succeeded','files.failed','download.started','site.account.updated','site.account.deleted','site.metrics.refreshed','subscription.created','subscription.updated','subscription.deleted') AND NOT EXISTS(SELECT 1 FROM plugin_v2_event_journal j WHERE j.event_id=o.id) ORDER BY o.created_at,o.id LIMIT $1 ON CONFLICT(event_id)DO NOTHING`, min(200, 100000-stored))
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s Store) Deliver(ctx context.Context) error {
	rows, e := s.DB.Query(ctx, `SELECT p.instance_id::text,p.handler,p.event_type,p.cursor,p.source_since FROM plugin_v2_subscriptions p JOIN plugin_v2_extensions x ON x.snapshot_id=p.snapshot_id JOIN plugin_v2_instances i ON i.id=p.instance_id JOIN plugin_v2_snapshots s ON s.id=p.snapshot_id WHERE i.desired_state='enabled' AND i.runtime_state='running' AND i.generation=s.generation AND i.grant_revision=s.grant_revision ORDER BY p.polled_at,p.instance_id,p.handler LIMIT 64`)
	if e != nil {
		return e
	}
	type sub struct {
		id, handler, typ string
		cursor           int64
		since            time.Time
	}
	list := []sub{}
	for rows.Next() {
		var v sub
		if e = rows.Scan(&v.id, &v.handler, &v.typ, &v.cursor, &v.since); e != nil {
			break
		}
		list = append(list, v)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	for _, p := range list {
		if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_subscriptions SET polled_at=clock_timestamp() WHERE instance_id=$1 AND handler=$2`, p.id, p.handler); e != nil {
			return e
		}
		snap, d, e := s.Descriptor(ctx, p.id, "event", p.handler)
		if e != nil {
			continue
		}
		eventsRows, e := s.DB.Query(ctx, `SELECT cursor,event_id::text,event_type,payload,created_at FROM plugin_v2_event_journal WHERE cursor>$1 ORDER BY cursor LIMIT 100`, p.cursor)
		if e != nil {
			return e
		}
		type entry struct {
			cursor  int64
			id, typ string
			payload json.RawMessage
			created time.Time
		}
		entries := []entry{}
		for eventsRows.Next() {
			var v entry
			if e = eventsRows.Scan(&v.cursor, &v.id, &v.typ, &v.payload, &v.created); e != nil {
				break
			}
			entries = append(entries, v)
		}
		other = eventsRows.Err()
		eventsRows.Close()
		if e != nil {
			return e
		}
		if other != nil {
			return other
		}
		for _, event := range entries {
			if event.typ == p.typ && !event.created.Before(p.since) {
				if _, e = s.eventRun(ctx, snap, d, event.cursor, event.id, event.payload); e != nil {
					var rpc *contracts.Error
					if errors.As(e, &rpc) && rpc.Code == contracts.Busy {
						break
					}
					if errors.Is(e, ErrConflict) {
						break
					}
					return e
				}
			}
			if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_subscriptions SET cursor=$3,polled_at=clock_timestamp() WHERE instance_id=$1 AND handler=$2 AND snapshot_id=$4 AND cursor<$3`, p.id, p.handler, event.cursor, snap.ID); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s Store) eventRun(ctx context.Context, snap Snapshot, d contracts.Descriptor, cursor int64, id string, payload json.RawMessage) (string, error) {
	raw, e := json.Marshal(events.Envelope{ID: id, Type: contracts.EventType(d.ID), SchemaVersion: 1, Payload: payload})
	if e != nil {
		return "", e
	}
	return s.Submit(ctx, snap, d, raw, fmt.Sprintf("event:%s:%d:%s:%s", snap.InstanceID, snap.GrantRevision, id, d.Handler), "", cursor, time.Now().Add(10*time.Minute))
}
func (s Store) Replay(ctx context.Context, actor, id string, after int64, limit int, confirmUnknown bool) (int, error) {
	if after < 0 || limit < 1 || limit > 200 {
		return 0, ErrInvalid
	}
	snap, e := s.Snapshot(ctx, id)
	if e != nil {
		return 0, e
	}
	rows, e := s.DB.Query(ctx, `SELECT cursor,event_id::text,event_type,payload FROM plugin_v2_event_journal WHERE cursor>$1 ORDER BY cursor LIMIT $2`, after, limit)
	if e != nil {
		return 0, e
	}
	type entry struct {
		cursor  int64
		id, typ string
		raw     json.RawMessage
	}
	entries := []entry{}
	for rows.Next() {
		var v entry
		if e = rows.Scan(&v.cursor, &v.id, &v.typ, &v.raw); e != nil {
			break
		}
		entries = append(entries, v)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	if other != nil {
		return 0, other
	}
	n := 0
	for _, v := range entries {
		for _, d := range snap.Metadata.Descriptors {
			if d.Kind != "event" || d.ID != v.typ {
				continue
			}
			if e = s.granted(ctx, id, snap, d); e != nil {
				return n, e
			}
			if !confirmUnknown {
				var unknown bool
				if e = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_runs WHERE instance_id=$1 AND handler=$2 AND event_cursor=$3 AND state='unknown')`, id, d.Handler, v.cursor).Scan(&unknown); e != nil {
					return n, e
				}
				if unknown {
					continue
				}
			}
			run, e := s.eventRun(ctx, snap, d, v.cursor, v.id, v.raw)
			if e != nil {
				return n, e
			}
			if confirmUnknown {
				if _, e = s.DB.Exec(ctx, `UPDATE plugin_v2_runs SET state='pending',reason='',lease_token=NULL,lease_until=NULL,deadline_at=now()+interval '10 minutes',finished_at=NULL,snapshot_id=$2,generation=$3,grant_revision=$4 WHERE id=$1 AND state IN('unknown','failed','canceled')`, run, snap.ID, snap.Generation, snap.GrantRevision); e != nil {
					return n, e
				}
			}
			n++
		}
	}
	details, _ := json.Marshal(map[string]any{"instanceId": id, "after": after, "limit": limit, "confirmUnknown": confirmUnknown, "count": n})
	_, e = s.DB.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,'plugins.v2.events.replay',$2)`, actor, details)
	return n, e
}
