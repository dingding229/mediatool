package externalapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type Key struct {
	KeySummary
	Token string `json:"token"`
}
type KeySummary struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Scopes       []string   `json:"scopes"`
	RevokedAt    *time.Time `json:"revokedAt"`
	LastUsedAt   *time.Time `json:"lastUsedAt"`
	LastIP       string     `json:"lastIP"`
	LastIPSource string     `json:"lastIPSource"`
	CreatedAt    time.Time  `json:"createdAt"`
}
type Repository struct {
	DB     *pgxpool.Pool
	Signer *Signer
}

var Scopes = map[string]bool{"status.read": true, "library.read": true, "layout.preview": true, "tasks.read": true, "search.run": true, "downloads.run": true}
var ErrUnauthorized = errors.New("API凭据无效或已撤销")
var ErrForbidden = errors.New("API凭据没有此接口权限")
var ErrRate = errors.New("API每分钟最多120次调用")
var ErrUnavailable = errors.New("API凭据服务暂不可用")
var ErrRevoked = errors.New("凭据已撤销，不能查看或复制")
var ErrDelete = errors.New("只能删除已撤销的凭据")
var idPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

func validID(v string) bool { return idPattern.MatchString(v) }

const columns = `id::text,name,scopes,revoked_at,last_used_at,coalesce(host(last_ip),''),last_ip_source,created_at`

func scanKey(row interface{ Scan(...any) error }) (KeySummary, error) {
	var k KeySummary
	var raw []byte
	e := row.Scan(&k.ID, &k.Name, &raw, &k.RevokedAt, &k.LastUsedAt, &k.LastIP, &k.LastIPSource, &k.CreatedAt)
	if e == nil {
		e = json.Unmarshal(raw, &k.Scopes)
	}
	return k, e
}
func audit(ctx context.Context, tx pgx.Tx, actor, action string, k KeySummary) error {
	if actor == "" {
		actor = "system:external-api"
	}
	details, _ := json.Marshal(map[string]any{"id": k.ID, "name": k.Name, "scopes": k.Scopes})
	_, e := tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,$2,$3)`, actor, "external_api."+action, details)
	return e
}
func (r Repository) Create(ctx context.Context, name string, scopes []string, actors ...string) (Key, error) {
	var out Key
	name = strings.TrimSpace(name)
	scopes = slices.Clone(scopes)
	slices.Sort(scopes)
	if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 || len(scopes) == 0 || len(scopes) > len(Scopes) {
		return out, errors.New("名称或权限无效")
	}
	for i, v := range scopes {
		if !Scopes[v] || i > 0 && scopes[i-1] == v {
			return out, errors.New("权限无效或重复")
		}
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer deferutil.Rollback(ctx, tx)
	raw, _ := json.Marshal(scopes)
	out.KeySummary, e = scanKey(tx.QueryRow(ctx, `INSERT INTO external_api_keys(id,name,scopes)VALUES($1,$2,$3)RETURNING `+columns, jobs.UUID(), name, raw))
	if e != nil {
		return out, e
	}
	out.Token, e = r.Signer.Sign(out.ID, out.Scopes, out.CreatedAt)
	if e != nil {
		return Key{}, e
	}
	actor := ""
	if len(actors) > 0 {
		actor = actors[0]
	}
	if e = audit(ctx, tx, actor, "created", out.KeySummary); e != nil {
		return Key{}, e
	}
	if e = audit(ctx, tx, actor, "revealed.create", out.KeySummary); e != nil {
		return Key{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Key{}, e
	}
	return out, nil
}
func (r Repository) List(ctx context.Context) ([]KeySummary, error) {
	rows, e := r.DB.Query(ctx, `SELECT `+columns+` FROM external_api_keys ORDER BY created_at DESC,id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []KeySummary{}
	for rows.Next() {
		k, e := scanKey(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
func (r Repository) Reveal(ctx context.Context, id, actor, action string) (Key, error) {
	if !validID(id) || action != "view" && action != "copy" {
		return Key{}, ErrUnauthorized
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return Key{}, e
	}
	defer deferutil.Rollback(ctx, tx)
	k, e := scanKey(tx.QueryRow(ctx, `SELECT `+columns+` FROM external_api_keys WHERE id=$1 FOR SHARE`, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return Key{}, ErrUnauthorized
	}
	if e != nil {
		return Key{}, e
	}
	if k.RevokedAt != nil {
		return Key{}, ErrRevoked
	}
	raw, e := r.Signer.Sign(k.ID, k.Scopes, k.CreatedAt)
	if e != nil {
		return Key{}, e
	}
	if e = audit(ctx, tx, actor, "revealed."+action, k); e != nil {
		return Key{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Key{}, e
	}
	return Key{k, raw}, nil
}
func (r Repository) Revoke(ctx context.Context, id string, actors ...string) error {
	return r.change(ctx, id, false, actors...)
}
func (r Repository) Delete(ctx context.Context, id string, actors ...string) error {
	return r.change(ctx, id, true, actors...)
}
func (r Repository) change(ctx context.Context, id string, remove bool, actors ...string) error {
	if !validID(id) {
		return ErrUnauthorized
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	k, e := scanKey(tx.QueryRow(ctx, `SELECT `+columns+` FROM external_api_keys WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrUnauthorized
	}
	if e != nil {
		return e
	}
	action := "revoked"
	if remove {
		if k.RevokedAt == nil {
			return ErrDelete
		}
		if _, e = tx.Exec(ctx, `DELETE FROM external_api_calls WHERE key_id=$1`, id); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `DELETE FROM external_api_keys WHERE id=$1`, id)
		action = "deleted"
	} else {
		_, e = tx.Exec(ctx, `UPDATE external_api_keys SET revoked_at=coalesce(revoked_at,now())WHERE id=$1`, id)
	}
	if e != nil {
		return e
	}
	actor := ""
	if len(actors) > 0 {
		actor = actors[0]
	}
	if e = audit(ctx, tx, actor, action, k); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (r Repository) Admit(ctx context.Context, token, scope, method, path string) (string, int64, error) {
	claims, e := r.Signer.Verify(token)
	if e != nil {
		return "", 0, e
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return "", 0, ErrUnavailable
	}
	defer deferutil.Rollback(ctx, tx)
	k, e := scanKey(tx.QueryRow(ctx, `SELECT `+columns+` FROM external_api_keys WHERE id=$1 AND revoked_at IS NULL FOR UPDATE`, claims.ID))
	if errors.Is(e, pgx.ErrNoRows) {
		return "", 0, ErrUnauthorized
	}
	if e != nil {
		return "", 0, ErrUnavailable
	}
	if k.CreatedAt.Unix() != claims.IssuedAt || !slices.Equal(k.Scopes, claims.Scopes) {
		return "", 0, ErrUnauthorized
	}
	if !slices.Contains(k.Scopes, scope) {
		return "", 0, ErrForbidden
	}
	var n int
	if e = tx.QueryRow(ctx, `SELECT count(*)FROM external_api_calls WHERE key_id=$1 AND created_at>now()-interval '1 minute'`, k.ID).Scan(&n); e != nil {
		return "", 0, ErrUnavailable
	}
	if n >= 120 {
		return "", 0, ErrRate
	}
	var call int64
	if e = tx.QueryRow(ctx, `INSERT INTO external_api_calls(key_id,credential_name,method,path,status)VALUES($1,$2,$3,$4,0)RETURNING id`, k.ID, k.Name, method, path).Scan(&call); e != nil {
		return "", 0, ErrUnavailable
	}
	if _, e = tx.Exec(ctx, `UPDATE external_api_keys SET last_used_at=now()WHERE id=$1`, k.ID); e != nil {
		return "", 0, ErrUnavailable
	}
	if e = tx.Commit(ctx); e != nil {
		return "", 0, ErrUnavailable
	}
	return k.ID, call, nil
}
