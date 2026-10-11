package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/platform/auth"
	"strings"
	"time"
)

const RecoveryLifetime = 30 * 24 * time.Hour

// Recovery codes have 256 bits of entropy, are shown once, and are never stored
// in plaintext. A new code replaces the previous code for the account.
func newRecoveryCode() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	s := hex.EncodeToString(raw)
	return "mt-" + s[:16] + "-" + s[16:32] + "-" + s[32:48] + "-" + s[48:], nil
}
func recoveryDigest(code string) ([]byte, error) {
	if len(code) > 100 {
		return nil, ErrCredentials
	}
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	if len(normalized) != 66 || !strings.HasPrefix(normalized, "mt") {
		return nil, ErrCredentials
	}
	if _, err := hex.DecodeString(normalized[2:]); err != nil {
		return nil, ErrCredentials
	}
	sum := sha256.Sum256([]byte(normalized))
	return sum[:], nil
}
func (r Repository) IssueRecoveryCode(ctx context.Context, a Account, currentPassword string) (string, time.Time, error) {
	checked, err := r.Authenticate(ctx, a.Username, currentPassword)
	if err != nil {
		return "", time.Time{}, err
	}
	if checked.ID != a.ID || checked.Version != a.Version {
		return "", time.Time{}, ErrCredentials
	}
	code, err := newRecoveryCode()
	if err != nil {
		return "", time.Time{}, ErrUnavailable
	}
	digest, _ := recoveryDigest(code)
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return "", time.Time{}, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	var version int64
	if err = tx.QueryRow(ctx, `SELECT credential_version FROM administrator_accounts WHERE id=$1 FOR UPDATE`, a.ID).Scan(&version); err != nil {
		return "", time.Time{}, ErrUnavailable
	}
	if version != a.Version {
		return "", time.Time{}, ErrCredentials
	}
	var expires time.Time
	err = tx.QueryRow(ctx, `INSERT INTO administrator_recovery_codes(account_id,code_digest,expires_at)VALUES($1,$2,now()+interval '30 days') ON CONFLICT(account_id) DO UPDATE SET code_digest=EXCLUDED.code_digest,expires_at=EXCLUDED.expires_at,created_at=now() RETURNING expires_at`, a.ID, digest).Scan(&expires)
	if err != nil {
		return "", time.Time{}, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','account.recovery_code_rotated',jsonb_build_object('accountId',$1::text))`, a.ID); err != nil {
		return "", time.Time{}, ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return "", time.Time{}, ErrUnavailable
	}
	return code, expires, nil
}
func (r Repository) RecoverPassword(ctx context.Context, username, code, next string) error {
	if r.DB == nil {
		return ErrUnavailable
	}
	name, err := NormalizeUsername(username)
	if err != nil {
		return ErrCredentials
	}
	digest, err := recoveryDigest(code)
	if err != nil {
		return ErrCredentials
	}
	hash, err := auth.HashPassword(next)
	if err != nil {
		return err
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT a.id::text FROM administrator_accounts a JOIN administrator_recovery_codes c ON c.account_id=a.id WHERE a.username=$1 AND c.code_digest=$2 AND c.expires_at>now() FOR UPDATE OF a,c`, name, digest).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCredentials
	}
	if err != nil {
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `DELETE FROM administrator_recovery_codes WHERE account_id=$1`, id); err != nil {
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `UPDATE administrator_accounts SET password_hash=$2,credential_version=credential_version+1,updated_at=now() WHERE id=$1`, id, hash); err != nil {
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `UPDATE administrator_sessions SET revoked_at=now() WHERE account_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('recovery-code','account.password_recovered',jsonb_build_object('accountId',$1::text))`, id); err != nil {
		return ErrUnavailable
	}
	return tx.Commit(ctx)
}
