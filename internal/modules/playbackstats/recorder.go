// Package playbackstats stores administrator-only per-session playback facts.
package playbackstats

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/clientip"
	"strings"
	"time"
)

// Snapshot contains no credentials, resolver URL or CDN URL. ID is the existing
// server-owned playback session ID, never a client-supplied user/device ID.
type Snapshot struct {
	DeviceOnly                                             bool
	ID, UserID, DeviceID, ItemID, SeriesID, SeasonID       string
	Type, Name, EpisodeTitle, Client, UserAgent, IP, State string
	Season, Episode                                        *int
	At                                                     time.Time
	Position                                               int64
	Redirect                                               bool
}
type Sink interface{ Submit(Snapshot) }
type Recorder struct {
	queue chan Snapshot
	db    *pgxpool.Pool
	log   *slog.Logger
}

func New(db *pgxpool.Pool, log *slog.Logger) *Recorder {
	if log == nil {
		log = slog.Default()
	}
	return &Recorder{make(chan Snapshot, 1024), db, log}
}

// Bounded, nonblocking admission: analytics cannot delay or fail a 302.
func (r *Recorder) Submit(v Snapshot) {
	if r == nil {
		return
	}
	v.IP = clientip.Sanitize(v.IP)
	v.UserAgent = truncate(v.UserAgent, 2048)
	v.Client = truncate(v.Client, 128)
	select {
	case r.queue <- v:
	default:
		r.log.Warn("播放统计队列已满，本次统计未写入", "module", "playbackstats")
	}
}

// Device observations count authenticated requests, separately from play counts.
func (r *Recorder) ObserveDevice(user, device, ip string) {
	if clientip.Sanitize(ip) != "" {
		r.Submit(Snapshot{UserID: user, DeviceID: device, IP: ip, At: time.Now(), DeviceOnly: true})
	}
}
func truncate(v string, n int) string {
	if len(v) > n {
		return strings.ToValidUTF8(v[:n], "")
	}
	return v
}
func (r *Recorder) Run(ctx context.Context, retention func(context.Context) (int, error)) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	r.log.Info("playback statistics retention registered", "module", "playbackstats", "batch", 1000, "defaultDays", 180)
	for {
		select {
		case <-ctx.Done():
			return
		case v := <-r.queue:
			c, cancel := context.WithTimeout(ctx, 3*time.Second)
			e := r.Write(c, v)
			cancel()
			if e != nil {
				r.log.Warn("播放统计写入失败，不影响播放", "module", "playbackstats")
			}
		case now := <-tick.C:
			c, cancel := context.WithTimeout(ctx, 5*time.Second)
			days, e := retention(c)
			if e == nil {
				e = r.Maintain(c, now, days)
			}
			cancel()
			if e != nil {
				r.log.Warn("播放统计保留清理暂缓", "module", "playbackstats")
			}
		}
	}
}
func (r *Recorder) Write(ctx context.Context, v Snapshot) error {
	if v.At.IsZero() {
		v.At = time.Now()
	}
	v.IP = clientip.Sanitize(v.IP)
	tx, e := r.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer deferutil.Rollback(ctx, tx)
	if v.DeviceOnly {
		if v.IP != "" {
			_, e = tx.Exec(ctx, `INSERT INTO media_device_ips(user_id,device_id,ip,first_seen_at,last_seen_at)VALUES($1::uuid,$2::uuid,$3::inet,$4,$4) ON CONFLICT(device_id,ip) DO UPDATE SET last_seen_at=greatest(media_device_ips.last_seen_at,EXCLUDED.last_seen_at),request_count=media_device_ips.request_count+1 WHERE media_device_ips.user_id=EXCLUDED.user_id`, v.UserID, v.DeviceID, v.IP, v.At)
			if e != nil {
				return e
			}
		}
		return tx.Commit(ctx)
	}

	// In-order serial writer + monotonic guards handle duplicates/late reports.
	// Pauses/seeks do not inflate duration; unobserved gaps are capped at 5 min.
	_, e = tx.Exec(ctx, `INSERT INTO media_playback_events(id,user_id,device_id,item_id,series_id,season_id,media_type,name,episode_title,season,episode,client,user_agent,started_at,last_seen_at,state,ended_at,position_ticks,source_ip,redirect_count)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4::bigint,$5,$6,$7,$8,$9,$10,$11,coalesce(nullif($12,''),(SELECT client FROM media_user_devices WHERE id=$3::uuid)),$13,$14::timestamptz,$14::timestamptz,$15::text,CASE WHEN $15::text='stopped' THEN $14::timestamptz END,$16,nullif($17,'')::inet,$18)
 ON CONFLICT(id) DO UPDATE SET
 duration_seconds=media_playback_events.duration_seconds+CASE WHEN media_playback_events.state='playing' AND NOT $19 AND EXCLUDED.last_seen_at>media_playback_events.last_seen_at THEN least(300,extract(epoch FROM EXCLUDED.last_seen_at-media_playback_events.last_seen_at),greatest(0,(EXCLUDED.position_ticks-media_playback_events.position_ticks)/10000000.0)) ELSE 0 END,
 last_seen_at=greatest(media_playback_events.last_seen_at,EXCLUDED.last_seen_at),
 state=CASE WHEN media_playback_events.state='stopped' OR EXCLUDED.last_seen_at<media_playback_events.last_seen_at OR $19 THEN media_playback_events.state ELSE EXCLUDED.state END,
 ended_at=CASE WHEN media_playback_events.state='stopped' THEN media_playback_events.ended_at WHEN EXCLUDED.last_seen_at<media_playback_events.last_seen_at OR $19 THEN media_playback_events.ended_at WHEN EXCLUDED.state='stopped' THEN EXCLUDED.last_seen_at END,
 position_ticks=CASE WHEN EXCLUDED.last_seen_at>=media_playback_events.last_seen_at AND NOT $19 THEN EXCLUDED.position_ticks ELSE media_playback_events.position_ticks END,
 source_ip=coalesce(EXCLUDED.source_ip,media_playback_events.source_ip),redirect_count=media_playback_events.redirect_count+EXCLUDED.redirect_count
 WHERE media_playback_events.user_id=EXCLUDED.user_id AND media_playback_events.device_id=EXCLUDED.device_id`,
		v.ID, v.UserID, v.DeviceID, v.ItemID, v.SeriesID, v.SeasonID, v.Type, v.Name, v.EpisodeTitle, v.Season, v.Episode, v.Client, v.UserAgent, v.At, v.State, v.Position, v.IP, boolNumber(v.Redirect), v.Redirect)
	if e != nil {
		return e
	}
	if v.IP != "" {
		_, e = tx.Exec(ctx, `INSERT INTO media_playback_ips(event_id,ip,first_seen_at,last_seen_at)VALUES($1::uuid,$2::inet,$3,$3) ON CONFLICT(event_id,ip) DO UPDATE SET first_seen_at=least(media_playback_ips.first_seen_at,EXCLUDED.first_seen_at),last_seen_at=greatest(media_playback_ips.last_seen_at,EXCLUDED.last_seen_at)`, v.ID, v.IP, v.At)
		if e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func boolNumber(v bool) int {
	if v {
		return 1
	}
	return 0
}
func (r *Recorder) Maintain(ctx context.Context, now time.Time, days int) error {
	if days < 1 || days > 730 {
		return nil
	}
	_, e := r.db.Exec(ctx, `UPDATE media_playback_events SET state='stopped',ended_at=last_seen_at WHERE id IN(SELECT id FROM media_playback_events WHERE state<>'stopped' AND last_seen_at<$1 LIMIT 1000)`, now.Add(-5*time.Minute))
	if e != nil {
		return e
	}
	_, e = r.db.Exec(ctx, `DELETE FROM media_device_ips WHERE (device_id,ip) IN(SELECT device_id,ip FROM media_device_ips WHERE last_seen_at<$1 ORDER BY last_seen_at LIMIT 1000)`, now.AddDate(0, 0, -days))
	if e != nil {
		return e
	}
	_, e = r.db.Exec(ctx, `DELETE FROM media_playback_events WHERE id IN(SELECT id FROM media_playback_events WHERE started_at<$1 ORDER BY started_at LIMIT 1000)`, now.AddDate(0, 0, -days))
	return e
}
