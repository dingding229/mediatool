package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Detail struct {
	Job
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	Queue       string     `json:"queue"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	AvailableAt time.Time  `json:"availableAt"`
	LeaseUntil  *time.Time `json:"leaseUntil"`
	LockedBy    *string    `json:"lockedBy"`
}
type Filter struct {
	State, Kind, Queue, Query string
	Limit, Offset             int
}
type Page struct {
	Items []Detail `json:"items"`
	Total int      `json:"total"`
}

const detailColumns = `id::text,kind,state,payload,attempts,max_attempts,coalesce(last_error,'') AS last_error,created_at,queue,updated_at,available_at,lease_until,locked_by,started_at`

func (r Repository) Browse(ctx context.Context, f Filter) (Page, error) {
	out := Page{Items: []Detail{}}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || len(f.Query) > 200 || len(f.Kind) > 100 || len(f.Queue) > 100 {
		return out, errors.New("invalid task filter")
	}
	where := ` WHERE ($1='' OR state=$1) AND ($2='' OR kind=$2) AND ($3='' OR queue=$3) AND ($4='' OR strpos(lower(id::text||' '||kind||' '||coalesce(last_error,'')),lower($4))>0)`
	// A single statement gives the count and page the same MVCC snapshot, including empty pages.
	var raw []byte
	err := r.DB.QueryRow(ctx, `WITH filtered AS (SELECT `+detailColumns+` FROM jobs`+where+`), paged AS (SELECT * FROM filtered ORDER BY created_at DESC,id DESC LIMIT $5 OFFSET $6) SELECT (SELECT count(*) FROM filtered),coalesce((SELECT jsonb_agg(to_jsonb(paged)) FROM paged),'[]')`, f.State, f.Kind, f.Queue, f.Query, f.Limit, f.Offset).Scan(&out.Total, &raw)
	if err != nil {
		return out, err
	}
	// Row decoding is explicit: SQL column names intentionally do not become public JSON.
	var records []struct {
		ID, Kind, State, Queue string
		Payload                json.RawMessage
		Attempts               int
		MaxAttempts            int        `json:"max_attempts"`
		LastError              string     `json:"last_error"`
		CreatedAt              time.Time  `json:"created_at"`
		UpdatedAt              time.Time  `json:"updated_at"`
		AvailableAt            time.Time  `json:"available_at"`
		LeaseUntil             *time.Time `json:"lease_until"`
		LockedBy               *string    `json:"locked_by"`
		StartedAt              *time.Time `json:"started_at"`
	}
	if err = json.Unmarshal(raw, &records); err != nil {
		return out, err
	}
	for _, v := range records {
		out.Items = append(out.Items, Detail{Job: Job{ID: v.ID, Kind: v.Kind, State: v.State, Payload: v.Payload, Attempts: v.Attempts, MaxAttempts: v.MaxAttempts, LastError: v.LastError, CreatedAt: v.CreatedAt}, Queue: v.Queue, UpdatedAt: v.UpdatedAt, AvailableAt: v.AvailableAt, LeaseUntil: v.LeaseUntil, LockedBy: v.LockedBy, StartedAt: v.StartedAt})
	}
	return out, nil
}
func (r Repository) Detail(ctx context.Context, id string) (Detail, error) {
	var d Detail
	err := r.DB.QueryRow(ctx, `SELECT `+detailColumns+` FROM jobs WHERE id=$1`, id).Scan(&d.ID, &d.Kind, &d.State, &d.Payload, &d.Attempts, &d.MaxAttempts, &d.LastError, &d.CreatedAt, &d.Queue, &d.UpdatedAt, &d.AvailableAt, &d.LeaseUntil, &d.LockedBy, &d.StartedAt)
	return d, err
}
