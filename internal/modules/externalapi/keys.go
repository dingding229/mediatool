package externalapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/secrets"
	"time"
)

type Key struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Token      string     `json:"token,omitempty"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}
type Repository struct {
	DB    *pgxpool.Pool
	Vault *secrets.Vault
}

var Scopes = map[string]bool{"status.read": true, "library.read": true, "layout.preview": true, "tasks.read": true, "search.run": true, "downloads.run": true}
var ErrUnauthorized = errors.New("API凭据无效、过期或已撤销")
var ErrForbidden = errors.New("API凭据没有此接口权限")
var ErrRate = errors.New("API每分钟最多120次调用")

func (r Repository) Create(ctx context.Context, name string, scopes []string, expires time.Time) (Key, error) {
	out := Key{ID: jobs.UUID(), Name: name, Scopes: scopes, ExpiresAt: expires}
	if len(name) < 1 || len(name) > 100 || len(scopes) == 0 || len(scopes) > len(Scopes) || !expires.After(time.Now()) || expires.After(time.Now().Add(366*24*time.Hour)) {
		return out, errors.New("名称、权限或有效期无效（最多366天）")
	}
	seen := map[string]bool{}
	for _, s := range scopes {
		if !Scopes[s] || seen[s] {
			return out, errors.New("权限无效或重复")
		}
		seen[s] = true
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return out, err
	}
	out.Token = "mt_api_" + hex.EncodeToString(raw)
	out.Prefix = out.Token[:15]
	digest := sha256.Sum256([]byte(out.Token))
	cipher, err := r.Vault.Seal("api-key:"+out.ID, map[string]string{"token": out.Token})
	if err != nil {
		return out, err
	}
	scopeJSON, _ := json.Marshal(scopes)
	err = r.DB.QueryRow(ctx, `INSERT INTO external_api_keys(id,name,token_digest,token_ciphertext,prefix,scopes,expires_at)VALUES($1,$2,$3,$4,$5,$6,$7)RETURNING created_at`, out.ID, name, digest[:], cipher, out.Prefix, scopeJSON, expires).Scan(&out.CreatedAt)
	return out, err
}
func (r Repository) List(ctx context.Context) ([]Key, error) {
	rows, err := r.DB.Query(ctx, `SELECT id::text,name,prefix,scopes,expires_at,revoked_at,last_used_at,created_at,token_ciphertext FROM external_api_keys ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Key{}
	for rows.Next() {
		var k Key
		var scopes, cipher []byte
		if err = rows.Scan(&k.ID, &k.Name, &k.Prefix, &scopes, &k.ExpiresAt, &k.RevokedAt, &k.LastUsedAt, &k.CreatedAt, &cipher); err != nil {
			return nil, err
		}
		if json.Unmarshal(scopes, &k.Scopes) != nil {
			return nil, ErrUnauthorized
		}
		values, err := r.Vault.Open("api-key:"+k.ID, cipher)
		if err != nil {
			return nil, err
		}
		k.Token = values["token"]
		out = append(out, k)
	}
	return out, rows.Err()
}
func (r Repository) Revoke(ctx context.Context, id string) error {
	tag, err := r.DB.Exec(ctx, `UPDATE external_api_keys SET revoked_at=coalesce(revoked_at,now())WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrUnauthorized
	}
	return nil
}
func (r Repository) Admit(ctx context.Context, token, scope, method, path string) (string, int64, error) {
	if len(token) != 71 {
		return "", 0, ErrUnauthorized
	}
	sum := sha256.Sum256([]byte(token))
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback(ctx)
	var id string
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT id::text,scopes FROM external_api_keys WHERE token_digest=$1 AND revoked_at IS NULL AND expires_at>now() FOR UPDATE`, sum[:]).Scan(&id, &raw); err != nil {
		return "", 0, ErrUnauthorized
	}
	var scopes []string
	if json.Unmarshal(raw, &scopes) != nil {
		return "", 0, ErrUnauthorized
	}
	ok := false
	for _, s := range scopes {
		if s == scope {
			ok = true
		}
	}
	if !ok {
		return "", 0, ErrForbidden
	}
	var n int
	if err = tx.QueryRow(ctx, `SELECT count(*)FROM external_api_calls WHERE key_id=$1 AND created_at>now()-interval '1 minute'`, id).Scan(&n); err != nil {
		return "", 0, err
	}
	if n >= 120 {
		return "", 0, ErrRate
	}
	var call int64
	if err = tx.QueryRow(ctx, `INSERT INTO external_api_calls(key_id,method,path,status)VALUES($1,$2,$3,0)RETURNING id`, id, method, path).Scan(&call); err != nil {
		return "", 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE external_api_keys SET last_used_at=now() WHERE id=$1`, id); err != nil {
		return "", 0, err
	}
	err = tx.Commit(ctx)
	return id, call, err
}
