package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"media-tool/internal/platform/buildinfo"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"sync"
	"time"
)

const settingsKey = "system.update-check"
const latestURL = "https://api.github.com/repos/" + Repository + "/releases/latest"
const hubPrefix = "https://hub.docker.com/v2/namespaces/dingding229/repositories/mediatool/tags/"
const Interval = 12 * time.Hour

var ErrBusy = errors.New("更新检查正在进行，请稍后读取结果")
var ErrUnavailable = errors.New("更新检查暂不可用")

type Result struct {
	CheckedAt time.Time `json:"checkedAt"`
	Status    string    `json:"status"`
	Latest    *Release  `json:"latest"`
}
type State struct {
	Enabled       bool           `json:"enabled"`
	Result        Result         `json:"result"`
	NoticeVersion string         `json:"noticeVersion"`
	Current       buildinfo.Info `json:"current"`
	HasUpdate     bool           `json:"hasUpdate"`
	IntervalHours int            `json:"intervalHours"`
}
type Service struct {
	DB      *pgxpool.Pool
	Client  *http.Client
	Logger  *slog.Logger
	Current func() buildinfo.Info
	flight  sync.Mutex
}

func New(db *pgxpool.Pool, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{DB: db, Logger: logger, Current: buildinfo.Current, Client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("release redirects are not allowed") }}}
}
func (s *Service) Get(ctx context.Context) (State, error) {
	out := State{Enabled: true, Current: s.Current(), IntervalHours: 12}
	if s.DB == nil {
		return out, ErrUnavailable
	}
	var raw []byte
	e := s.DB.QueryRow(ctx, `SELECT value FROM app_settings WHERE key=$1`, settingsKey).Scan(&raw)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	if len(raw) > 0 && json.Unmarshal(raw, &out) != nil {
		return out, ErrUnavailable
	}
	// Persist only settings/results, never stale executable provenance or availability.
	out.Current = s.Current()
	out.IntervalHours = 12
	out.HasUpdate = false
	if out.Result.Status == "verified" && out.Result.Latest != nil {
		if n, ok := buildinfo.Compare(out.Current.Version, out.Result.Latest.Version); ok && n < 0 {
			out.HasUpdate = true
		}
	}
	return out, nil
}
func (s *Service) SetEnabled(ctx context.Context, enabled bool) (State, error) {
	if s.DB == nil {
		return State{}, ErrUnavailable
	}
	raw, _ := json.Marshal(map[string]bool{"enabled": enabled})
	_, e := s.DB.Exec(ctx, `INSERT INTO app_settings(key,value)VALUES($1,$2) ON CONFLICT(key)DO UPDATE SET value=jsonb_set(app_settings.value,'{enabled}',$2::jsonb->'enabled'),updated_at=now()`, settingsKey, raw)
	if e != nil {
		return State{}, e
	}
	return s.Get(ctx)
}
func (s *Service) store(ctx context.Context, r Result) error {
	raw, e := json.Marshal(r)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(ctx, `INSERT INTO app_settings(key,value)VALUES($1,jsonb_build_object('enabled',true,'result',$2::jsonb)) ON CONFLICT(key)DO UPDATE SET value=jsonb_set(app_settings.value,'{result}',$2::jsonb),updated_at=now()`, settingsKey, raw)
	return e
}
func (s *Service) Check(parent context.Context) (State, error) {
	if s.DB == nil {
		return State{}, ErrUnavailable
	}
	if !s.flight.TryLock() {
		return State{}, ErrBusy
	}
	defer s.flight.Unlock()
	ctx, cancel := context.WithTimeout(parent, 24*time.Second)
	defer cancel()
	conn, e := s.DB.Acquire(ctx)
	if e != nil {
		return State{}, e
	}
	defer conn.Release()
	var locked bool
	if e = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(193293,8)`).Scan(&locked); e != nil {
		return State{}, e
	}
	if !locked {
		return State{}, ErrBusy
	}
	defer func() {
		clean, done := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer done()
		var unlocked bool
		if e := conn.QueryRow(clean, `SELECT pg_advisory_unlock(193293,8)`).Scan(&unlocked); e != nil || !unlocked {
			deferutil.CloseContext(clean, conn.Hijack(), "Discard connection so no session advisory lock leaks back into the pool")
			s.Logger.Warn("更新检查锁释放失败", "module", "updatecheck")
		}
	}()
	old, e := s.Get(ctx)
	if e != nil {
		return State{}, e
	}
	if !old.Result.CheckedAt.IsZero() && time.Since(old.Result.CheckedAt) < time.Minute {
		return old, nil
	}
	result := Result{CheckedAt: time.Now().UTC(), Status: "unavailable"}
	v, e := s.fetch(ctx)
	switch {
	case errors.Is(e, errNoRelease):
		result.Status = "no_release"
	case errors.Is(e, ErrUntrusted):
		result.Status = "untrusted"
	case e == nil:
		result.Status = "verified"
		result.Latest = &v
	}
	if e = s.store(ctx, result); e != nil {
		return State{}, e
	}
	return s.Get(ctx)
}

// TakeNotice is a cross-tab/restart-safe one-time claim for each stable version.
func (s *Service) TakeNotice(ctx context.Context) (*Release, error) {
	v, e := s.Get(ctx)
	if e != nil {
		return nil, e
	}
	if !v.HasUpdate || v.Result.Latest == nil {
		return nil, nil
	}
	tag := v.Result.Latest.Version
	result, e := s.DB.Exec(ctx, `UPDATE app_settings SET value=jsonb_set(value,'{noticeVersion}',to_jsonb($2::text)),updated_at=now() WHERE key=$1 AND coalesce(value->>'noticeVersion','')<>$2 AND value#>>'{result,status}'='verified' AND value#>>'{result,latest,version}'=$2`, settingsKey, tag)
	if e != nil {
		return nil, e
	}
	if result.RowsAffected() == 0 {
		return nil, nil
	}
	return v.Result.Latest, nil
}
func (s *Service) Run(ctx context.Context) {
	if s.DB == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	s.Logger.Info("release update checker registered", "module", "updatecheck", "intervalHours", 12)
	for {
		v, e := s.Get(ctx)
		if e == nil && shouldCheck(v, time.Now()) {
			if _, e = s.Check(ctx); e != nil && ctx.Err() == nil {
				s.Logger.Warn("更新检查暂缓", "module", "updatecheck")
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

var errNoRelease = errors.New("no stable release")

func (s *Service) getJSON(ctx context.Context, address string, target any) error {
	req, e := http.NewRequestWithContext(ctx, "GET", address, nil)
	if e != nil {
		return ErrUnavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MediaTool-UpdateCheck/1")
	response, e := s.Client.Do(req)
	if e != nil {
		return ErrUnavailable
	}
	defer deferutil.Close(response.Body, "Read-only release response already checked")
	if address == latestURL && response.StatusCode == 404 {
		return errNoRelease
	}
	if response.StatusCode != 200 {
		return ErrUnavailable
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, 2<<20+1))
	if e != nil || len(data) > 2<<20 {
		return ErrUnavailable
	}
	if json.Unmarshal(data, target) != nil {
		return ErrUntrusted
	}
	return nil
}
func (s *Service) fetch(ctx context.Context) (Release, error) {
	var github struct {
		Tag         string    `json:"tag_name"`
		Body        string    `json:"body"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
		PublishedAt time.Time `json:"published_at"`
	}
	if e := s.getJSON(ctx, latestURL, &github); e != nil {
		return Release{}, e
	}
	if github.Draft || github.Prerelease {
		return Release{}, ErrUntrusted
	}
	release, e := parseRelease(github.Tag, github.Body, github.PublishedAt)
	if e != nil {
		return release, e
	}
	var hub struct {
		Name   string `json:"name"`
		Digest string `json:"digest"`
		Images []struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
			Digest       string `json:"digest"`
		} `json:"images"`
	}
	if e = s.getJSON(ctx, hubPrefix+release.Tag, &hub); e != nil {
		return Release{}, e
	}
	if hub.Name != release.Tag || hub.Digest != release.Digest {
		return Release{}, ErrUntrusted
	}
	amd64 := 0
	for _, image := range hub.Images {
		if image.Architecture == "unknown" {
			continue
		}
		if image.Architecture != "amd64" || image.OS != "linux" || !digestPattern.MatchString(image.Digest) {
			return Release{}, ErrUntrusted
		}
		amd64++
	}
	if amd64 != 1 {
		return Release{}, ErrUntrusted
	}
	return release, nil
}

func shouldCheck(v State, now time.Time) bool {
	return v.Enabled && (v.Result.CheckedAt.IsZero() || now.Sub(v.Result.CheckedAt) >= Interval)
}
