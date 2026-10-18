package maintenance

import (
	"context"
	"time"
)

type AlertSummary struct {
	Code, State string
	Hits        int
	Value       *float64
	LastSeenAt  *time.Time
}

func (e Engine) AlertPage(ctx context.Context, start, limit int) ([]AlertSummary, error) {
	if e.DB == nil || start < 0 || limit < 1 || limit > 200 {
		return nil, ErrUnsafe
	}
	rows, err := e.DB.Query(ctx, `SELECT code,state,hits,value,last_seen_at FROM ops_alerts ORDER BY code LIMIT $1 OFFSET $2`, limit+1, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertSummary{}
	for rows.Next() {
		var v AlertSummary
		if err = rows.Scan(&v.Code, &v.State, &v.Hits, &v.Value, &v.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
