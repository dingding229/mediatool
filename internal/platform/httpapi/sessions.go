package httpapi

import (
	"context"
	"crypto/sha256"
	"media-tool/internal/modules/accounts"
	"media-tool/internal/platform/auth"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"strings"
	"time"
)

func (a *API) sessionAccount(r *http.Request) (accounts.Account, error) {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil || !a.Auth.VerifySignature(cookie.Value) {
		return accounts.Account{}, accounts.ErrCredentials
	}
	digest := sha256.Sum256([]byte(cookie.Value))
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	var reader SessionReader = a.Deps.AuthSessions
	if reader == nil {
		reader = a.accountRepository()
	}
	return reader.Session(ctx, digest[:])
}

// Call only before response headers start. SSE heartbeats and nested ownership
// checks use sessionAccount (read-only), never an unobservable DB-only renewal.
func (a *API) activeSessionAccount(w http.ResponseWriter, r *http.Request) (accounts.Account, error) {
	account, err := a.sessionAccount(r)
	if err != nil || a.Deps.AuthSessions != nil || strings.HasPrefix(r.URL.Path, "/api/v1/ops/restores") {
		return account, err
	}
	cookie, e := r.Cookie(auth.CookieName)
	if e != nil {
		return account, accounts.ErrCredentials
	}
	digest := sha256.Sum256([]byte(cookie.Value))
	ctx, cancel := context.WithTimeout(r.Context(), 100*time.Millisecond)
	defer cancel()
	touch, e := a.accountRepository().TouchSession(ctx, digest[:], adminSessionMetadata(r))
	if e != nil {
		a.Deps.Logger.Warn("administrator session activity update failed", "error", e)
	} else if touch.Renewed {
		a.Auth.SetCookieUntil(w, cookie.Value, touch.ExpiresAt, touch.AsOf)
	}
	return account, nil
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
