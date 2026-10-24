package playeraccess

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/clientip"
	"strings"
)

type Repository struct{ DB *pgxpool.Pool }

// A bounded transaction serializes administrator policy mutations and cleanup.
// Playback decisions and event recording must not acquire this global lock.
func (r Repository) begin(ctx context.Context) (pgx.Tx, error) {
	if r.DB == nil {
		return nil, ErrUnavailable
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='1s'; SELECT pg_advisory_xact_lock(745)`); err != nil {
		deferutil.Rollback(ctx, tx)
		return nil, ErrUnavailable
	}
	return tx, nil
}
func readPolicy(ctx context.Context, tx pgx.Tx) (Policy, error) {
	p := Policy{Rules: []Rule{}, BlacklistRules: []Rule{}}
	if err := tx.QueryRow(ctx, `SELECT enabled,mode,revision,updated_at FROM player_ua_policy WHERE id='main'`).Scan(&p.Enabled, &p.Mode, &p.Revision, &p.UpdatedAt); err != nil {
		return p, ErrUnavailable
	}
	rows, err := tx.Query(ctx, `SELECT id::text,label,match_type,user_agent,enabled,list_type FROM player_ua_rules ORDER BY list_type,match_type,label,id`)
	if err != nil {
		return p, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var rule Rule
		var list string
		if err = rows.Scan(&rule.ID, &rule.Label, &rule.MatchType, &rule.UserAgent, &rule.Enabled, &list); err != nil {
			return p, ErrUnavailable
		}
		if list == "blacklist" {
			p.BlacklistRules = append(p.BlacklistRules, rule)
		} else {
			p.Rules = append(p.Rules, rule)
		}
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
	defer deferutil.Rollback(ctx, tx)
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
	// Both lists share one revision and transaction. IDs may not move between
	// lists; identical UA patterns in separate lists are intentional and valid.
	in.Mode = in.EffectiveMode()
	in.Enabled = in.Mode != "off"
	if in.Rules == nil {
		in.Rules = current.Rules
	}
	if in.BlacklistRules == nil {
		in.BlacklistRules = current.BlacklistRules
	}
	if err = in.Validate(); err != nil {
		return Policy{}, err
	}
	known := map[string]string{}
	for _, rule := range current.Rules {
		known[rule.ID] = "whitelist"
	}
	for _, rule := range current.BlacklistRules {
		known[rule.ID] = "blacklist"
	}
	if _, err = tx.Exec(ctx, `DELETE FROM player_ua_rules`); err != nil {
		return Policy{}, ErrUnavailable
	}
	for list, rules := range map[string][]Rule{"whitelist": in.Rules, "blacklist": in.BlacklistRules} {
		for i := range rules {
			if rules[i].ID == "" {
				rules[i].ID = jobs.UUID()
			} else if known[rules[i].ID] != list {
				return Policy{}, ErrInvalid
			}
			v := rules[i]
			if _, err = tx.Exec(ctx, `INSERT INTO player_ua_rules(id,label,match_type,user_agent,enabled,list_type)VALUES($1,$2,$3,$4,$5,$6)`, v.ID, v.Label, v.MatchType, v.UserAgent, v.Enabled, list); err != nil {
				return Policy{}, ErrUnavailable
			}
		}
	}
	if err = tx.QueryRow(ctx, `UPDATE player_ua_policy SET enabled=$1,mode=$2,revision=revision+1,updated_at=now() WHERE id='main' RETURNING revision,updated_at`, in.Enabled, in.Mode).Scan(&in.Revision, &in.UpdatedAt); err != nil {
		return Policy{}, ErrUnavailable
	}
	if err = audit(ctx, tx, "player_ua.policy_saved", in); err != nil {
		return Policy{}, ErrUnavailable
	}
	return in, nil
}
func (r Repository) Save(ctx context.Context, in Policy) (Policy, error) {
	if in.Revision < 1 {
		return Policy{}, ErrInvalid
	}
	tx, err := r.begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer deferutil.Rollback(ctx, tx)
	out, err := replace(ctx, tx, in)
	if err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, ErrUnavailable
	}
	return out, nil
}

type policySnapshotReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func playbackPolicySnapshot(ctx context.Context, q policySnapshotReader) (Policy, error) {
	p := Policy{Rules: []Rule{}, BlacklistRules: []Rule{}}
	var raw, black []byte
	if err := q.QueryRow(ctx, `SELECT enabled,mode,revision,updated_at,
 coalesce((SELECT jsonb_agg(jsonb_build_object('id',id,'label',label,'matchType',match_type,'userAgent',user_agent,'enabled',enabled) ORDER BY match_type,label,id) FROM player_ua_rules WHERE list_type='whitelist'),'[]'::jsonb),
 coalesce((SELECT jsonb_agg(jsonb_build_object('id',id,'label',label,'matchType',match_type,'userAgent',user_agent,'enabled',enabled) ORDER BY match_type,label,id) FROM player_ua_rules WHERE list_type='blacklist'),'[]'::jsonb)
 FROM player_ua_policy WHERE id='main'`).Scan(&p.Enabled, &p.Mode, &p.Revision, &p.UpdatedAt, &raw, &black); err != nil {
		return p, ErrUnavailable
	}
	if json.Unmarshal(raw, &p.Rules) != nil || json.Unmarshal(black, &p.BlacklistRules) != nil {
		return p, ErrUnavailable
	}
	return p, nil
}

func (r Repository) Check(ctx context.Context, in Request) (Decision, error) {
	in.ClientIP = clientip.Sanitize(in.ClientIP)
	in.ConnectionIP = clientip.Sanitize(in.ConnectionIP)
	if !ValidUA(in.UserAgent) || len(in.Entry) > 16 || len(in.Method) > 16 || len(in.ClientIP) > 100 {
		return Decision{}, ErrInvalid
	}
	if r.DB == nil {
		return Decision{}, ErrUnavailable
	}
	// One SQL statement gives the policy and its rules the same MVCC snapshot,
	// without a global lock or a long-lived repeatable-read transaction around
	// the per-UA upsert (which would cause concurrent serialization failures).
	p, err := playbackPolicySnapshot(ctx, r.DB)
	if err != nil {
		return Decision{}, err
	}
	d := p.Decide(in.UserAgent)
	id := ObservationID(in.UserAgent)
	_, err = r.DB.Exec(ctx, `INSERT INTO player_ua_observations(id,user_agent,last_allowed,last_reason,last_entry,last_method,last_client_ip,last_rule_id,last_policy_revision,allowed_count,denied_count)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,CASE WHEN $3 THEN 1 ELSE 0 END,CASE WHEN $3 THEN 0 ELSE 1 END)ON CONFLICT(id)DO UPDATE SET last_seen_at=now(),request_count=player_ua_observations.request_count+1,allowed_count=player_ua_observations.allowed_count+EXCLUDED.allowed_count,denied_count=player_ua_observations.denied_count+EXCLUDED.denied_count,last_allowed=EXCLUDED.last_allowed,last_reason=EXCLUDED.last_reason,last_entry=EXCLUDED.last_entry,last_method=EXCLUDED.last_method,last_client_ip=EXCLUDED.last_client_ip,last_rule_id=EXCLUDED.last_rule_id,last_policy_revision=EXCLUDED.last_policy_revision`, id, in.UserAgent, d.Allowed, d.Reason, in.Entry, in.Method, in.ClientIP, d.RuleID, p.Revision)
	if err != nil {
		return d, ErrUnavailable
	}
	// Keep the aggregate bounded without serializing decisions. Concurrent
	// cleanup may remove the same oldest row, which is harmless.
	if _, err = r.DB.Exec(ctx, `DELETE FROM player_ua_observations WHERE id IN(SELECT id FROM player_ua_observations ORDER BY last_seen_at,id LIMIT greatest((SELECT count(*) FROM player_ua_observations)-$1,0) FOR UPDATE SKIP LOCKED)`, MaxObservations); err != nil {
		return d, ErrUnavailable
	}
	return d, nil
}
func (r Repository) AllowObservation(ctx context.Context, id, label string, revision int64) (Policy, error) {
	return r.AddObservation(ctx, id, label, revision, "whitelist")
}
func (r Repository) AddObservation(ctx context.Context, id, label string, revision int64, list string) (Policy, error) {
	if !validObservationID(id) || revision < 1 || list != "whitelist" && list != "blacklist" {
		return Policy{}, ErrInvalid
	}
	tx, err := r.begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer deferutil.Rollback(ctx, tx)
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
	rules := &p.Rules
	if list == "blacklist" {
		rules = &p.BlacklistRules
	}
	duplicate := false
	for i, r := range *rules {
		if r.MatchType == "exact" && r.UserAgent == ua {
			(*rules)[i].Enabled = true
			if label != "" {
				(*rules)[i].Label = label
			}
			duplicate = true
		}
	}
	if !duplicate {
		*rules = append(*rules, Rule{Label: label, MatchType: "exact", UserAgent: ua, Enabled: true})
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
	if len(q) > 200 || page < 1 || page > 20000 || size < 1 || size > 100 || filter != "" && filter != "whitelisted" && filter != "unlisted" && filter != "blacklisted" {
		return out, ErrInvalid
	}
	tx, err := r.begin(ctx)
	if err != nil {
		return out, err
	}
	defer deferutil.Rollback(ctx, tx)
	p, err := readPolicy(ctx, tx)
	if err != nil {
		return out, err
	}
	out.PolicyRevision = p.Revision
	matching := `EXISTS(SELECT 1 FROM player_ua_rules r WHERE r.enabled AND r.list_type='whitelist' AND o.user_agent<>'' AND (r.match_type='exact' AND r.user_agent=o.user_agent OR r.match_type='prefix' AND left(o.user_agent,length(r.user_agent))=r.user_agent OR r.match_type='contains' AND position(r.user_agent in o.user_agent)>0))`
	blackMatching := strings.ReplaceAll(matching, "r.list_type='whitelist'", "r.list_type='blacklist'")
	where := ` WHERE ($1='' OR position(lower($1) in lower(o.user_agent))>0) AND ($2='' OR ($2='whitelisted' AND ` + matching + `) OR ($2='unlisted' AND NOT ` + matching + `) OR ($2='blacklisted' AND ` + blackMatching + `))`
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
		o.Blacklisted = Match(p.BlacklistRules, o.UserAgent) != ""
		o.LastClientIP = clientip.Sanitize(o.LastClientIP)
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
	defer deferutil.Rollback(ctx, tx)
	p, err := readPolicy(ctx, tx)
	if err != nil {
		return err
	}
	if p.Revision != revision {
		return ErrConflict
	}
	if _, err = tx.Exec(ctx, `DELETE FROM player_access_events`); err != nil {
		return ErrUnavailable
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
