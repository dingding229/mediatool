package v2

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

const journalLimit = 100000
const journalRetention = 7 * 24 * time.Hour

// ReplayExpired is explicit: never silently skip missing history or move a
// subscriber's cursor to make room. Zero replayWindow is unlimited freshness
// within this physical retention window, not unlimited durable storage.
type ReplayExpired struct {
	RetiredCursor int64 `json:"retiredCursor"`
}

func (e *ReplayExpired) Error() string {
	return "event history outside retention; inspect cursor before replay"
}

type JournalCapacity struct {
	Stored           int    `json:"stored"`
	Limit            int    `json:"limit"`
	RetiredCursor    int64  `json:"retiredCursor"`
	RetentionSeconds int    `json:"retentionSeconds"`
	State            string `json:"state"`
	NextAction       string `json:"nextAction"`
}

func (s Store) JournalCapacity(ctx context.Context) (JournalCapacity, error) {
	out := JournalCapacity{Limit: journalLimit, RetentionSeconds: int(journalRetention.Seconds()), State: "ready", NextAction: "none"}
	e := s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM plugin_v2_event_journal),retired_cursor FROM plugin_v2_journal_retention WHERE singleton`).Scan(&out.Stored, &out.RetiredCursor)
	if out.Stored >= journalLimit {
		out.State = "blocked"
		out.NextAction = "inspect_subscriber_and_retention"
	} else if out.Stored >= journalLimit*8/10 {
		out.State = "warning"
		out.NextAction = "inspect_subscriber_and_retention"
	}
	return out, e
}

// CollectJournal owns the same serialization fence as Bridge and explicit
// Replay. Only an ACKed contiguous prefix older than seven INGESTION days is
// retired. Late committed facts get a fresh ingestion horizon regardless of
// their original timestamp. Unresolved runs (especially unknown) pin history.
func (s Store) CollectJournal(ctx context.Context) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7063001)`); e != nil {
		return e
	}
	// Retain an event's dedupe key until the replay horizon expires. Replay holds
	// the same lock, so pruning cannot race a user-confirmed replay into a new run.
	if _, e = tx.Exec(ctx, `DELETE FROM plugin_v2_runs WHERE id IN(SELECT r.id FROM plugin_v2_runs r LEFT JOIN plugin_v2_event_journal j ON j.cursor=r.event_cursor WHERE r.state IN('succeeded','failed','canceled') AND r.finished_at<clock_timestamp()-interval '7 days' AND (r.event_cursor IS NULL OR j.ingested_at<clock_timestamp()-interval '7 days') ORDER BY r.finished_at LIMIT 1000)`); e != nil {
		return e
	}
	var floor int64
	e = tx.QueryRow(ctx, `SELECT coalesce(max(cursor),0) FROM (SELECT cursor FROM plugin_v2_event_journal WHERE cursor<=coalesce((SELECT min(cursor) FROM plugin_v2_subscriptions),9223372036854775807) AND cursor<coalesce((SELECT min(event_cursor) FROM plugin_v2_runs),9223372036854775807) AND cursor<coalesce((SELECT min(cursor) FROM plugin_v2_event_journal WHERE ingested_at>=clock_timestamp()-interval '7 days'),9223372036854775807) ORDER BY cursor LIMIT 1000) q`).Scan(&floor)
	if e != nil {
		return e
	}
	if floor > 0 {
		// Rows inserted manually/missing receipts must fail closed, not re-ingest.
		var missing bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_event_journal j JOIN outbox_events o ON o.id=j.event_id WHERE j.cursor<=$1 AND o.plugin_journal_cursor IS DISTINCT FROM j.cursor)`, floor).Scan(&missing); e != nil {
			return e
		}
		if missing {
			return errors.New("journal receipt mismatch; collection stopped")
		}
		if _, e = tx.Exec(ctx, `DELETE FROM plugin_v2_event_journal WHERE cursor<=$1`, floor); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE plugin_v2_journal_retention SET retired_cursor=greatest(retired_cursor,$1) WHERE singleton`, floor); e != nil {
			return e
		}
	}
	// Compact only consumed source payloads after notification dispatch; keep
	// id/type/aggregate receipts forever for EmitTx / business transition dedupe.
	// Never delete source rows, pending notifications, or plugin.notice receipts.
	if _, e = tx.Exec(ctx, `WITH ready AS(SELECT id FROM outbox_events WHERE NOT plugin_compacted AND plugin_journal_cursor<=(SELECT retired_cursor FROM plugin_v2_journal_retention WHERE singleton) AND dispatched_at IS NOT NULL ORDER BY plugin_journal_cursor LIMIT 1000 FOR UPDATE SKIP LOCKED) UPDATE outbox_events o SET payload='{}',plugin_compacted=true FROM ready WHERE o.id=ready.id`); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func replayFence(ctx context.Context, tx pgx.Tx, after int64) error {
	if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7063001)`); e != nil {
		return e
	}
	var floor int64
	if e := tx.QueryRow(ctx, `SELECT greatest(retired_cursor,coalesce((SELECT max(cursor) FROM plugin_v2_event_journal WHERE ingested_at<clock_timestamp()-interval '7 days'),0)) FROM plugin_v2_journal_retention WHERE singleton`).Scan(&floor); e != nil {
		return e
	}
	if after < floor {
		return &ReplayExpired{floor}
	}
	return nil
}
