package v2

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"slices"
	"strings"
	"time"
)

var ErrAuthentication = errors.New("plugin authentication required")
var ErrRateLimited = errors.New("plugin API rate limited")

type APIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Token     string     `json:"token,omitempty"`
	Routes    []string   `json:"routes"`
	ExpiresAt time.Time  `json:"expiresAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

func (s Store) CreateAPIKey(ctx context.Context, actor, id, name string, routes []string, expires time.Time) (APIKey, error) {
	out := APIKey{ID: jobs.UUID(), Name: name, Routes: routes, ExpiresAt: expires}
	if len(name) < 1 || len(name) > 100 || len(routes) < 1 || len(routes) > 64 || !expires.After(time.Now()) || expires.After(time.Now().Add(366*24*time.Hour)) {
		return out, ErrInvalid
	}
	snap, e := s.Snapshot(ctx, id)
	if e != nil {
		return out, e
	}
	seen := map[string]bool{}
	for _, route := range routes {
		if seen[route] {
			return out, ErrInvalid
		}
		seen[route] = true
		parts := strings.Split(route, ":")
		if len(parts) != 2 {
			return out, ErrInvalid
		}
		_, d, e := s.Descriptor(ctx, id, "api", parts[0])
		if e != nil || !slices.Contains(d.Methods, parts[1]) || d.Auth != "apikey" && d.Auth != "bearer" {
			return out, ErrInvalid
		}
	}
	raw := make([]byte, 32)
	if _, e = rand.Read(raw); e != nil {
		return out, e
	}
	out.Token = "mt_plugin_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(out.Token))
	routeJSON, _ := json.Marshal(routes)
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,63))`, id); e != nil {
		return out, e
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_api_keys WHERE instance_id=$1 AND revoked_at IS NULL AND expires_at>now()`, id).Scan(&count); e != nil {
		return out, e
	}
	if count >= 32 {
		return out, ErrInvalid
	}
	tag, e := tx.Exec(ctx, `INSERT INTO plugin_v2_api_keys(id,instance_id,name,token_digest,fingerprint,grant_revision,routes,expires_at)SELECT $1,$2,$3,$4,i.approved_fingerprint,$5,$6,$7 FROM plugin_v2_instances i WHERE i.id=$2 AND i.grant_revision=$5 AND i.generation=$8 AND i.desired_state='enabled'`, out.ID, id, name, sum[:], snap.GrantRevision, routeJSON, expires, snap.Generation)
	if e != nil {
		return out, e
	}
	if tag.RowsAffected() != 1 {
		return APIKey{}, ErrConflict
	}
	details, _ := json.Marshal(map[string]string{"instanceId": id, "keyId": out.ID})
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,'plugins.v2.api-key.create',$2)`, actor, details); e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}
func (s Store) CheckAPIKey(ctx context.Context, id, route, method, token string, snap Snapshot) error {
	if len(token) != 74 {
		return ErrAuthentication
	}
	sum := sha256.Sum256([]byte(token))
	var allowed bool
	e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_v2_api_keys k JOIN plugin_v2_instances i ON i.id=k.instance_id WHERE k.instance_id=$1 AND k.token_digest=$2 AND k.routes ? $3 AND k.revoked_at IS NULL AND k.expires_at>now() AND k.fingerprint=i.approved_fingerprint AND k.grant_revision=$4 AND i.grant_revision=$4 AND i.generation=$5 AND i.desired_state='enabled')`, id, sum[:], route+":"+method, snap.GrantRevision, snap.Generation).Scan(&allowed)
	if e != nil {
		return e
	}
	if !allowed {
		return ErrAuthentication
	}
	return nil
}
func (s Store) RevokeAPIKey(ctx context.Context, actor, id, key string) error {
	if !validUUID(id) || !validUUID(key) {
		return ErrInvalid
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer rollback(ctx, tx)
	tag, e := tx.Exec(ctx, `UPDATE plugin_v2_api_keys SET revoked_at=coalesce(revoked_at,now()) WHERE id=$1 AND instance_id=$2`, key, id)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	details, _ := json.Marshal(map[string]string{"instanceId": id, "keyId": key})
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,'plugins.v2.api-key.revoke',$2)`, actor, details); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s Store) LimitAPI(ctx context.Context, id string) error {
	var used int
	e := s.DB.QueryRow(ctx, `INSERT INTO plugin_v2_api_limits(instance_id,bucket,used)VALUES($1,date_trunc('minute',clock_timestamp()),1) ON CONFLICT(instance_id)DO UPDATE SET bucket=excluded.bucket,used=CASE WHEN plugin_v2_api_limits.bucket=excluded.bucket THEN plugin_v2_api_limits.used+1 ELSE 1 END WHERE plugin_v2_api_limits.bucket<>excluded.bucket OR plugin_v2_api_limits.used<60 RETURNING used`, id).Scan(&used)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrRateLimited
	}
	return e
}
