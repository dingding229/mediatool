package accounts

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/auth"
	deferutil "media-tool/internal/platform/cleanup"
	"regexp"
	"strings"
	"time"
)

var ErrClosed = errors.New("administrator registration closed")
var ErrCredentials = errors.New("invalid administrator credentials")
var ErrUnavailable = errors.New("administrator database unavailable")
var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,31}$`)

type Account struct {
	ID                 string    `json:"id"`
	Username           string    `json:"username"`
	MustChangePassword bool      `json:"mustChangePassword"`
	Version            int64     `json:"-"`
	Hash               string    `json:"-"`
	SessionExpiresAt   time.Time `json:"-"`
	SessionObservedAt  time.Time `json:"-"`
}
type Repository struct{ DB *pgxpool.Pool }

func NormalizeUsername(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if !usernamePattern.MatchString(value) {
		return "", errors.New("账号需3–32位英文字母、数字、下划线、短横线或点，且以字母或数字开头")
	}
	return value, nil
}
func (r Repository) Registered(ctx context.Context) (bool, error) {
	if r.DB == nil {
		return false, ErrUnavailable
	}
	var exists bool
	if err := r.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM administrator_accounts)`).Scan(&exists); err != nil {
		return false, ErrUnavailable
	}
	return exists, nil
}
func (r Repository) Register(ctx context.Context, username, password, token string, metadata ...SessionMetadata) (Account, error) {
	value, err := NormalizeUsername(username)
	if err != nil {
		return Account{}, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return Account{}, err
	}
	if r.DB == nil {
		return Account{}, ErrUnavailable
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return Account{}, ErrUnavailable
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(716)`); err != nil {
		return Account{}, ErrUnavailable
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM administrator_accounts)`).Scan(&exists); err != nil {
		return Account{}, ErrUnavailable
	}
	if exists {
		return Account{}, ErrClosed
	}
	account := Account{ID: jobs.UUID(), Username: value, Version: 1}
	if _, err = tx.Exec(ctx, `INSERT INTO administrator_accounts(id,username,password_hash)VALUES($1,$2,$3)`, account.ID, account.Username, hash); err != nil {
		return Account{}, ErrUnavailable
	}
	validity, err := insertSession(ctx, tx, account, token, metadata...)
	if err != nil {
		return Account{}, ErrUnavailable
	}
	account.SessionExpiresAt, account.SessionObservedAt = validity.ExpiresAt, validity.AsOf
	raw, _ := json.Marshal(map[string]string{"accountId": account.ID})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','account.register',$1)`, raw); err != nil {
		return Account{}, ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return Account{}, ErrUnavailable
	}
	return account, nil
}
func (r Repository) Authenticate(ctx context.Context, username, password string) (Account, error) {
	if r.DB == nil {
		return Account{}, ErrUnavailable
	}
	value, formatErr := NormalizeUsername(username)
	var a Account
	var valid bool
	err := r.DB.QueryRow(ctx, `SELECT id::text,username,password_hash,credential_version,must_change_password,coalesce(temporary_password_expires_at>now(),true) FROM administrator_accounts WHERE username=$1`, value).Scan(&a.ID, &a.Username, &a.Hash, &a.Version, &a.MustChangePassword, &valid)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrUnavailable
	}
	encoded := a.Hash
	if encoded == "" {
		encoded = auth.DummyPasswordHash
	}
	matches := auth.VerifyPassword(encoded, password)
	if formatErr != nil || err != nil || !matches || !valid {
		return Account{}, ErrCredentials
	}
	a.Hash = ""
	return a, nil
}
func insertSession(ctx context.Context, tx pgx.Tx, a Account, token string, metadata ...SessionMetadata) (SessionDeadline, error) {
	digest := sha256.Sum256([]byte(token))
	m := sessionMetadata(metadata)
	var expires SessionDeadline
	err := tx.QueryRow(ctx, `INSERT INTO administrator_sessions(token_digest,account_id,credential_version,expires_at,ip,user_agent)
 SELECT $1,a.id,$3,least(now()+interval '7 days',now()+interval '30 days',coalesce(a.temporary_password_expires_at,now()+interval '7 days')),$4,$5
 FROM administrator_accounts a WHERE a.id=$2 AND a.credential_version=$3 AND coalesce(a.temporary_password_expires_at>now(),true)
 RETURNING expires_at,created_at`, digest[:], a.ID, a.Version, m.IP, m.UserAgent).Scan(&expires.ExpiresAt, &expires.AsOf)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionDeadline{}, ErrCredentials
	}
	return expires, err
}
func (r Repository) CreateSession(ctx context.Context, account Account, token string, metadata ...SessionMetadata) (SessionDeadline, error) {
	if r.DB == nil {
		return SessionDeadline{}, ErrUnavailable
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return SessionDeadline{}, ErrUnavailable
	}
	defer deferutil.Rollback(ctx, tx)
	var current int64
	if err = tx.QueryRow(ctx, `SELECT credential_version FROM administrator_accounts WHERE id=$1 FOR SHARE`, account.ID).Scan(&current); err != nil {
		return SessionDeadline{}, ErrUnavailable
	}
	if current != account.Version {
		return SessionDeadline{}, ErrCredentials
	}
	if _, err = tx.Exec(ctx, `DELETE FROM administrator_sessions WHERE expires_at<now()-interval '1 day'`); err != nil {
		return SessionDeadline{}, ErrUnavailable
	}
	expires, err := insertSession(ctx, tx, account, token, metadata...)
	if err != nil {
		return SessionDeadline{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','session.login',jsonb_build_object('accountId',$1::text))`, account.ID); err != nil {
		return SessionDeadline{}, ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return SessionDeadline{}, err
	}
	return expires, nil
}
func (r Repository) Session(ctx context.Context, digest []byte) (Account, error) {
	if r.DB == nil {
		return Account{}, ErrUnavailable
	}
	var a Account
	err := r.DB.QueryRow(ctx, `SELECT a.id::text,a.username,a.credential_version,a.must_change_password FROM administrator_sessions s JOIN administrator_accounts a ON a.id=s.account_id WHERE s.token_digest=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND s.created_at+interval '30 days'>now() AND s.credential_version=a.credential_version AND coalesce(a.temporary_password_expires_at>now(),true)`, digest).Scan(&a.ID, &a.Username, &a.Version, &a.MustChangePassword)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrCredentials
	}
	if err != nil {
		return a, ErrUnavailable
	}
	return a, nil
}
func (r Repository) ChangePassword(ctx context.Context, a Account, current, next string) error {
	checked, err := r.Authenticate(ctx, a.Username, current)
	if err != nil {
		return err
	}
	if checked.ID != a.ID || checked.Version != a.Version {
		return ErrCredentials
	}
	if current == next {
		return errors.New("新密码不能与当前密码相同")
	}
	hash, err := auth.HashPassword(next)
	if err != nil {
		return err
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer deferutil.Rollback(ctx, tx)
	tag, err := tx.Exec(ctx, `UPDATE administrator_accounts SET password_hash=$2,credential_version=credential_version+1,must_change_password=false,temporary_password_expires_at=NULL,updated_at=now()WHERE id=$1 AND credential_version=$3`, a.ID, hash, a.Version)
	if err != nil {
		return ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return ErrCredentials
	}
	if _, err = tx.Exec(ctx, `UPDATE administrator_sessions SET revoked_at=now()WHERE account_id=$1 AND revoked_at IS NULL`, a.ID); err != nil {
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `DELETE FROM administrator_recovery_codes WHERE account_id=$1`, a.ID); err != nil {
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','account.password_changed',jsonb_build_object('accountId',$1::text))`, a.ID); err != nil {
		return ErrUnavailable
	}
	return tx.Commit(ctx)
}
