// Package safeerrors deliberately does not scrape logs or accept exceptions.
// Only bounded, allowlisted summaries enter this bus; plugin logs cannot recurse.
package safeerrors

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"media-tool/internal/platform/pluginactor"
	"media-tool/internal/platform/pluginstate"
	"strings"
	"time"
)

type Bus struct{ DB *pgxpool.Pool }
type Summary struct{ Source, Code string }
type aggregate struct {
	contracts.OperationFailed
	Closed bool `json:"closed"`
}

func valid(v Summary) bool {
	switch v.Source {
	case "jobs", "system", "library", "downloads", "subscriptions", "metadata":
	default:
		return false
	}
	return v.Code == "terminal_failure" || v.Code == "dependency_unavailable" || v.Code == "worker_claim_failed"
}
func EmitTx(ctx context.Context, tx pgx.Tx, v Summary) error {
	if strings.HasPrefix(v.Source, "plugins") || pluginactor.InstanceID(ctx) != "" {
		return nil
	}
	if !valid(v) {
		return errors.New("unsupported safe error summary")
	}
	window := time.Now().UTC().Truncate(time.Minute)
	for range 2 {
		event := aggregate{OperationFailed: contracts.OperationFailed{Source: v.Source, Code: v.Code, Window: window.Format(time.RFC3339)}}
		key := "plugins.errors." + v.Source + "." + v.Code + "." + window.Format("20060102T1504")
		if e := pluginstate.Lock(ctx, tx, key, &event); e != nil {
			return e
		}
		if event.Closed {
			window = window.Add(time.Minute)
			continue
		}
		event.Count = min(event.Count+1, 1000000)
		return pluginstate.Put(ctx, tx, key, event)
	}
	return errors.New("safe error aggregation temporarily busy")
}
func (b Bus) Report(ctx context.Context, v Summary) error {
	if strings.HasPrefix(v.Source, "plugins") || pluginactor.InstanceID(ctx) != "" {
		return nil
	}
	tx, e := b.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	if e = EmitTx(ctx, tx, v); e != nil {
		return e
	}
	return dbutil.Commit(ctx, tx)
}

// Flush emits one immutable aggregate after its minute closes, at most 200 per
// batch. Closing the consumed state and outbox insertion commit together; a
// late writer cannot reopen the closed bucket and lose its occurrence.
func (b Bus) Flush(ctx context.Context) error {
	tx, e := b.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	rows, e := tx.Query(ctx, `SELECT key,value FROM app_settings WHERE key LIKE 'plugins.errors.%' AND NOT coalesce((value->>'closed')::boolean,false) AND (value->>'window')::timestamptz<date_trunc('minute',clock_timestamp()) ORDER BY key FOR UPDATE SKIP LOCKED LIMIT 200`)
	if e != nil {
		return e
	}
	type entry struct {
		k string
		v contracts.OperationFailed
	}
	list := []entry{}
	for rows.Next() {
		var x entry
		var raw []byte
		if e = rows.Scan(&x.k, &raw); e != nil {
			break
		}
		if e = json.Unmarshal(raw, &x.v); e != nil {
			break
		}
		list = append(list, x)
	}
	other := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if other != nil {
		return other
	}
	for _, x := range list {
		if !valid(Summary{x.v.Source, x.v.Code}) || x.v.Count < 1 {
			return errors.New("invalid safe error aggregate")
		}
		if e = outbox.EmitVersionTx(ctx, tx, "system.operation.failed", x.k, "v1", x.v); e != nil {
			return e
		}
		if e = pluginstate.Put(ctx, tx, x.k, aggregate{OperationFailed: x.v, Closed: true}); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `DELETE FROM app_settings WHERE key IN(SELECT key FROM app_settings WHERE key LIKE 'plugins.errors.%' AND updated_at<now()-interval '1 day' LIMIT 200)`); e != nil {
		return e
	}
	return dbutil.Commit(ctx, tx)
}
