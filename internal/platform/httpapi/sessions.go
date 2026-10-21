package httpapi

import (
	"context"
	"crypto/sha256"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"time"
)

func (a *API) sessionAccount(r *http.Request) (accounts.Account, error) {
	if !a.Auth.Authorized(r) {
		return accounts.Account{}, accounts.ErrCredentials
	}
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		return accounts.Account{}, accounts.ErrCredentials
	}
	digest := sha256.Sum256([]byte(cookie.Value))
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	var reader SessionReader = a.Deps.AuthSessions
	if reader == nil {
		reader = a.accountRepository()
	}
	account, err := reader.Session(ctx, digest[:])
	if err == nil && a.Deps.AuthSessions == nil {
		if err = a.accountRepository().TouchSession(ctx, digest[:], adminSessionMetadata(r)); err != nil {
			return accounts.Account{}, accounts.ErrUnavailable
		}
	}
	return account, err
}
func (a *API) authorized(r *http.Request) bool { _, err := a.sessionAccount(r); return err == nil }
func (a *API) revokeSession(r *http.Request) error {
	if a.Deps.DB == nil {
		return nil
	}
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		return nil
	}
	digest := sha256.Sum256([]byte(cookie.Value))
	bounded, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	tx, err := a.Deps.DB.Begin(bounded)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(bounded, tx)
	if _, err = tx.Exec(bounded, `UPDATE administrator_sessions SET revoked_at=now()WHERE token_digest=$1`, digest[:]); err != nil {
		return err
	}
	if _, err = tx.Exec(bounded, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','session.logout','{}')`); err != nil {
		return err
	}
	return tx.Commit(bounded)
}
