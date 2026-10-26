package playeraccess

import (
	"context"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/clientip"
	"time"
)

type Event struct {
	ID             string    `json:"id"`
	ObservationID  string    `json:"observationId"`
	UserAgent      string    `json:"userAgent"`
	ClientIP       string    `json:"clientIp"`
	ConnectionIP   string    `json:"connectionIp"`
	IPSource       string    `json:"ipSource"`
	IPPublic       bool      `json:"ipPublic"`
	Entry          string    `json:"entry"`
	Method         string    `json:"method"`
	Allowed        bool      `json:"allowed"`
	Reason         string    `json:"reason"`
	PolicyRevision int64     `json:"policyRevision"`
	Status         int       `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
}
type EventPage struct {
	Items      []Event `json:"items"`
	Total      int     `json:"total"`
	Page       int     `json:"page"`
	PageSize   int     `json:"pageSize"`
	MaxRecords int     `json:"maxRecords"`
}

func (r Repository) RecordEvent(ctx context.Context, in Request, d Decision) (string, error) {
	in.ClientIP = clientip.Sanitize(in.ClientIP)
	in.ConnectionIP = clientip.Sanitize(in.ConnectionIP)
	if r.DB == nil {
		return "", ErrUnavailable
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `DELETE FROM player_access_events WHERE id IN(SELECT id FROM player_access_events ORDER BY created_at,id LIMIT greatest((SELECT count(*) FROM player_access_events)-10000+1,0) FOR UPDATE SKIP LOCKED)`); err != nil {
		return "", ErrUnavailable
	}
	id := jobs.UUID()
	_, err = tx.Exec(ctx, `INSERT INTO player_access_events(id,observation_id,user_agent,client_ip,entry,method,allowed,reason,policy_revision,connection_ip,ip_source,ip_public)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, ObservationID(in.UserAgent), in.UserAgent, in.ClientIP, in.Entry, in.Method, d.Allowed, d.Reason, d.PolicyRevision, in.ConnectionIP, in.IPSource, in.IPPublic)
	if err != nil {
		return "", ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return "", ErrUnavailable
	}
	return id, nil
}
func (r Repository) FinishEvent(ctx context.Context, id string, status int) error {
	if r.DB == nil || !validID(id) {
		return ErrInvalid
	}

	_, err := r.DB.Exec(ctx, `UPDATE player_access_events SET status=$2 WHERE id=$1`, id, status)
	return err
}
func (r Repository) Events(ctx context.Context, q, observation string, page, size int) (EventPage, error) {
	return r.FilteredEvents(ctx, q, observation, page, size)
}
func (r Repository) FilteredEvents(ctx context.Context, q, observation string, page, size int) (EventPage, error) {
	out := EventPage{Items: []Event{}, Page: page, PageSize: size, MaxRecords: 10000}
	if r.DB == nil {
		return out, ErrUnavailable
	}
	if len(q) > 200 || page < 1 || size < 1 || size > 100 || observation != "" && !validObservationID(observation) {
		return out, ErrInvalid
	}
	where := ` WHERE ($1='' OR position(lower($1) IN lower(user_agent||' '||client_ip))>0)AND($2=''OR observation_id=$2)`
	if err := r.DB.QueryRow(ctx, `SELECT count(*) FROM player_access_events`+where, q, observation).Scan(&out.Total); err != nil {
		return out, ErrUnavailable
	}
	rows, err := r.DB.Query(ctx, `SELECT id::text,observation_id,user_agent,client_ip,entry,method,allowed,reason,policy_revision,status,created_at,connection_ip,ip_source,ip_public FROM player_access_events`+where+` ORDER BY created_at DESC,id LIMIT $3 OFFSET $4`, q, observation, size, (page-1)*size)
	if err != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var e Event
		if err = rows.Scan(&e.ID, &e.ObservationID, &e.UserAgent, &e.ClientIP, &e.Entry, &e.Method, &e.Allowed, &e.Reason, &e.PolicyRevision, &e.Status, &e.CreatedAt, &e.ConnectionIP, &e.IPSource, &e.IPPublic); err != nil {
			return out, ErrUnavailable
		}
		e.ClientIP = clientip.Sanitize(e.ClientIP)
		e.ConnectionIP = clientip.Sanitize(e.ConnectionIP)
		out.Items = append(out.Items, e)
	}
	return out, rows.Err()
}
