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
	"regexp"
	"strings"
)

var ErrClosed = errors.New("administrator registration closed")
var ErrCredentials = errors.New("invalid administrator credentials")
var ErrUnavailable = errors.New("administrator database unavailable")
var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,31}$`)

type Account struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Version  int64  `json:"-"`
	Hash     string `json:"-"`
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
func (r Repository) Register(ctx context.Context, username, password, token string) (Account, error) {
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
	defer tx.Rollback(ctx)
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
	if err = insertSession(ctx, tx, account, token); err != nil {
		return Account{}, ErrUnavailable
	}
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
	err := r.DB.QueryRow(ctx, `SELECT id::text,username,password_hash,credential_version FROM administrator_accounts WHERE username=$1`, value).Scan(&a.ID, &a.Username, &a.Hash, &a.Version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrUnavailable
	}
	encoded := a.Hash
	if encoded == "" {
		encoded = auth.DummyPasswordHash
	}
	matches := auth.VerifyPassword(encoded, password)
	if formatErr != nil || err != nil || !matches {
		return Account{}, ErrCredentials
	}
	a.Hash = ""
	return a, nil
}
func insertSession(ctx context.Context, tx pgx.Tx, a Account, token string) error {
	digest := sha256.Sum256([]byte(token))
	_, err := tx.Exec(ctx, `INSERT INTO administrator_sessions(token_digest,account_id,credential_version,expires_at)VALUES($1,$2,$3,now()+interval '8 hours')`, digest[:], a.ID, a.Version)
	return err
}
func (r Repository) CreateSession(ctx context.Context, account Account, token string) error {
	if r.DB == nil {
		return ErrUnavailable
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback(ctx)
	var current int64
	if err = tx.QueryRow(ctx, `SELECT credential_version FROM administrator_accounts WHERE id=$1 FOR SHARE`, account.ID).Scan(&current); err != nil {
		return ErrUnavailable
	}
	if current != account.Version {
		return ErrCredentials
	}
	if _, err = tx.Exec(ctx, `DELETE FROM administrator_sessions WHERE expires_at<now()-interval '1 day'`); err != nil {
		return ErrUnavailable
	}
	if err = insertSession(ctx, tx, account, token); err != nil {
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','session.login',jsonb_build_object('accountId',$1::text))`, account.ID); err != nil {
		return ErrUnavailable
	}
	return tx.Commit(ctx)
}
func (r Repository) Session(ctx context.Context, digest []byte) (Account, error) {
	if r.DB == nil {
		return Account{}, ErrUnavailable
	}
	var a Account
	err := r.DB.QueryRow(ctx, `SELECT a.id::text,a.username,a.credential_version FROM administrator_sessions s JOIN administrator_accounts a ON a.id=s.account_id WHERE s.token_digest=$1 AND s.revoked_at IS NULL AND s.expires_at>now()AND s.credential_version=a.credential_version`, digest).Scan(&a.ID, &a.Username, &a.Version)
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
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE administrator_accounts SET password_hash=$2,credential_version=credential_version+1,updated_at=now()WHERE id=$1 AND credential_version=$3`, a.ID, hash, a.Version)
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
