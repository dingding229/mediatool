package nativetmdb

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/adapters/tmdb"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

type Connections interface {
	TMDB(context.Context) (tmdb.Client, error)
}
type Engine struct {
	DB          *pgxpool.Pool
	Connections Connections
}

func (e Engine) Language(ctx context.Context) (string, error) {
	if e.Connections == nil {
		return "", ErrInvalid
	}
	client, err := e.Connections.TMDB(ctx)
	if err != nil {
		return "", err
	}
	if client.Language == "" {
		return "zh-CN", nil
	}
	return client.Language, nil
}
func (e Engine) Process(ctx context.Context, j jobs.Job) error {
	if e.DB == nil || e.Connections == nil {
		return ErrInvalid
	}
	var in struct {
		ID string `json:"detailId"`
	}
	if json.Unmarshal(j.Payload, &in) != nil || in.ID == "" {
		return ErrInvalid
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var target Target
	var language string
	var attempt int
	var started time.Time
	// Claim only while an exposable asset still requires this identity.
	err := e.DB.QueryRow(bounded, `WITH live AS (`+catalog.ExposableAssetsSQL+`) UPDATE native_tmdb_details d SET state='fetching',fetching_started_at=clock_timestamp(),attempts=least(attempts+1,100),updated_at=now()
 WHERE id=$1 AND next_attempt_at<=now() AND (state<>'fetching' OR fetching_started_at<now()-interval '1 minute') AND EXISTS(SELECT 1 FROM live WHERE tmdb_id=d.tmdb_id AND media_type=CASE WHEN d.tmdb_type='season' THEN 'series' ELSE d.tmdb_type END AND (d.tmdb_type<>'season' OR season=d.season_number))
 RETURNING tmdb_type,tmdb_id,season_number,language,attempts,fetching_started_at`, in.ID).Scan(&target.Type, &target.TMDBID, &target.Season, &language, &attempt, &started)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	client, fetchErr := e.Connections.TMDB(bounded)
	var payload []byte
	if fetchErr == nil {
		client.Language = language
		region := "CN"
		if parts := strings.Split(language, "-"); len(parts) == 2 {
			region = parts[1]
		}
		details, err := client.NativeDetails(bounded, nativelibrary.TMDBDetailType(target.Type), target.TMDBID, target.Season, region)
		fetchErr = err
		if err == nil {
			payload, fetchErr = json.Marshal(details)
			if len(payload) > 4<<20 {
				fetchErr = ErrInvalid
			}
		}
	}
	// The request has finished: no transaction/row lock was held over HTTP. A
	// started-at fence prevents an old timed-out attempt overwriting recovery.
	save, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	state, message, delay := nativelibrary.TMDBDetailReady, "", 30*24*time.Hour
	if fetchErr != nil {
		state, message, delay = nativelibrary.TMDBDetailFailed, "TMDB详情暂不可用，将自动重试", tmdb.ExponentialBackoff(attempt, fetchErr)
	}
	result, err := e.DB.Exec(save, `UPDATE native_tmdb_details SET state=$3,fetching_started_at=NULL,payload=CASE WHEN $3='ready' THEN $4::jsonb ELSE payload END,
 fetched_at=CASE WHEN $3='ready' THEN now() ELSE fetched_at END,next_attempt_at=now()+($5::bigint*interval '1 millisecond'),error=$6,updated_at=now()
 WHERE id=$1 AND state='fetching' AND fetching_started_at=$2`, in.ID, started, state, stringOrObject(payload), delay.Milliseconds(), message)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return jobs.ErrLeaseLost
	}
	return nil
}
func stringOrObject(raw []byte) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

func (e Engine) Refresh(ctx context.Context, target Target) (string, error) {
	if !target.Valid() {
		return "", ErrInvalid
	}
	language, err := e.Language(ctx)
	if err != nil {
		return "", err
	}
	return (Repository{DB: e.DB}).Enqueue(ctx, target, language, true)
}
