// Package outbox records business facts in the caller's transaction. Consumers
// own their independent delivery cursors; notification dispatch is not stolen.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
)

// EmitTx records one immutable fact. factID must identify a single occurrence,
// NOT a reusable aggregate: e.g. files operationID, not a directory/libraryID.
// Repeated facts for one aggregate need distinct occurrence IDs. Callers that
// already fence a state transition may INSERT a new outbox UUID in that transaction.
func EmitTx(ctx context.Context, tx pgx.Tx, kind, factID string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil || len(raw) > 32<<10 {
		return errors.New("invalid outbox fact")
	}
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1||':'||$2,630))`, kind, factID); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload,created_at)SELECT gen_random_uuid(),$1,$2,$3,clock_timestamp() WHERE NOT EXISTS(SELECT 1 FROM outbox_events WHERE event_type=$1 AND aggregate_id=$2)`, kind, factID, raw)
	return e
}
