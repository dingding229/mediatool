package nativeauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/platform/auth"
)

func limitDigest(secret, scope string) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	if _, err := h.Write([]byte("media-tool:member-rate:v1:" + scope)); err != nil {
		return nil
	}
	return h.Sum(nil)
}
func (s *Store) ChangePassword(ctx context.Context, u contracts.User, t contracts.Token, current, next string) (err error) {
	if s == nil || s.DB == nil {
		return contracts.ErrUnavailable
	}
	if current == next || auth.ValidatePassword(next) != nil {
		return contracts.ErrInvalidPassword
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	release, err := passwordPermit()
	if err != nil {
		return err
	}
	defer release()
	var original string
	err = s.DB.QueryRow(ctx, `SELECT password_hash FROM media_users WHERE id=$1::uuid AND credential_version=$2 AND enabled`, u.ID, u.CredentialVersion).Scan(&original)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.ErrUnauthenticated
	}
	if err != nil {
		return contracts.ErrUnavailable
	}
	if !auth.VerifyPassword(original, current) {
		return contracts.ErrUnauthenticated
	}
	hash, err := auth.HashPassword(next)
	if err != nil {
		return contracts.ErrInvalidPassword
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return contracts.ErrUnavailable
	}
	defer rollback(ctx, tx, &err)
	var version int64
	err = tx.QueryRow(ctx, `SELECT credential_version FROM media_users WHERE id=$1::uuid AND enabled AND NOT must_change_password AND credential_version=$2 AND (expires_at IS NULL OR expires_at>now()) FOR UPDATE`, u.ID, u.CredentialVersion).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.ErrUnauthenticated
	}
	if err != nil {
		return contracts.ErrUnavailable
	}
	var active bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_user_tokens WHERE token_digest=$1 AND user_id=$2::uuid AND revoked_at IS NULL AND expires_at>now() AND credential_version=$3)`, t.Digest[:], u.ID, version).Scan(&active)
	if err != nil {
		return contracts.ErrUnavailable
	}
	if !active {
		return contracts.ErrUnauthenticated
	}
	tag, err := tx.Exec(ctx, `UPDATE media_users SET password_hash=$3,must_change_password=false,credential_version=credential_version+1,updated_at=now() WHERE id=$1::uuid AND credential_version=$2`, u.ID, version, hash)
	if err != nil {
		return contracts.ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return contracts.ErrUnauthenticated
	}
	if _, err = tx.Exec(ctx, `UPDATE media_user_tokens SET revoked_at=now() WHERE user_id=$1::uuid AND token_digest<>$2 AND revoked_at IS NULL`, u.ID, t.Digest[:]); err != nil {
		return contracts.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `UPDATE media_user_sessions SET revoked_at=now() WHERE user_id=$1::uuid AND revoked_at IS NULL`, u.ID); err != nil {
		return contracts.ErrUnavailable
	}
	tag, err = tx.Exec(ctx, `UPDATE media_user_tokens SET credential_version=$3,last_seen_at=now(),expires_at=now()+interval '365 days' WHERE user_id=$1::uuid AND token_digest=$2 AND revoked_at IS NULL`, u.ID, t.Digest[:], version+1)
	if err != nil {
		return contracts.ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return contracts.ErrUnauthenticated
	}
	if err = tx.Commit(ctx); err != nil {
		return contracts.ErrUnavailable
	}
	return nil
}
