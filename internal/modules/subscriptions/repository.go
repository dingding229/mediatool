package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"media-tool/internal/platform/scheduling"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/settings"
)

var ErrRunModeConflict = errors.New("已有待执行或执行中的订阅任务与本次预览/实际执行模式不同，请等待其结束后再操作")

func runModeConflict(existingPreview, requestedPreview bool) error {
	if existingPreview != requestedPreview {
		return ErrRunModeConflict
	}
	return nil
}

type Engine struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
}
type Record struct {
	ArchivedAt      *time.Time    `json:"archivedAt,omitempty"`
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

const subscriptionColumns = `id::text,title,identity,enabled,source,interval_seconds,rules,revision,last_state,last_message,next_run_at,updated_at,archived_at`

func scanRecord(row pgx.Row) (Record, error) {
	var record Record
	var identity, rules []byte
	var interval int
	err := row.Scan(&record.ID, &record.Title, &identity, &record.Enabled, &record.Source, &interval, &rules, &record.Revision, &record.LastState, &record.LastMessage, &record.NextRunAt, &record.UpdatedAt, &record.ArchivedAt)
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
func (e Engine) List(ctx context.Context) ([]Record, error) { return e.Page(ctx, 0, 200, "", "") }
func (e Engine) Page(ctx context.Context, start, limit int, query, id string) ([]Record, error) {
	if start < 0 || start > 200000 || limit < 1 || limit > 200 || len(query) > 800 {
		return nil, errors.New("invalid page")
	}
	rows, err := e.DB.Query(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions WHERE archived_at IS NULL AND ($3='' OR title ILIKE '%'||$3||'%') AND ($4='' OR id::text=$4) ORDER BY created_at DESC,id LIMIT $1 OFFSET $2`, limit, start, query, id)
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
	return e.save(ctx, id, in, false)
}
func (e Engine) Ensure(ctx context.Context, in Input) (Record, error) {
	return e.save(ctx, "", in, true)
}

var ErrAlreadyOwned = errors.New("media already exposed")

func (e Engine) save(ctx context.Context, id string, in Input, ensure bool) (Record, error) {
	created := id == ""
	if in.Source == "" {
		in.Source = "search"
	}
	prefs, err := (settings.Repository{DB: e.DB}).Get(ctx)
	if err != nil {
		return Record{}, err
	}
	in.IntervalMinutes = prefs.Value.SubscriptionIntervalMinutes
	in.Rules.MinSize = 0
	in.Rules.MaxSize = 0
	in.Rules.MinSeeders = 0
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
	if ensure && (item.ID != in.TMDBID || item.MediaType != in.MediaType) {
		return Record{}, errors.New("media identity response mismatch")
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
	defer deferutil.Rollback(ctx, tx)
	if ensure {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,66))`, fmt.Sprintf("subscription:%s:%d", item.MediaType, item.ID)); err != nil {
			return Record{}, err
		}
		var existing string
		err = tx.QueryRow(ctx, `SELECT id::text FROM subscriptions WHERE identity->>'id'=$1 AND media_type=$2 ORDER BY created_at LIMIT 1`, strconv.Itoa(item.ID), item.MediaType).Scan(&existing)
		if err == nil {
			return scanRecord(tx.QueryRow(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions WHERE id=$1`, existing))
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Record{}, err
		}
		var owned bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_assets a JOIN media_asset_bindings b ON b.asset_id=a.id JOIN media_items m ON m.id=b.media_id WHERE m.tmdb_id=$1 AND m.media_type=$2 AND a.artifacts_state='ready')`, item.ID, item.MediaType).Scan(&owned); err != nil {
			return Record{}, err
		}
		if owned {
			return Record{}, ErrAlreadyOwned
		}
	}
	var cronExpression string
	// Serialize cron assignment with system-setting updates, including new rows.
	if err = tx.QueryRow(ctx, `SELECT value->>'subscriptionCron' FROM application_settings WHERE singleton FOR SHARE`).Scan(&cronExpression); err != nil {
		return Record{}, err
	}
	if id == "" {
		id = jobs.UUID()
		_, err = tx.Exec(ctx, `INSERT INTO subscriptions(id,title,media_type,identity,rules,enabled,source,interval_seconds)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, item.Title, in.MediaType, identity, rules, in.Enabled, in.Source, in.IntervalMinutes*60)
	} else {
		old, loadErr := scanRecord(tx.QueryRow(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions WHERE id=$1 FOR UPDATE`, id))
		if loadErr != nil {
			return Record{}, loadErr
		}
		if old.ArchivedAt != nil {
			return Record{}, errors.New("历史订阅仅供查看，请新建订阅继续跟进")
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
	next, _ := scheduling.Next(cronExpression, time.Now(), 5*time.Minute)
	if _, err = tx.Exec(ctx, `UPDATE subscriptions SET next_run_at=$2 WHERE id=$1`, id, next); err != nil {
		return Record{}, err
	}
	kind, revision := "subscription.updated", in.Revision+1
	if created {
		kind, revision = "subscription.created", 1
	}
	if err = outbox.EmitVersionTx(ctx, tx, kind, id, strconv.FormatInt(revision, 10), contracts.SubscriptionChanged{SubscriptionID: id, Revision: revision, Enabled: in.Enabled, MediaType: item.MediaType, TMDBID: item.ID}); err != nil {
		return Record{}, err
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return Record{}, err
	}
	return e.Get(ctx, id)
}
func (e Engine) Queue(ctx context.Context, id string, preview bool) (string, error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer deferutil.Rollback(ctx, tx)
	record, err := scanRecord(tx.QueryRow(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return "", err
	}
	if record.ArchivedAt != nil {
		return "", errors.New("历史订阅不可执行")
	}
	var existing string
	var existingPreview bool
	err = tx.QueryRow(ctx, `SELECT id::text,preview FROM subscription_runs WHERE subscription_id=$1 AND state IN ('pending','running')`, id).Scan(&existing, &existingPreview)
	if err == nil {
		if conflictErr := runModeConflict(existingPreview, preview); conflictErr != nil {
			return "", conflictErr
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	runID := jobs.UUID()
	if _, err = tx.Exec(ctx, `INSERT INTO subscription_runs(id,subscription_id,preview,revision)VALUES($1,$2,$3,$4)`, runID, id, preview, record.Revision); err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]string{"runId": runID})
	if _, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,payload,idempotency_key,max_attempts)VALUES($1,'subscription.evaluate',$2,$3,5)`, jobs.UUID(), payload, "subscription.evaluate:"+runID); err != nil {
		return "", err
	}
	prefs, err := (settings.Repository{DB: e.DB}).Get(ctx)
	if err != nil {
		return "", err
	}
	next, err := scheduling.Next(prefs.Value.SubscriptionCron, time.Now(), 5*time.Minute)
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE subscriptions SET next_run_at=$2,last_state='pending',last_message=''WHERE id=$1`, id, next); err != nil {
		return "", err
	}
	return runID, dbutil.Commit(ctx, tx)
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
	defer dbutil.Unlock(context.Background(), conn, `SELECT pg_advisory_unlock(74812911)`)
	rows, err := e.DB.Query(ctx, `SELECT id::text FROM subscriptions WHERE enabled AND archived_at IS NULL AND next_run_at<=now() ORDER BY next_run_at LIMIT 20`)
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
	defer deferutil.Rollback(ctx, tx)
	var active int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM subscription_runs WHERE subscription_id=$1 AND state IN ('pending','running')`, id).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return errors.New("subscription evaluation is active; disable and wait before deleting")
	}
	var fact contracts.SubscriptionChanged
	fact.SubscriptionID = id
	err = tx.QueryRow(ctx, `UPDATE subscriptions SET archived_at=now(),enabled=false,revision=revision+1,updated_at=now() WHERE id=$1 AND archived_at IS NULL RETURNING revision,media_type,(identity->>'id')::int`, id).Scan(&fact.Revision, &fact.MediaType, &fact.TMDBID)
	if err != nil {
		return err
	}
	if err = outbox.EmitVersionTx(ctx, tx, "subscription.deleted", id, strconv.FormatInt(fact.Revision, 10), fact); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}
