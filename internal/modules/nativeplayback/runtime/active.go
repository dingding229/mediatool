package runtime

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/nativeplayback"
	"time"
)

// Reap is bounded, cross-process safe and never changes item user data.
func Reap(ctx context.Context, db *pgxpool.Pool) (int64, error) {
	if db == nil {
		return 0, nativeplayback.ErrUpstream
	}
	tag, err := db.Exec(ctx, `WITH stale AS(SELECT id FROM native_playback_sessions WHERE state<>'stopped' AND last_progress_at<clock_timestamp()-interval '5 minutes' ORDER BY last_progress_at,id LIMIT 200 FOR UPDATE SKIP LOCKED) UPDATE native_playback_sessions s SET state='stopped',ended_at=clock_timestamp() FROM stale WHERE s.id=stale.id`)
	return tag.RowsAffected(), err
}
func MaintainSessions(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		check, cancel := context.WithTimeout(ctx, 3*time.Second)
		_, err := Reap(check, db)
		cancel()
		if err != nil && ctx.Err() == nil {
			logger.Warn("Native playback session cleanup deferred")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func ListActive(parent context.Context, db *pgxpool.Pool, start, limit int) (nativeplayback.ActiveSessions, error) {
	out := nativeplayback.ActiveSessions{Items: []nativeplayback.ActiveSession{}, StartIndex: start, Limit: limit}
	if start < 0 || start > 1000000 || limit < 1 || limit > 200 {
		return out, nativeplayback.ErrInvalid
	}
	if db == nil {
		return out, nativeplayback.ErrUpstream
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	// Count + page in one statement also returns total for an empty last page.
	rows, err := db.Query(ctx, `WITH active AS MATERIALIZED(SELECT s.id,s.user_id,s.device_id,s.item_id,s.state,s.position_ticks,s.last_progress_at FROM native_playback_sessions s WHERE s.state<>'stopped' AND s.last_progress_at>=statement_timestamp()-interval '5 minutes'), page AS(SELECT * FROM active ORDER BY last_progress_at DESC,id LIMIT $1 OFFSET $2) SELECT (SELECT count(*) FROM active),p.id::text,u.id::text,u.username,d.id::text,d.name,p.item_id::text,coalesce(m.title,''),p.state,p.position_ticks,p.last_progress_at,coalesce(i.entity_type,''),b.season,b.episode,coalesce(b.episode_title,'') FROM (SELECT 1) x LEFT JOIN page p ON true LEFT JOIN media_users u ON u.id=p.user_id LEFT JOIN media_user_devices d ON d.id=p.device_id LEFT JOIN native_item_ids i ON i.id=p.item_id `+mediausers.ItemMetadataJoins+` ORDER BY p.last_progress_at DESC,p.id`, limit, start)
	if err != nil {
		return out, nativeplayback.ErrUpstream
	}
	defer rows.Close()
	for rows.Next() {
		var v nativeplayback.ActiveSession
		var id *string
		var user, name, device, deviceName, item, itemName, state *string
		var pos *int64
		var at *time.Time
		if err = rows.Scan(&out.Total, &id, &user, &name, &device, &deviceName, &item, &itemName, &state, &pos, &at, &v.Type, &v.Season, &v.Episode, &v.EpisodeTitle); err != nil {
			return out, nativeplayback.ErrUpstream
		}
		if id != nil {
			v = nativeplayback.ActiveSession{ID: *id, UserID: *user, UserName: *name, DeviceID: *device, DeviceName: mediausers.ManagementLabel(*deviceName), ItemID: *item, ItemName: mediausers.ManagementLabel(*itemName), State: *state, PositionTicks: *pos, LastProgressAt: *at, Type: v.Type, Season: v.Season, Episode: v.Episode, EpisodeTitle: v.EpisodeTitle}
			out.Items = append(out.Items, v)
		}
	}
	if rows.Err() != nil {
		return out, nativeplayback.ErrUpstream
	}
	return out, nil
}

type SessionRepository struct{ DB *pgxpool.Pool }

func (r SessionRepository) List(ctx context.Context, start, limit int) (nativeplayback.ActiveSessions, error) {
	return ListActive(ctx, r.DB, start, limit)
}
