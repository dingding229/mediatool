package v2

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"math/rand/v2"
	"time"

	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/jobs"
)

type Lease struct {
	ID, PluginID, Token, Owner, Fingerprint string
	GrantRevision                           int64
	Config                                  contracts.Config
	Scopes                                  []string
}

func (s Store) Claim(ctx context.Context, owner string) (Lease, error) {
	return s.claim(ctx, owner, nil)
}
func (s Store) claim(ctx context.Context, owner string, excluded []string) (Lease, error) {
	var l Lease
	if excluded == nil {
		excluded = []string{}
	}
	if !validUUID(owner) {
		return l, ErrInvalid
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return l, e
	}
	defer rollback(ctx, tx)
	e = tx.QueryRow(ctx, `SELECT i.id::text,i.plugin_id,i.approved_fingerprint,i.grant_revision FROM plugin_v2_instances i JOIN plugin_v2_definitions d ON d.id=i.plugin_id WHERE d.availability='available' AND d.fingerprint=i.approved_fingerprint AND i.desired_state='enabled' AND i.generation IS NULL AND i.worker_pid=0 AND NOT(i.id=ANY($1::uuid[])) AND i.next_restart_at<=now() AND i.runtime_state IN('stopped','backoff') ORDER BY i.next_restart_at,i.id FOR UPDATE OF i SKIP LOCKED LIMIT 1`, excluded).Scan(&l.ID, &l.PluginID, &l.Fingerprint, &l.GrantRevision)
	if e != nil {
		return l, e
	}
	d, e := definition(l.PluginID)
	if e != nil || d.Fingerprint != l.Fingerprint {
		return l, ErrConflict
	}
	l.Token, l.Owner = jobs.UUID(), owner
	if _, e = tx.Exec(ctx, `UPDATE plugin_v2_instances SET generation=$2,owner_id=$3,lease_until=now()+interval '30 seconds',runtime_state='starting',updated_at=now() WHERE id=$1`, l.ID, l.Token, owner); e != nil {
		return l, e
	}
	var cipher []byte
	if e = tx.QueryRow(ctx, `SELECT ciphertext,revision FROM plugin_v2_configs WHERE id=$1`, l.ID).Scan(&cipher, &l.Config.Revision); e != nil {
		return l, e
	}
	l.Config.Value, e = s.open("config", l.ID, cipher)
	if e != nil {
		return l, e
	}
	rows, e := tx.Query(ctx, `SELECT scope FROM plugin_v2_grants WHERE instance_id=$1 AND fingerprint=$2 ORDER BY scope`, l.ID, l.Fingerprint)
	if e != nil {
		return l, e
	}
	for rows.Next() {
		var scope string
		if e = rows.Scan(&scope); e != nil {
			break
		}
		l.Scopes = append(l.Scopes, scope)
	}
	e2 := rows.Err()
	rows.Close()
	if e != nil {
		return l, e
	}
	if e2 != nil {
		return l, e2
	}
	return l, tx.Commit(ctx)
}
func (s Store) Renew(ctx context.Context, l Lease, running bool) error {
	if running {
		defer invalidateNameHooks(s.DB)
	}
	state := Starting
	if running {
		state = Running
	}
	tag, e := s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET lease_until=now()+interval '30 seconds',runtime_state=$5,updated_at=now() WHERE id=$1 AND generation=$2 AND owner_id=$3 AND grant_revision=$4 AND desired_state='enabled' AND lease_until>now() AND runtime_state IN('starting','running')`, l.ID, l.Token, l.Owner, l.GrantRevision, state)
	if e == nil && tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return e
}
func RetryDelay(attempt int) time.Duration {
	seconds := []int{1, 5, 30, 120, 600}
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= len(seconds) {
		attempt = len(seconds) - 1
	}
	return time.Duration(seconds[attempt]) * time.Second
}
func (s Store) Finish(ctx context.Context, l Lease, why Reason) error {
	var n int
	e := s.DB.QueryRow(ctx, `SELECT CASE WHEN restart_window<now()-interval '1 hour' THEN 0 ELSE restart_count END FROM plugin_v2_instances WHERE id=$1 AND generation=$2 AND owner_id=$3`, l.ID, l.Token, l.Owner).Scan(&n)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrConflict
	}
	if e != nil {
		return e
	}
	delay := RetryDelay(n)
	delay += time.Duration(rand.Int64N(int64(delay/5) + 1))
	state := Backoff
	if n >= 9 {
		state = NeedsAction
	}
	tag, e := s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET generation=NULL,owner_id=NULL,lease_until=NULL,runtime_state=CASE WHEN reason IN('permission_review_required','definition_missing','definition_unsupported') THEN 'needs_action' WHEN runtime_state='stopping' OR desired_state<>'enabled' OR $5='' THEN 'stopped' ELSE $4 END,reason=CASE WHEN reason IN('permission_review_required','definition_missing','definition_unsupported') THEN reason WHEN runtime_state='stopping' OR desired_state<>'enabled' OR $5='' THEN '' ELSE $5 END,restart_count=CASE WHEN runtime_state='stopping' OR desired_state<>'enabled' OR $5='' THEN restart_count ELSE $7 END,restart_window=CASE WHEN restart_window<now()-interval '1 hour' THEN now() ELSE restart_window END,next_restart_at=CASE WHEN runtime_state='stopping' OR $5='' THEN now() ELSE now()+$6::interval END,updated_at=now() WHERE id=$1 AND generation=$2 AND owner_id=$3 AND lease_until>now()`, l.ID, l.Token, l.Owner, state, why, delay.String(), min(n+1, 10))
	if e == nil && tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return e
}
func (s Store) Reclaim(ctx context.Context) error {
	_, e := s.DB.Exec(ctx, `WITH stale AS(SELECT id FROM plugin_v2_instances WHERE lease_until<=now() ORDER BY lease_until,id FOR UPDATE SKIP LOCKED LIMIT 200) UPDATE plugin_v2_instances p SET generation=NULL,owner_id=NULL,lease_until=NULL,runtime_state=CASE WHEN reason IN('permission_review_required','definition_missing','definition_unsupported') THEN 'needs_action' WHEN desired_state<>'enabled' THEN 'stopped' WHEN restart_count>=9 THEN 'needs_action' ELSE 'backoff' END,restart_count=LEAST(restart_count+1,10),reason=CASE WHEN reason IN('permission_review_required','definition_missing','definition_unsupported') THEN reason ELSE 'worker_failed' END,next_restart_at=now()+interval '5 seconds' FROM stale WHERE p.id=stale.id`)
	return e
}
