package accounts

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"github.com/jackc/pgx/v5/pgconn"
)

type attemptLimitExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func pruneExpiredAttempts(ctx context.Context, q attemptLimitExecer) error {
	_, err := q.Exec(ctx, `DELETE FROM administrator_auth_limits WHERE scope_digest IN (SELECT scope_digest FROM administrator_auth_limits WHERE window_start<now()-interval '1 day' ORDER BY window_start LIMIT 256)`)
	return err
}

// Shared PG counters are HMAC-scoped; neither raw passwords nor usernames/IPs
// are persisted as limiter keys. Counters remain effective across restarts.
func (r Repository) TakeAttempt(ctx context.Context, secret, scope string, limit int) (bool, error) {
	if r.DB == nil {
		return false, ErrUnavailable
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte("media-tool:auth-rate:v1:" + scope))
	digest := h.Sum(nil)
	// Bound retention of expired source counters even when attempts come from
	// changing addresses. Only digests are retained, not raw network identities.
	if err := pruneExpiredAttempts(ctx, r.DB); err != nil {
		return false, ErrUnavailable
	}
	var attempts int
	err := r.DB.QueryRow(ctx, `INSERT INTO administrator_auth_limits(scope_digest,window_start,attempts)VALUES($1,now(),1)ON CONFLICT(scope_digest)DO UPDATE SET window_start=CASE WHEN administrator_auth_limits.window_start<now()-interval '1 minute' THEN now()ELSE administrator_auth_limits.window_start END,attempts=CASE WHEN administrator_auth_limits.window_start<now()-interval '1 minute' THEN 1 ELSE administrator_auth_limits.attempts+1 END RETURNING attempts`, digest).Scan(&attempts)
	if err != nil {
		return false, ErrUnavailable
	}
	return attempts <= limit, nil
}
