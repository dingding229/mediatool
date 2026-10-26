package externalapi

import (
	"context"
	"time"
)

type Call struct {
	KeyID     string    `json:"keyId"`
	Name      string    `json:"name"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	ClientIP  string    `json:"clientIP"`
	IPSource  string    `json:"ipSource"`
	CreatedAt time.Time `json:"createdAt"`
}

func (r Repository) Calls(ctx context.Context, id string, limit int) ([]Call, error) {
	if id != "" && !validID(id) {
		return nil, ErrUnauthorized
	}
	if limit < 1 || limit > 100 {
		return nil, ErrUnauthorized
	}
	rows, e := r.DB.Query(ctx, `SELECT coalesce(c.key_id::text,''),coalesce(k.name,nullif(c.credential_name,''),'旧凭据'),c.method,c.path,c.status,coalesce(host(c.client_ip),''),c.ip_source,c.created_at FROM external_api_calls c LEFT JOIN external_api_keys k ON k.id=c.key_id WHERE($1=''OR c.key_id=nullif($1,'')::uuid)ORDER BY c.created_at DESC,c.id DESC LIMIT $2`, id, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Call{}
	for rows.Next() {
		var v Call
		if e = rows.Scan(&v.KeyID, &v.Name, &v.Method, &v.Path, &v.Status, &v.ClientIP, &v.IPSource, &v.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
