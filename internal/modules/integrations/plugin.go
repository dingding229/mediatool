package integrations

import (
	"context"
	"time"
)

// Summary never decrypts credentials or returns settings/check messages.
type Summary struct {
	ID, Kind, Name, State string
	Enabled               bool
	Revision              int64
	CheckedAt             *time.Time
}

func (r Repository) Summaries(ctx context.Context, start, limit int, query, id string) ([]Summary, error) {
	if r.DB == nil || start < 0 || start > 200000 || limit < 1 || limit > 200 || len([]rune(query)) > 200 {
		return nil, ErrInvalid
	}
	rows, e := r.DB.Query(ctx, `SELECT id::text,kind,name,enabled,revision,last_check_state,last_checked_at FROM integrations WHERE($1=''OR position(lower($1)IN lower(name||' '||kind))>0)AND($2=''OR id::text=$2) ORDER BY kind,id LIMIT $3 OFFSET $4`, query, id, limit+1, start)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var v Summary
		if e = rows.Scan(&v.ID, &v.Kind, &v.Name, &v.Enabled, &v.Revision, &v.State, &v.CheckedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
