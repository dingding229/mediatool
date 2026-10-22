package subscriptions

import (
	"context"
	"time"
)

func (e Engine) Browse(ctx context.Context, history bool) ([]Record, error) {
	if !history {
		return e.List(ctx)
	}
	rows, err := e.DB.Query(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions WHERE archived_at IS NOT NULL ORDER BY archived_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type HistoryRun struct {
	ID         string     `json:"id"`
	Preview    bool       `json:"preview"`
	State      string     `json:"state"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

func (e Engine) History(ctx context.Context, id string) ([]HistoryRun, error) {
	if _, err := e.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := e.DB.Query(ctx, `SELECT id::text,preview,state,created_at,finished_at FROM subscription_runs WHERE subscription_id=$1 ORDER BY created_at DESC LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryRun{}
	for rows.Next() {
		var r HistoryRun
		if err = rows.Scan(&r.ID, &r.Preview, &r.State, &r.CreatedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (e Engine) BrowsePage(ctx context.Context, history bool, start, limit int, query, status string) ([]Record, error) {
	rows, err := e.DB.Query(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions WHERE (archived_at IS NOT NULL)=$1 AND ($4='' OR title ILIKE '%'||$4||'%') AND ($5='' OR $5='all' OR enabled=($5='enabled')) ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`, history, limit, start, query, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		v, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
