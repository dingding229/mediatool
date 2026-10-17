package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	deferutil "media-tool/internal/platform/cleanup"
	"time"
)

var ErrLeaseLost = errors.New("job lease lost")
var ErrOperatorRequired = errors.New("explicit operator action required")

type Job struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	State       string          `json:"state"`
	Payload     json.RawMessage `json:"payload"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"maxAttempts"`
	LastError   string          `json:"lastError,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	LeaseToken  string          `json:"-"`
}
type Repository struct{ DB *pgxpool.Pool }

func UUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[:8], s[8:12], s[12:16], s[16:20], s[20:])
}
func (r Repository) Create(ctx context.Context, kind string, payload json.RawMessage) (Job, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer deferutil.Rollback(ctx, tx)
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	j := Job{ID: UUID(), Kind: kind, Payload: payload, State: "pending", MaxAttempts: 3}
	err = tx.QueryRow(ctx, `INSERT INTO jobs(id,kind,payload) VALUES($1,$2,$3) RETURNING created_at`, j.ID, kind, payload).Scan(&j.CreatedAt)
	if err != nil {
		return Job{}, err
	}
	body, _ := json.Marshal(map[string]string{"kind": kind})
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload) VALUES($1,'job.queued',$2,$3)`, UUID(), j.ID, body); err != nil {
		return Job{}, err
	}
	return j, tx.Commit(ctx)
}
func (r Repository) List(ctx context.Context) ([]Job, error) {
	rows, err := r.DB.Query(ctx, `SELECT id::text,kind,state,payload,attempts,max_attempts,coalesce(last_error,''),created_at FROM jobs ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Job{}
	for rows.Next() {
		var j Job
		if err = rows.Scan(&j.ID, &j.Kind, &j.State, &j.Payload, &j.Attempts, &j.MaxAttempts, &j.LastError, &j.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, j)
	}
	return list, rows.Err()
}
func (r Repository) Claim(ctx context.Context, owner string, lease time.Duration) (*Job, error) {
	return r.ClaimQueue(ctx, owner, lease, "default")
}
func (r Repository) ClaimQueue(ctx context.Context, owner string, lease time.Duration, queue string) (*Job, error) {
	if queue == "" {
		queue = "default"
	}
	if err := r.expireExhausted(ctx); err != nil {
		return nil, err
	}
	var err error
	token := UUID()
	var j Job
	err = r.DB.QueryRow(ctx, `WITH candidate AS (
 SELECT id FROM jobs WHERE queue=$4 AND (kind LIKE 'ops.%' OR NOT (SELECT paused FROM ops_mode WHERE singleton)) AND attempts<max_attempts AND
 ((state IN ('pending','retry') AND available_at<=now()) OR (state='running' AND lease_until<now()))
 ORDER BY available_at,created_at FOR UPDATE SKIP LOCKED LIMIT 1
 ) UPDATE jobs j SET state='running',started_at=now(),attempts=j.attempts+1,locked_by=$1,lease_token=$2,
 lease_until=now()+($3::bigint*interval '1 millisecond'),updated_at=now()
 FROM candidate c WHERE j.id=c.id RETURNING j.id::text,j.kind,j.state,j.payload,j.attempts,j.max_attempts,coalesce(j.last_error,''),j.created_at,j.lease_token::text`, owner, token, lease.Milliseconds(), queue).Scan(&j.ID, &j.Kind, &j.State, &j.Payload, &j.Attempts, &j.MaxAttempts, &j.LastError, &j.CreatedAt, &j.LeaseToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}
func (r Repository) Heartbeat(ctx context.Context, j Job, lease time.Duration) error {
	tag, err := r.DB.Exec(ctx, `UPDATE jobs SET lease_until=now()+($3::bigint*interval '1 millisecond'),updated_at=now() WHERE id=$1 AND lease_token=$2 AND state='running' AND lease_until>now()`, j.ID, j.LeaseToken, lease.Milliseconds())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}
func (r Repository) Finish(ctx context.Context, j Job, workErr error) error {
	state := "succeeded"
	message := ""
	delay := 0
	if workErr != nil {
		state = "retry"
		if j.Attempts >= j.MaxAttempts {
			state = "failed"
		}
		if errors.Is(workErr, ErrOperatorRequired) {
			state = "paused"
		}
		message = workErr.Error()
		if len(message) > 1000 {
			message = message[:1000]
		}
		delay = 1 << min(j.Attempts, 8)
		if state == "paused" {
			delay = 0
		}
		if j.Kind == "emby.confirm" {
			delay = min(delay, 60)
		}
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	tag, err := tx.Exec(ctx, `UPDATE jobs SET
 state=CASE WHEN $3='succeeded' AND payload->>'rerun'='true' THEN 'pending' ELSE $3 END,
 attempts=CASE WHEN $3='succeeded' AND payload->>'rerun'='true' THEN 0 ELSE attempts END,
 payload=CASE WHEN $3='succeeded' AND payload->>'rerun'='true' THEN payload-'rerun' ELSE payload END,
 last_error=NULLIF($4,''),available_at=CASE WHEN $3='succeeded' AND payload->>'rerun'='true' THEN now() ELSE now()+($5::integer*interval '1 second') END,
 lease_until=NULL,lease_token=NULL,locked_by=NULL,updated_at=now()
 WHERE id=$1 AND lease_token=$2 AND state='running' AND lease_until>now()`, j.ID, j.LeaseToken, state, message, delay)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	var persistedState string
	if err = tx.QueryRow(ctx, `SELECT state FROM jobs WHERE id=$1`, j.ID).Scan(&persistedState); err != nil {
		return err
	}
	body, marshalErr := json.Marshal(map[string]string{"kind": j.Kind, "state": persistedState})
	if marshalErr != nil {
		return marshalErr
	}
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload) VALUES($1,$2,$3,$4)`, UUID(), "job."+persistedState, j.ID, body); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r Repository) expireExhausted(ctx context.Context) error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	rows, err := tx.Query(ctx, `UPDATE jobs SET state='failed',last_error='lease expired after final attempt',lease_until=NULL,lease_token=NULL,locked_by=NULL,updated_at=now() WHERE state='running' AND lease_until<now() AND attempts>=max_attempts RETURNING id::text,kind`)
	if err != nil {
		return err
	}
	type expired struct{ id, kind string }
	out := []expired{}
	for rows.Next() {
		var e expired
		if err = rows.Scan(&e.id, &e.kind); err != nil {
			rows.Close()
			return err
		}
		out = append(out, e)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	for _, e := range out {
		body, _ := json.Marshal(map[string]string{"kind": e.kind, "state": "failed"})
		if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_id,payload)VALUES($1,'job.failed',$2,$3)`, UUID(), e.id, body); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
