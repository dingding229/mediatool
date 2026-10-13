package settings

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"regexp"
	"strings"
)

var ErrInvalid = errors.New("invalid application settings")
var ErrRevision = errors.New("settings changed; refresh first")

type Value struct {
	MovieTemplate               string `json:"movieTemplate"`
	SeriesTemplate              string `json:"seriesTemplate"`
	SearchCacheSeconds          int    `json:"searchCacheSeconds"`
	TMDBCacheSeconds            int    `json:"tmdbCacheSeconds"`
	SubscriptionIntervalMinutes int    `json:"subscriptionIntervalMinutes"`
	EmbyUserID                  string `json:"embyUserId"`
}
type Record struct {
	Value    Value `json:"value"`
	Revision int64 `json:"revision"`
}
type Repository struct{ DB *pgxpool.Pool }

func Defaults() Value {
	return Value{MovieTemplate: "{title} ({year}) - {resolution}{ext}", SeriesTemplate: "{title} - S{season}E{episode} - {resolution}{ext}", SearchCacheSeconds: 900, TMDBCacheSeconds: 86400, SubscriptionIntervalMinutes: 30}
}

var token = regexp.MustCompile(`\{([A-Za-z]+)\}`)

func Validate(v Value) error {
	for _, s := range []string{v.MovieTemplate, v.SeriesTemplate} {
		if len(s) == 0 || len(s) > 512 || strings.ContainsAny(s, "/\\\x00\r\n") || !strings.Contains(s, "{title}") {
			return ErrInvalid
		}
		cleaned := token.ReplaceAllStringFunc(s, func(m string) string {
			for _, x := range []string{"title", "year", "season", "episode", "resolution", "source", "codec", "group", "ext", "original"} {
				if m == "{"+x+"}" {
					return ""
				}
			}
			return m
		})
		if strings.ContainsAny(cleaned, "{}") {
			return ErrInvalid
		}
	}
	if !strings.Contains(v.SeriesTemplate, "{season}") || !strings.Contains(v.SeriesTemplate, "{episode}") {
		return ErrInvalid
	}
	if v.SubscriptionIntervalMinutes < 5 || v.SubscriptionIntervalMinutes > 1440 {
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
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE application_settings SET value=$1,revision=revision+1,updated_at=now() WHERE singleton AND revision=$2`, raw, revision)
	if err != nil {
		return Record{}, err
	}
	if tag.RowsAffected() != 1 {
		return Record{}, ErrRevision
	}
	if _, err = tx.Exec(ctx, `UPDATE subscriptions SET interval_seconds=$1,revision=revision+1,next_run_at=CASE WHEN enabled THEN now()+make_interval(mins=>$2) ELSE next_run_at END,updated_at=now() WHERE interval_seconds<>$1`, v.SubscriptionIntervalMinutes*60, v.SubscriptionIntervalMinutes); err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return r.Get(ctx)
}
