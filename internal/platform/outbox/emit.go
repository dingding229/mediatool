// Package outbox records business facts in the caller's transaction. Consumers
// own their independent delivery cursors; notification dispatch is not stolen.
package outbox

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

// EmitVersionTx keys a fact by immutable business version, not by a reusable
// aggregate or random retry key. version is the row revision or collection
// attempt identity. Retrying the same fact cannot create a second delivery.
func EmitVersionTx(ctx context.Context, tx pgx.Tx, kind, aggregateID, version string, value any) error {
	sum := sha256.Sum256([]byte(kind + "\x00" + aggregateID + "\x00" + version))
	factID := fmt.Sprintf("%x-%x-%x-%x-%x", sum[:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
	return EmitTx(ctx, tx, kind, factID, value)
}
