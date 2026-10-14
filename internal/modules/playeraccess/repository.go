package playeraccess

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
)

type Repository struct{ DB *pgxpool.Pool }

// A bounded transaction serializes decisions, aggregates and administrator
// changes. It is always released BEFORE any provider/network/stream work.
func (r Repository) begin(ctx context.Context) (pgx.Tx, error) {
	if r.DB == nil {
		return nil, ErrUnavailable
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='1s'; SELECT pg_advisory_xact_lock(745)`); err != nil {
		tx.Rollback(ctx)
		return nil, ErrUnavailable
	}
	return tx, nil
}
func readPolicy(ctx context.Context, tx pgx.Tx) (Policy, error) {
	p := Policy{Rules: []Rule{}}
	if err := tx.QueryRow(ctx, `SELECT enabled,revision,updated_at FROM player_ua_policy WHERE id='main'`).Scan(&p.Enabled, &p.Revision, &p.UpdatedAt); err != nil {
		return p, ErrUnavailable
	}
	rows, err := tx.Query(ctx, `SELECT id::text,label,match_type,user_agent,enabled FROM player_ua_rules ORDER BY match_type,label,id`)
	if err != nil {
		return p, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var rule Rule
		if err = rows.Scan(&rule.ID, &rule.Label, &rule.MatchType, &rule.UserAgent, &rule.Enabled); err != nil {
			return p, ErrUnavailable
		}
		p.Rules = append(p.Rules, rule)
	}
	if rows.Err() != nil {
		return p, ErrUnavailable
	}
	return p, nil
}
func (r Repository) Policy(ctx context.Context) (Policy, error) {
	tx, err := r.begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback(ctx)
	p, err := readPolicy(ctx, tx)
	if err != nil {
		return p, err
	}
	if err = tx.Commit(ctx); err != nil {
		return p, ErrUnavailable
	}
	return p, nil
}
func audit(ctx context.Context, tx pgx.Tx, action string, details any) error {
	raw, _ := json.Marshal(details)
	_, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator',$1,$2)`, action, raw)
	return err
}
func replace(ctx context.Context, tx pgx.Tx, in Policy) (Policy, error) {
	current, err := readPolicy(ctx, tx)
	if err != nil {
		return Policy{}, err
	}
	if current.Revision != in.Revision {
		return Policy{}, ErrConflict
	}
	// IDs are server-assigned for new rules; existing IDs must belong to this policy.
	known := map[string]bool{}
	for _, rule := range current.Rules {
		known[rule.ID] = true
	}
	for i := range in.Rules {
		if in.Rules[i].ID == "" {
			in.Rules[i].ID = jobs.UUID()
		} else if !known[in.Rules[i].ID] {
			return Policy{}, ErrInvalid
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM player_ua_rules`); err != nil {
		return Policy{}, ErrUnavailable
	}
	for _, r := range in.Rules {
		if _, err = tx.Exec(ctx, `INSERT INTO player_ua_rules(id,label,match_type,user_agent,enabled)VALUES($1,$2,$3,$4,$5)`, r.ID, r.Label, r.MatchType, r.UserAgent, r.Enabled); err != nil {
			return Policy{}, ErrUnavailable
		}
	}
	if err = tx.QueryRow(ctx, `UPDATE player_ua_policy SET enabled=$1,revision=revision+1,updated_at=now() WHERE id='main' RETURNING revision,updated_at`, in.Enabled).Scan(&in.Revision, &in.UpdatedAt); err != nil {
		return Policy{}, ErrUnavailable
	}
	if err = audit(ctx, tx, "player_ua.policy_saved", in); err != nil {
		return Policy{}, ErrUnavailable
	}
	return in, nil
}
func (r Repository) Save(ctx context.Context, in Policy) (Policy, error) {
	if err := in.Validate(); err != nil {
		return Policy{}, err
	}
	tx, err := r.begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback(ctx)
	out, err := replace(ctx, tx, in)
	if err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, ErrUnavailable
	}
	return out, nil
}
func (r Repository) Check(ctx context.Context, in Request) (Decision, error) {
	if !ValidUA(in.UserAgent) || len(in.Entry) > 16 || len(in.Method) > 16 || len(in.ClientIP) > 100 {
		return Decision{}, ErrInvalid
	}
	tx, err := r.begin(ctx)
	if err != nil {
		return Decision{}, err
	}
	defer tx.Rollback(ctx)
	p, err := readPolicy(ctx, tx)
	if err != nil {
		return Decision{}, err
	}
	d := p.Decide(in.UserAgent)
	id := ObservationID(in.UserAgent)
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM player_ua_observations WHERE id=$1)`, id).Scan(&exists); err != nil {
		return d, ErrUnavailable
	}
	if !exists {
		// Drop only oldest aggregate rows, never rules, playback grants or raw logs.
		if _, err = tx.Exec(ctx, `DELETE FROM player_ua_observations WHERE id IN(SELECT id FROM player_ua_observations ORDER BY last_seen_at,id LIMIT greatest((SELECT count(*) FROM player_ua_observations)-$1+1,0))`, MaxObservations); err != nil {
			return d, ErrUnavailable
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO player_ua_observations(id,user_agent,last_allowed,last_reason,last_entry,last_method,last_client_ip,last_rule_id,last_policy_revision,allowed_count,denied_count)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,CASE WHEN $3 THEN 1 ELSE 0 END,CASE WHEN $3 THEN 0 ELSE 1 END)ON CONFLICT(id)DO UPDATE SET last_seen_at=now(),request_count=player_ua_observations.request_count+1,allowed_count=player_ua_observations.allowed_count+EXCLUDED.allowed_count,denied_count=player_ua_observations.denied_count+EXCLUDED.denied_count,last_allowed=EXCLUDED.last_allowed,last_reason=EXCLUDED.last_reason,last_entry=EXCLUDED.last_entry,last_method=EXCLUDED.last_method,last_client_ip=EXCLUDED.last_client_ip,last_rule_id=EXCLUDED.last_rule_id,last_policy_revision=EXCLUDED.last_policy_revision`, id, in.UserAgent, d.Allowed, d.Reason, in.Entry, in.Method, in.ClientIP, d.RuleID, p.Revision)
	if err != nil {
		return d, ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return d, ErrUnavailable
	}
	return d, nil
}
func (r Repository) AllowObservation(ctx context.Context, id, label string, revision int64) (Policy, error) {
	if !validObservationID(id) || revision < 1 {
		return Policy{}, ErrInvalid
	}
	tx, err := r.begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback(ctx)
	p, err := readPolicy(ctx, tx)
	if err != nil {
		return p, err
	}
	if p.Revision != revision {
		return p, ErrConflict
	}
	var ua string
	err = tx.QueryRow(ctx, `SELECT user_agent FROM player_ua_observations WHERE id=$1`, id).Scan(&ua)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, ErrUnavailable
	}
	duplicate := false
	for i, r := range p.Rules {
		if r.MatchType == "exact" && r.UserAgent == ua {
			p.Rules[i].Enabled = true
			if label != "" {
				p.Rules[i].Label = label
			}
			duplicate = true
		}
	}
	if !duplicate {
		p.Rules = append(p.Rules, Rule{Label: label, MatchType: "exact", UserAgent: ua, Enabled: true})
	}
	if err = p.Validate(); err != nil {
		return p, err
	}
	out, err := replace(ctx, tx, p)
	if err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, ErrUnavailable
	}
	return out, nil
}
func (r Repository) Observations(ctx context.Context, q, filter string, page, size int) (Page, error) {
	out := Page{Items: []Observation{}, Page: page, PageSize: size, MaxRecords: MaxObservations}
	if len(q) > 200 || page < 1 || page > 20000 || size < 1 || size > 100 || filter != "" && filter != "whitelisted" && filter != "unlisted" {
		return out, ErrInvalid
	}
	tx, err := r.begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	p, err := readPolicy(ctx, tx)
	if err != nil {
		return out, err
	}
	out.PolicyRevision = p.Revision
	matching := `EXISTS(SELECT 1 FROM player_ua_rules r WHERE r.enabled AND o.user_agent<>'' AND (r.match_type='exact' AND r.user_agent=o.user_agent OR r.match_type='prefix' AND left(o.user_agent,length(r.user_agent))=r.user_agent))`
	where := ` WHERE ($1='' OR position(lower($1) in lower(o.user_agent))>0) AND ($2='' OR ($2='whitelisted' AND ` + matching + `) OR ($2='unlisted' AND NOT ` + matching + `))`
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM player_ua_observations o`+where, q, filter).Scan(&out.Total); err != nil {
		return out, ErrUnavailable
	}
	rows, err := tx.Query(ctx, `SELECT id,user_agent,first_seen_at,last_seen_at,request_count,allowed_count,denied_count,last_allowed,last_reason,last_entry,last_method,last_client_ip,last_rule_id,last_policy_revision FROM player_ua_observations o`+where+` ORDER BY last_seen_at DESC,id LIMIT $3 OFFSET $4`, q, filter, size, (page-1)*size)
	if err != nil {
		return out, ErrUnavailable
	}
	for rows.Next() {
		var o Observation
		if err = rows.Scan(&o.ID, &o.UserAgent, &o.FirstSeenAt, &o.LastSeenAt, &o.RequestCount, &o.AllowedCount, &o.DeniedCount, &o.LastAllowed, &o.LastReason, &o.LastEntry, &o.LastMethod, &o.LastClientIP, &o.LastRuleID, &o.LastPolicyRevision); err != nil {
			rows.Close()
			return out, ErrUnavailable
		}
		o.MatchedRuleID = Match(p.Rules, o.UserAgent)
		o.Whitelisted = o.MatchedRuleID != ""
		out.Items = append(out.Items, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return out, ErrUnavailable
	}
	return out, nil
}
func (r Repository) Clear(ctx context.Context, confirm bool, revision int64) error {
	if !confirm {
		return ErrInvalid
	}
	tx, err := r.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	p, err := readPolicy(ctx, tx)
	if err != nil {
		return err
	}
	if p.Revision != revision {
		return ErrConflict
	}
	tag, err := tx.Exec(ctx, `DELETE FROM player_ua_observations`)
	if err != nil {
		return ErrUnavailable
	}
	if err = audit(ctx, tx, "player_ua.observations_cleared", map[string]any{"rows": tag.RowsAffected(), "policyRevision": p.Revision}); err != nil {
		return ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return ErrUnavailable
	}
	return nil
}
