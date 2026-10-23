package accounts

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/clientip"
	"strings"
	"time"
	"unicode/utf8"
)

type SessionMetadata struct{ IP, UserAgent string }

func sessionMetadata(values []SessionMetadata) SessionMetadata {
	if len(values) > 0 {
		v := values[0]
		v.IP = clientip.Sanitize(v.IP)
		v.UserAgent = boundedUserAgent(v.UserAgent)
		return v
	}
	return SessionMetadata{}
}

// Bound bytes without splitting a UTF-8 rune, including on initial login.
func boundedUserAgent(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= 512 {
		return s
	}
	s = s[:512]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

type SessionInfo struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"accountId"`
	Username     string    `json:"username"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	IP           string    `json:"ip"`
	UserAgent    string    `json:"userAgent"`
	Current      bool      `json:"current"`
}

type SessionPage struct {
	Items   []SessionInfo `json:"items"`
	HasNext bool          `json:"hasNext"`
}

// Touch only a still-valid session; authentication itself always reads PG, never
// an authority cache. Record raw UA/IP, bounded UA/IP, never cookies, hashes or login bodies.
func (r Repository) TouchSession(ctx context.Context, digest []byte, metadata SessionMetadata) error {
	metadata.IP = clientip.Sanitize(metadata.IP)
	metadata.UserAgent = boundedUserAgent(metadata.UserAgent)
	_, err := r.DB.Exec(ctx, `UPDATE administrator_sessions s SET last_active_at=clock_timestamp(),ip=$2,user_agent=$3 FROM administrator_accounts a WHERE s.token_digest=$1 AND s.account_id=a.id AND s.credential_version=a.credential_version AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND (s.last_active_at < clock_timestamp()-interval '60 seconds' OR s.ip<>$2 OR s.user_agent<>$3)`, digest, metadata.IP, metadata.UserAgent)
	return err
}
func (r Repository) Sessions(ctx context.Context, digest []byte, page int) (SessionPage, error) {
	rows, err := r.DB.Query(ctx, `SELECT s.id::text,a.id::text,a.username,s.created_at,s.last_active_at,s.expires_at,s.ip,s.user_agent,s.token_digest=$1 FROM administrator_sessions s JOIN administrator_accounts a ON a.id=s.account_id WHERE s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.credential_version=a.credential_version AND coalesce(a.temporary_password_expires_at>clock_timestamp(),true) ORDER BY s.created_at DESC,s.id LIMIT 51 OFFSET $2`, digest, (page-1)*50)
	if err != nil {
		return SessionPage{}, err
	}
	defer rows.Close()
	out := []SessionInfo{}
	for rows.Next() {
		var v SessionInfo
		if err = rows.Scan(&v.ID, &v.AccountID, &v.Username, &v.CreatedAt, &v.LastActiveAt, &v.ExpiresAt, &v.IP, &v.UserAgent, &v.Current); err != nil {
			return SessionPage{}, err
		}
		v.IP = clientip.Sanitize(v.IP)
		out = append(out, v)
	}
	hasNext := len(out) > 50
	if hasNext {
		out = out[:50]
	}
	return SessionPage{Items: out, HasNext: hasNext}, rows.Err()
}
func (r Repository) RevokeSession(ctx context.Context, id string, digest []byte) (bool, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer deferutil.Rollback(ctx, tx)
	var current bool
	err = tx.QueryRow(ctx, `UPDATE administrator_sessions SET revoked_at=coalesce(revoked_at,clock_timestamp()) WHERE id=$1 RETURNING token_digest=$2`, id, digest).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrCredentials
	}
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','session.revoke',jsonb_build_object('sessionId',$1::text))`, id); err != nil {
		return false, err
	}
	return current, tx.Commit(ctx)
}
