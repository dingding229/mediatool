package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	deferutil "media-tool/internal/platform/cleanup"
)

var ErrInitialization = errors.New("administrator initialization token missing, invalid or expired")

func ValidInitializationToken(token string) bool {
	if len(token) != base64.RawURLEncoding.EncodedLen(32) {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	return err == nil && len(decoded) == 32
}

func lockRegistration(ctx context.Context, tx pgx.Tx) (bool, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(716)`); err != nil {
		return false, err
	}
	var registered bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM administrator_accounts)`).Scan(&registered)
	return registered, err
}

// PrintInitialization is for startup only. Never pass an application logger or
// the web log store. A new empty-database startup supersedes the previous token.
func (r Repository) PrintInitialization(ctx context.Context, stdout io.Writer) error {
	if r.DB == nil {
		return ErrUnavailable
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer deferutil.Rollback(ctx, tx)
	registered, err := lockRegistration(ctx, tx)
	if err != nil {
		return ErrUnavailable
	}
	if registered {
		return nil
	}
	var entropy [32]byte
	if _, err = rand.Read(entropy[:]); err != nil {
		return errors.New("administrator initialization entropy unavailable")
	}
	token := base64.RawURLEncoding.EncodeToString(entropy[:])
	digest := sha256.Sum256([]byte(token))
	_, err = tx.Exec(ctx, `INSERT INTO administrator_initialization(singleton,token_digest,expires_at) VALUES(true,$1,clock_timestamp()+interval '24 hours')
ON CONFLICT(singleton) DO UPDATE SET token_digest=excluded.token_digest,expires_at=excluded.expires_at`, digest[:])
	if err != nil || tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	if _, err = fmt.Fprintf(stdout, "[ADMIN INITIALIZATION] Token: %s | Expires in 24 hours. Enter it in first administrator registration. Startup console only.\n", token); err != nil {
		return errors.New("administrator initialization console unavailable; restart to issue a new token")
	}
	return nil
}

func consumeInitialization(ctx context.Context, tx pgx.Tx, token string) error {
	if !ValidInitializationToken(token) {
		return ErrInitialization
	}
	digest := sha256.Sum256([]byte(token))
	tag, err := tx.Exec(ctx, `DELETE FROM administrator_initialization WHERE singleton AND token_digest=$1 AND expires_at>clock_timestamp()`, digest[:])
	if err != nil {
		return ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return ErrInitialization
	}
	return nil
}
