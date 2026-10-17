// Package nativetmdb maintains provider snapshots off the browsing hot path.
package nativetmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativelibrary/catalog"
)

type Target struct {
	Type   string `json:"type"`
	TMDBID int64  `json:"tmdbId"`
	Season *int   `json:"season,omitempty"`
}
type Repository struct{ DB *pgxpool.Pool }

var ErrInvalid = errors.New("invalid native TMDB detail request")

const targetsSQL = `WITH live AS (` + catalog.ExposableAssetsSQL + `), targets AS (
 SELECT DISTINCT media_type AS type,tmdb_id,NULL::integer AS season FROM live WHERE tmdb_id>0
 UNION SELECT DISTINCT 'season',tmdb_id,season FROM live WHERE media_type='series' AND season BETWEEN 0 AND 999)
 SELECT type,tmdb_id,season,type||':'||tmdb_id||':'||coalesce(season,-1) AS cursor FROM targets`

func (t Target) Valid() bool {
	return t.TMDBID > 0 && ((t.Type == "movie" || t.Type == "series") && t.Season == nil || t.Type == "season" && t.Season != nil && *t.Season >= 0 && *t.Season <= 999)
}
func (t Target) key(language string) string {
	season := -1
	if t.Season != nil {
		season = *t.Season
	}
	return fmt.Sprintf("native.tmdb:%s:%d:%d:%s", t.Type, t.TMDBID, season, language)
}

// ScheduleBatch has a bounded keyset cursor. No provider calls or long locks.
func (r Repository) ScheduleBatch(ctx context.Context, language, after string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := r.DB.Exec(ctx, `WITH expired AS(SELECT id FROM native_tmdb_details WHERE state='fetching' AND fetching_started_at<now()-interval '1 minute' ORDER BY fetching_started_at LIMIT 200) UPDATE native_tmdb_details d SET state='failed',fetching_started_at=NULL,next_attempt_at=now(),error='上次详情获取中断，已安排重试',updated_at=now() FROM expired e WHERE d.id=e.id`); err != nil {
		return after, 0, err
	}
	rows, err := r.DB.Query(ctx, targetsSQL+` WHERE (type||':'||tmdb_id||':'||coalesce(season,-1))>$1 ORDER BY cursor LIMIT 200`, after)
	if err != nil {
		return after, 0, err
	}
	type entry struct {
		target Target
		cursor string
	}
	batch := []entry{}
	for rows.Next() {
		var x entry
		if err := rows.Scan(&x.target.Type, &x.target.TMDBID, &x.target.Season, &x.cursor); err != nil {
			rows.Close()
			return after, 0, err
		}
		batch = append(batch, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return after, 0, err
	}
	for _, x := range batch {
		if _, err := r.enqueue(ctx, x.target, language, false, false); err != nil {
			return after, 0, err
		}
		after = x.cursor
	}
	return after, len(batch), nil
}
func (r Repository) EnqueueAsset(ctx context.Context, assetID, language string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	query := `WITH live AS (` + catalog.ExposableAssetsSQL + `) SELECT media_type,tmdb_id,season FROM live WHERE asset_id=$1`
	var t Target
	var season *int
	err := r.DB.QueryRow(ctx, query, assetID).Scan(&t.Type, &t.TMDBID, &season)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = r.enqueue(ctx, t, language, false, true); err != nil {
		return err
	}
	if t.Type == "series" && season != nil {
		t.Type, t.Season = "season", season
		_, err = r.enqueue(ctx, t, language, false, true)
	}
	return err
}

// Enqueue validates current exposable evidence even for manual refresh. A single
// business key merges pending work; running jobs record rerun for jobs.Finish.
func (r Repository) Enqueue(ctx context.Context, t Target, language string, force bool) (string, error) {
	return r.enqueue(ctx, t, language, force, force)
}
func (r Repository) enqueue(ctx context.Context, t Target, language string, force, priority bool) (id string, err error) {
	if r.DB == nil || !t.Valid() || language == "" || len(language) > 16 {
		return "", ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if e := tx.Rollback(cleanup); e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = errors.Join(err, e)
		}
	}()
	var eligible bool
	err = tx.QueryRow(ctx, `WITH live AS (`+catalog.ExposableAssetsSQL+`) SELECT EXISTS(SELECT 1 FROM live WHERE tmdb_id=$2 AND media_type=CASE WHEN $1='season' THEN 'series' ELSE $1 END AND ($1<>'season' OR season=$3))`, t.Type, t.TMDBID, t.Season).Scan(&eligible)
	if err != nil {
		return "", err
	}
	if !eligible {
		return "", ErrInvalid
	}
	var due bool
	err = tx.QueryRow(ctx, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,season_number,language)VALUES($1,$2,$3,$4)
 ON CONFLICT(identity_key) DO UPDATE SET updated_at=native_tmdb_details.updated_at
 RETURNING id::text,($5 OR (state<>'fetching' AND next_attempt_at<=now() AND (state<>'ready' OR fetched_at<now()-interval '30 days')))`, t.Type, t.TMDBID, t.Season, language, force).Scan(&id, &due)
	if err != nil {
		return "", err
	}
	if !due {
		return id, tx.Commit(ctx)
	}
	payload, e := json.Marshal(map[string]string{"detailId": id})
	if e != nil {
		return "", e
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs(id,kind,queue,payload,idempotency_key,max_attempts,available_at)VALUES($1,'native.tmdb.fetch','metadata',$2,$3,5,CASE WHEN $4 THEN now()-interval '1 year' ELSE now() END)
 ON CONFLICT(idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET
 payload=CASE WHEN jobs.state='running' THEN jsonb_set(jobs.payload,'{rerun}','true'::jsonb) ELSE excluded.payload END,
 state=CASE WHEN jobs.state IN('succeeded','failed','retry') THEN 'pending' ELSE jobs.state END,
 attempts=CASE WHEN jobs.state IN('succeeded','failed','retry') THEN 0 ELSE jobs.attempts END,
 available_at=CASE WHEN jobs.state IN('succeeded','failed','retry') THEN excluded.available_at ELSE least(jobs.available_at,excluded.available_at) END,
 last_error=NULL`, jobs.UUID(), payload, t.key(language), priority)
	if err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}
