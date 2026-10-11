package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
)

type Engine struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
}
type Record struct {
	ID              string        `json:"id"`
	Title           string        `json:"title"`
	Media           metadata.Item `json:"media"`
	Enabled         bool          `json:"enabled"`
	Source          string        `json:"source"`
	IntervalMinutes int           `json:"intervalMinutes"`
	Rules           Rules         `json:"rules"`
	Revision        int64         `json:"revision"`
	LastState       string        `json:"lastState"`
	LastMessage     string        `json:"lastMessage"`
	NextRunAt       time.Time     `json:"nextRunAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

const subscriptionColumns = `id::text,title,identity,enabled,source,interval_seconds,rules,revision,last_state,last_message,next_run_at,updated_at`

func scanRecord(row pgx.Row) (Record, error) {
	var record Record
	var identity, rules []byte
	var interval int
	err := row.Scan(&record.ID, &record.Title, &identity, &record.Enabled, &record.Source, &interval, &rules, &record.Revision, &record.LastState, &record.LastMessage, &record.NextRunAt, &record.UpdatedAt)
	if err != nil {
		return record, errors.New("subscription unavailable")
	}
	if json.Unmarshal(identity, &record.Media) != nil || json.Unmarshal(rules, &record.Rules) != nil {
		return record, errors.New("invalid subscription record")
	}
	record.IntervalMinutes = interval / 60
	return record, nil
}
func (e Engine) Get(ctx context.Context, id string) (Record, error) {
	return scanRecord(e.DB.QueryRow(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions WHERE id=$1`, id))
}
func (e Engine) List(ctx context.Context) ([]Record, error) {
	rows, err := e.DB.Query(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}
func (e Engine) Save(ctx context.Context, id string, in Input) (Record, error) {
	if in.Source == "" {
		in.Source = "search"
	}
	if in.IntervalMinutes == 0 {
		in.IntervalMinutes = 30
	}
	if err := Validate(in); err != nil {
		return Record{}, err
	}
	for _, accountID := range in.Rules.AccountIDs {
		c, err := e.Connections.Repository.Get(ctx, accountID)
		if err != nil || c.Record.Kind != "ptsite" || !c.Record.Enabled {
			return Record{}, errors.New("select enabled PT accounts")
		}
		if in.Source != "search" && c.Values["rssUrl"] == "" {
			return Record{}, errors.New("RSS source requires an encrypted RSS URL on every selected account")
		}
	}
	client, err := e.Connections.TMDB(ctx)
	if err != nil {
		return Record{}, errors.New("configure TMDB before creating subscriptions")
	}
	item, err := client.Details(ctx, in.MediaType, in.TMDBID)
	if err != nil {
		return Record{}, errors.New("media identity lookup failed")
	}
	if in.Rules.Keyword == "" {
		in.Rules.Keyword = item.Title
	}
	identity, _ := json.Marshal(item)
	rules, _ := json.Marshal(in.Rules)
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	if id == "" {
		id = jobs.UUID()
		_, err = tx.Exec(ctx, `INSERT INTO subscriptions(id,title,media_type,identity,rules,enabled,source,interval_seconds)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, item.Title, in.MediaType, identity, rules, in.Enabled, in.Source, in.IntervalMinutes*60)
	} else {
		old, loadErr := scanRecord(tx.QueryRow(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions WHERE id=$1 FOR UPDATE`, id))
		if loadErr != nil {
			return Record{}, loadErr
		}
		if old.Revision != in.Revision {
			return Record{}, errors.New("subscription changed; reload before saving")
		}
		if old.Media.ID != item.ID || old.Media.MediaType != item.MediaType {
			return Record{}, errors.New("media identity is immutable; create a new subscription")
		}
		_, err = tx.Exec(ctx, `UPDATE subscriptions SET rules=$2,enabled=$3,source=$4,interval_seconds=$5,revision=revision+1,next_run_at=now(),updated_at=now()WHERE id=$1`, id, rules, in.Enabled, in.Source, in.IntervalMinutes*60)
	}
	if err != nil {
		return Record{}, err
	}
	audit, _ := json.Marshal(map[string]string{"subscriptionId": id})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','subscription.save',$1)`, audit); err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return e.Get(ctx, id)
}
func (e Engine) Queue(ctx context.Context, id string, preview bool) (string, error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	record, err := scanRecord(tx.QueryRow(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return "", err
	}
	var existing string
	if tx.QueryRow(ctx, `SELECT id::text FROM subscription_runs WHERE subscription_id=$1 AND state IN ('pending','running')`, id).Scan(&existing) == nil {
		return existing, nil
	}
	runID := jobs.UUID()
	if _, err = tx.Exec(ctx, `INSERT INTO subscription_runs(id,subscription_id,preview,revision)VALUES($1,$2,$3,$4)`, runID, id, preview, record.Revision); err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]string{"runId": runID})
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key,max_attempts)VALUES($1,'subscription.evaluate',$2,$3,5)`, jobs.UUID(), payload, "subscription.evaluate:"+runID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE subscriptions SET next_run_at=now()+($2::integer*interval '1 second'),last_state='pending',last_message=''WHERE id=$1`, id, record.IntervalMinutes*60); err != nil {
		return "", err
	}
	return runID, tx.Commit(ctx)
}
func (e Engine) Schedule(ctx context.Context) error {
	conn, err := e.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var acquired bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(74812911)`).Scan(&acquired); err != nil || !acquired {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(74812911)`)
	rows, err := e.DB.Query(ctx, `SELECT id::text FROM subscriptions WHERE enabled AND next_run_at<=now() ORDER BY next_run_at LIMIT 20`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if _, err = e.Queue(ctx, id, false); err != nil {
			return err
		}
	}
	return nil
}
func (e Engine) Delete(ctx context.Context, id string) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var active int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM subscription_runs WHERE subscription_id=$1 AND state IN ('pending','running')`, id).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return errors.New("subscription evaluation is active; disable and wait before deleting")
	}
	if _, err = tx.Exec(ctx, `UPDATE download_tasks SET subscription_id=NULL WHERE subscription_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM subscriptions WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
