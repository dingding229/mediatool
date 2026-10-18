package sitemetrics

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/sites"
)

type Summary struct {
	ID, Name, State, Origin, TemplateID string
	Enabled                             bool
	Revision                            int64
	Metrics                             sites.Metrics
}

// Summaries reads only bounded public columns, never encrypted account data or raw errors.
func (e Engine) Summaries(ctx context.Context, start, limit int, query, id string, siteIDs []string) ([]Summary, error) {
	if start < 0 || start > 200000 || limit < 1 || limit > 200 || len(query) > 800 {
		return nil, errors.New("invalid page")
	}
	rows, err := e.DB.Query(ctx, `SELECT i.id::text,i.name,i.enabled,i.revision,coalesce(s.state,'not_synced'),coalesce(s.metrics,'{}'::jsonb),coalesce(i.settings->>'endpoint',''),coalesce(i.settings->>'definitionId','') FROM integrations i LEFT JOIN site_metrics s ON s.account_id=i.id WHERE i.kind='ptsite' AND ($3='' OR i.name ILIKE '%'||$3||'%')AND($4='' OR i.id::text=$4)AND(coalesce(cardinality($5::text[]),0)=0 OR i.id::text=ANY($5))ORDER BY i.name,i.id LIMIT $1 OFFSET $2`, limit, start, query, id, siteIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var v Summary
		var raw []byte
		if err = rows.Scan(&v.ID, &v.Name, &v.Enabled, &v.Revision, &v.State, &raw, &v.Origin, &v.TemplateID); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &v.Metrics); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
