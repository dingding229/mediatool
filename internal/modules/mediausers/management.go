package mediausers

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/platform/clientip"
	dbutil "media-tool/internal/platform/database"
	"time"

	"github.com/jackc/pgx/v5"
)

// Management DTOs exclude device keys, UA, tokens and source paths. Only
// administrator routes return sanitized public IP evidence.
type Device struct {
	LastIP       string    `json:"lastIp"`
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Client       string    `json:"client"`
	AppVersion   string    `json:"appVersion"`
	LastSeenAt   time.Time `json:"lastSeenAt"`
	CreatedAt    time.Time `json:"createdAt"`
	ActiveTokens int       `json:"activeTokens"`
}
type HistoryItem struct {
	Season          *int      `json:"season"`
	Episode         *int      `json:"episode"`
	EpisodeTitle    string    `json:"episodeTitle"`
	DurationSeconds float64   `json:"durationSeconds"`
	ItemID          string    `json:"itemId"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	PositionTicks   int64     `json:"positionTicks"`
	PlayCount       int       `json:"playCount"`
	Played          bool      `json:"played"`
	LastPlayedAt    time.Time `json:"lastPlayedAt"`
}
type ManagementPage[T any] struct {
	Items      []T `json:"items"`
	Total      int `json:"total"`
	StartIndex int `json:"startIndex"`
	Limit      int `json:"limit"`
}

func (r Repository) managementUser(ctx context.Context, id string, start, limit int) error {
	if !validID(id) || start < 0 || start > 1000000 || limit < 1 || limit > 200 {
		return ErrInvalid
	}
	if r.DB == nil {
		return ErrUnavailable
	}
	var found bool
	if err := r.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_users WHERE id=$1)`, id).Scan(&found); err != nil {
		return ErrUnavailable
	}
	if !found {
		return ErrNotFound
	}
	return nil
}
func (r Repository) Devices(parent context.Context, id string, start, limit int) (ManagementPage[Device], error) {
	out := ManagementPage[Device]{Items: []Device{}, StartIndex: start, Limit: limit}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	if err := r.managementUser(ctx, id, start, limit); err != nil {
		return out, err
	}
	// Count is independent of OFFSET, including empty last pages. Only page devices
	// visit the token index; no digest is read into the application.
	rows, err := r.DB.Query(ctx, `WITH page AS (SELECT id,name,client,app_version,last_seen_at,created_at,last_ip FROM media_user_devices WHERE user_id=$1 ORDER BY last_seen_at DESC,id LIMIT $2 OFFSET $3)
 SELECT (SELECT count(*) FROM media_user_devices WHERE user_id=$1),p.id::text,p.name,p.client,p.app_version,p.last_seen_at,p.created_at,coalesce(host(p.last_ip),''),
 (SELECT count(*) FROM media_user_tokens t JOIN media_users u ON u.id=t.user_id WHERE t.user_id=$1 AND t.device_id=p.id AND t.revoked_at IS NULL AND t.expires_at>statement_timestamp() AND t.credential_version=u.credential_version AND u.enabled AND NOT u.must_change_password AND (u.expires_at IS NULL OR u.expires_at>statement_timestamp()))
 FROM (SELECT 1) x LEFT JOIN page p ON true ORDER BY p.last_seen_at DESC,p.id`, id, limit, start)
	if err != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var v Device
		var key *string
		var name, client, version *string
		var seen, created *time.Time
		if err = rows.Scan(&out.Total, &key, &name, &client, &version, &seen, &created, &v.LastIP, &v.ActiveTokens); err != nil {
			return out, ErrUnavailable
		}
		if key != nil {
			v.ID = *key
			v.Name = ManagementLabel(*name)
			v.Client = ManagementLabel(*client)
			v.AppVersion = ManagementLabel(*version)
			v.LastSeenAt = *seen
			v.CreatedAt = *created
			v.LastIP = clientip.Sanitize(v.LastIP)
			out.Items = append(out.Items, v)
		}
	}
	if rows.Err() != nil {
		return out, ErrUnavailable
	}
	return out, nil
}

// RevokeDevice retains the device/history, revokes every credential for that
// device and releases its playback slots in one short transaction. No user-wide
// credential_version bump: other devices remain logged in. Repeats are safe.
func (r Repository) RevokeDevice(parent context.Context, userID, deviceID string) error {
	return r.revokeDevice(parent, userID, deviceID, nil)
}
func (r Repository) RevokeDeviceExpected(parent context.Context, userID, deviceID string, revision int64) error {
	return r.revokeDevice(parent, userID, deviceID, &revision)
}
func (r Repository) revokeDevice(parent context.Context, userID, deviceID string, expected *int64) (err error) {
	if !validID(userID) || !validID(deviceID) {
		return ErrInvalid
	}
	if r.DB == nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer done()
		if e := tx.Rollback(cleanup); e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = ErrUnavailable
		}
	}()
	// Same lock order as Login: user then device, so revoke cannot miss a concurrent login.
	var key string
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT credential_version FROM media_users WHERE id=$1 FOR UPDATE`, userID).Scan(&revision); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return ErrUnavailable
	}
	if expected != nil && (*expected < 1 || revision != *expected) {
		return ErrConflict
	}
	if err = tx.QueryRow(ctx, `SELECT id::text FROM media_user_devices WHERE id=$1 AND user_id=$2 FOR UPDATE`, deviceID, userID).Scan(&key); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `UPDATE media_user_tokens SET revoked_at=clock_timestamp() WHERE user_id=$1 AND device_id=$2 AND revoked_at IS NULL`, userID, deviceID); err != nil {
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `UPDATE native_playback_sessions SET state='stopped',ended_at=clock_timestamp() WHERE user_id=$1 AND device_id=$2 AND state<>'stopped'`, userID, deviceID); err != nil {
		return ErrUnavailable
	}
	raw, e := json.Marshal(map[string]string{"userId": userID, "deviceId": deviceID})
	if e != nil {
		return ErrInvalid
	}
	tag, e := tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','media_user.device_revoke',$1)`, raw)
	if e != nil || tag.RowsAffected() != 1 {
		return ErrUnavailable
	}
	return dbError(dbutil.Commit(ctx, tx))
}

// History is the latest watched state per item (not a raw event log). Existing
// (user_id,last_played_at DESC,item_id) partial index bounds page lookup.
func (r Repository) History(parent context.Context, id string, start, limit int) (ManagementPage[HistoryItem], error) {
	out := ManagementPage[HistoryItem]{Items: []HistoryItem{}, StartIndex: start, Limit: limit}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	if err := r.managementUser(ctx, id, start, limit); err != nil {
		return out, err
	}
	rows, err := r.DB.Query(ctx, `WITH page AS MATERIALIZED(SELECT item_id,position_ticks,play_count,played,last_played_at FROM media_user_item_data WHERE user_id=$1 AND last_played_at IS NOT NULL ORDER BY last_played_at DESC,item_id LIMIT $2 OFFSET $3)
 SELECT (SELECT count(*) FROM media_user_item_data WHERE user_id=$1 AND last_played_at IS NOT NULL),p.item_id::text,
 coalesce(m.title,'已移除条目'),i.entity_type,p.position_ticks,(SELECT count(*) FROM media_playback_events WHERE user_id=$1 AND item_id=p.item_id)::int,p.played,p.last_played_at,
 b.season,b.episode,coalesce(b.episode_title,''),(SELECT coalesce(sum(duration_seconds),0) FROM media_playback_events WHERE user_id=$1 AND item_id=p.item_id)
 FROM (SELECT 1)x LEFT JOIN page p ON true LEFT JOIN native_item_ids i ON i.id=p.item_id
`+ItemMetadataJoins+` ORDER BY p.last_played_at DESC,p.item_id`, id, limit, start)
	if err != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var v HistoryItem
		var key, name, kind *string
		var pos *int64
		var count *int
		var played *bool
		var at *time.Time
		if err = rows.Scan(&out.Total, &key, &name, &kind, &pos, &count, &played, &at, &v.Season, &v.Episode, &v.EpisodeTitle, &v.DurationSeconds); err != nil {
			return out, ErrUnavailable
		}
		if key != nil {
			v.ItemID = *key
			v.Name = ManagementLabel(*name)
			v.Type = *kind
			v.PositionTicks = *pos
			v.PlayCount = *count
			v.Played = *played
			v.LastPlayedAt = *at
			out.Items = append(out.Items, v)
		}
	}
	if rows.Err() != nil {
		return out, ErrUnavailable
	}
	return out, nil
}
