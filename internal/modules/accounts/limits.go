package accounts

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"

	"github.com/jackc/pgx/v5/pgconn"
	deferutil "media-tool/internal/platform/cleanup"
)

type attemptLimitExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func pruneExpiredAttempts(ctx context.Context, q attemptLimitExecer) error {
	_, err := q.Exec(ctx, `DELETE FROM administrator_auth_limits WHERE scope_digest IN (SELECT scope_digest FROM administrator_auth_limits WHERE window_start<now()-interval '1 day' ORDER BY window_start LIMIT 256)`)
	return err
}

func takeAttempt(ctx context.Context, q attemptLimitExecer, secret, scope string, limit int) (bool, error) {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte("media-tool:auth-rate:v1:" + scope))
	tag, err := q.Exec(ctx, `INSERT INTO administrator_auth_limits(scope_digest,window_start,attempts) VALUES($1,now(),1)
ON CONFLICT(scope_digest) DO UPDATE SET
 window_start=CASE WHEN administrator_auth_limits.window_start<=now()-interval '1 minute' THEN now() ELSE administrator_auth_limits.window_start END,
 attempts=CASE WHEN administrator_auth_limits.window_start<=now()-interval '1 minute' THEN 1 ELSE administrator_auth_limits.attempts+1 END
WHERE administrator_auth_limits.window_start<=now()-interval '1 minute' OR administrator_auth_limits.attempts<$2`, h.Sum(nil), limit)
	if err != nil {
		return false, ErrUnavailable
	}
	return tag.RowsAffected() == 1, nil
}

// Shared PG counters are HMAC-scoped and remain effective across restarts.
func (r Repository) TakeAttempt(ctx context.Context, secret, scope string, limit int) (bool, error) {
	if r.DB == nil || pruneExpiredAttempts(ctx, r.DB) != nil {
		return false, ErrUnavailable
	}
	return takeAttempt(ctx, r.DB, secret, scope, limit)
}

// AdmitAuthentication takes the source and global budgets in that order in a
// single transaction. A blocked source never reads or charges the global row.
func (r Repository) AdmitAuthentication(ctx context.Context, secret, ip string) (bool, error) {
	if r.DB == nil || pruneExpiredAttempts(ctx, r.DB) != nil {
		return false, ErrUnavailable
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return false, ErrUnavailable
	}
	defer deferutil.Rollback(ctx, tx)
	if allowed, err := takeAttempt(ctx, tx, secret, "ip:"+ip, 20); err != nil || !allowed {
		return false, err
	}
	allowed, err := takeAttempt(ctx, tx, secret, "global", 120)
	// Keep the admitted source charge even when the global budget is full.
	if err != nil || tx.Commit(ctx) != nil {
		return false, ErrUnavailable
	}
	return allowed, nil
}
