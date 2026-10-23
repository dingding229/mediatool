package settings

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/playeraccess"
	"media-tool/internal/modules/releasename"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/logging"
	"media-tool/internal/platform/scheduling"
	"regexp"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid application settings")
var ErrRevision = errors.New("settings changed; refresh first")

type Value struct {
	PlaybackRetentionDays       int                   `json:"playbackRetentionDays"`
	Logging                     logging.Policy        `json:"logging"`
	Access                      Access                `json:"access"`
	CleanupAfterImport          bool                  `json:"cleanupAfterImport"`
	TrustedProxyCIDRs           []string              `json:"trustedProxyCidrs"`
	Scraping                    metadata.ScrapePolicy `json:"scraping"`
	MovieTemplate               string                `json:"movieTemplate"`
	SeriesTemplate              string                `json:"seriesTemplate"`
	SearchCacheSeconds          int                   `json:"searchCacheSeconds"`
	TMDBCacheSeconds            int                   `json:"tmdbCacheSeconds"`
	SubscriptionIntervalMinutes int                   `json:"-"`
	SubscriptionCron            string                `json:"subscriptionCron"`
	EmbyUserID                  string                `json:"embyUserId"`
}
type Record struct {
	Value    Value `json:"value"`
	Revision int64 `json:"revision"`
}
type Repository struct{ DB *pgxpool.Pool }

func Defaults() Value {
	return Value{PlaybackRetentionDays: 180, Logging: logging.DefaultPolicy(), Scraping: metadata.DefaultScrapePolicy(), MovieTemplate: "{title} ({year})/{title} ({year}) - {videoFormat}", SeriesTemplate: "{title} ({year})/Season {season}/{title} - {season_episode} - 第{episode}集", SearchCacheSeconds: 900, TMDBCacheSeconds: 86400, SubscriptionIntervalMinutes: 30, SubscriptionCron: "*/30 * * * *"}
}

func Validate(v Value) error {
	if v.PlaybackRetentionDays < 1 || v.PlaybackRetentionDays > 730 {
		return ErrInvalid
	}
	if err := v.Logging.Validate(); err != nil {
		return err
	}
	if err := ValidateAccess(v.Access); err != nil {
		return err
	}
	if !playeraccess.ValidateProxies(v.TrustedProxyCIDRs) {
		return errors.New("可信代理必须为最多32个规范CIDR，不能使用0.0.0.0/0或::/0")
	}
	s := v.Scraping
	if s.Movie.Thumb || s.Series.Thumb || s.Season.Backdrop || s.Season.Logo || s.Season.Thumb || s.Episode.Poster || s.Episode.Backdrop || s.Episode.Logo {
		return ErrInvalid
	}
	for _, s := range []string{v.MovieTemplate, v.SeriesTemplate} {
		if releasename.ValidateTemplate(s) != nil || !strings.Contains(s, "{title}") {
			return ErrInvalid
		}
	}
	if strings.Contains(v.SeriesTemplate, "/") {
		parts := strings.Split(v.SeriesTemplate, "/")
		if len(parts) != 3 || !strings.Contains(parts[1], "{season}") {
			return ErrInvalid
		}
	}
	if !strings.Contains(v.SeriesTemplate, "{season_episode}") && (!strings.Contains(v.SeriesTemplate, "{season}") || !strings.Contains(v.SeriesTemplate, "{episode}")) {
		return ErrInvalid
	}
	if _, err := scheduling.Next(v.SubscriptionCron, time.Now(), 5*time.Minute); err != nil {
		return ErrInvalid
	}
	if v.SearchCacheSeconds < 0 || v.SearchCacheSeconds > 86400 || v.TMDBCacheSeconds < 0 || v.TMDBCacheSeconds > 2592000 || len(v.EmbyUserID) > 100 || v.EmbyUserID != "" && !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(v.EmbyUserID) {
		return ErrInvalid
	}
	return nil
}
func (r Repository) Get(ctx context.Context) (Record, error) {
	out := Record{Value: Defaults(), Revision: 1}
	if r.DB == nil {
		return out, nil
	}
	var raw []byte
	if err := r.DB.QueryRow(ctx, `SELECT value,revision FROM application_settings WHERE singleton`).Scan(&raw, &out.Revision); err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out.Value); err != nil {
		return out, err
	}
	return out, Validate(out.Value)
}
func (r Repository) Save(ctx context.Context, v Value, revision int64) (Record, error) {
	if Validate(v) != nil {
		return Record{}, ErrInvalid
	}
	raw, _ := json.Marshal(v)
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer deferutil.Rollback(ctx, tx)
	var previous []byte
	if err = tx.QueryRow(ctx, `SELECT value FROM application_settings WHERE singleton FOR UPDATE`).Scan(&previous); err != nil {
		return Record{}, err
	}
	var prior Value
	if err = json.Unmarshal(previous, &prior); err != nil {
		return Record{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE application_settings SET value=$1,revision=revision+1,updated_at=now() WHERE singleton AND revision=$2`, raw, revision)
	if err != nil {
		return Record{}, err
	}
	if tag.RowsAffected() != 1 {
		return Record{}, ErrRevision
	}
	if prior.SubscriptionCron != v.SubscriptionCron {
		next, _ := scheduling.Next(v.SubscriptionCron, time.Now(), 5*time.Minute)
		if _, err = tx.Exec(ctx, `UPDATE subscriptions SET revision=revision+1,next_run_at=CASE WHEN enabled THEN $1 ELSE next_run_at END,updated_at=now()`, next); err != nil {
			return Record{}, err
		}

	}
	if err = emitSettings(ctx, tx, "application", revision+1, changedSettings(prior, v)); err != nil {
		return Record{}, err
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return Record{}, err
	}
	return r.Get(ctx)
}
