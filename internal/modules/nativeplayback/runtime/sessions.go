package runtime

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativeplayback"
	"time"
)

type reservation struct {
	ID  string
	At  time.Time
	New bool
}

// The user row serializes only short DB admission transactions. No network or
// filesystem operation holds it. stopped/ended=start is an unactivated draft.
func (s *Service) session(parent context.Context, a nativeplayback.Actor, item, source string, activate bool) (out reservation, err error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, nativeplayback.ErrUpstream
	}
	defer func() {
		if e := tx.Rollback(ctx); err == nil && e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = nativeplayback.ErrUpstream
		}
	}()
	var allowed bool
	var limit int
	err = tx.QueryRow(ctx, `SELECT enabled AND NOT must_change_password AND (expires_at IS NULL OR expires_at>clock_timestamp()) AND credential_version=$2,max_streams FROM media_users WHERE id=$1::uuid FOR UPDATE`, a.User.ID, a.User.CredentialVersion).Scan(&allowed, &limit)
	if err != nil {
		return out, nativeplayback.ErrUpstream
	}
	if !allowed {
		return out, nativeplayback.ErrForbidden
	}
	// Recheck the device/user relation, never trust a body DeviceId.
	var owned bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_user_devices WHERE id=$1::uuid AND user_id=$2::uuid)`, a.Device.ID, a.User.ID).Scan(&owned); err != nil {
		return out, nativeplayback.ErrUpstream
	}
	if !owned {
		return out, nativeplayback.ErrForbidden
	}
	if _, err = tx.Exec(ctx, `UPDATE native_playback_sessions SET state='stopped',ended_at=clock_timestamp() WHERE user_id=$1::uuid AND state<>'stopped' AND last_progress_at<clock_timestamp()-interval '5 minutes'`, a.User.ID); err != nil {
		return out, nativeplayback.ErrUpstream
	}
	{
		err = tx.QueryRow(ctx, `SELECT id::text FROM native_playback_sessions WHERE user_id=$1::uuid AND device_id=$2::uuid AND media_source_id=$3 AND state<>'stopped' ORDER BY last_progress_at DESC,id LIMIT 1`, a.User.ID, a.Device.ID, source).Scan(&out.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, nativeplayback.ErrUpstream
		}
		if out.ID != "" && !activate {
			if err = tx.Commit(ctx); err != nil {
				return out, nativeplayback.ErrUpstream
			}
			return out, nil
		}
		if activate && out.ID == "" {
			var count int
			if err = tx.QueryRow(ctx, `SELECT count(DISTINCT(device_id,media_source_id)) FROM native_playback_sessions WHERE user_id=$1::uuid AND state<>'stopped' AND last_progress_at>=clock_timestamp()-interval '5 minutes'`, a.User.ID).Scan(&count); err != nil {
				return out, nativeplayback.ErrUpstream
			}
			if count >= limit {
				return out, nativeplayback.ErrStreamLimit
			}
			out.New = true
		}
	}
	if out.ID == "" {
		err = tx.QueryRow(ctx, `SELECT id::text FROM native_playback_sessions WHERE user_id=$1::uuid AND device_id=$2::uuid AND item_id=$3::bigint AND state='stopped' AND ended_at=started_at AND last_progress_at>=clock_timestamp()-interval '5 minutes' ORDER BY last_progress_at DESC,id LIMIT 1`, a.User.ID, a.Device.ID, item).Scan(&out.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, nativeplayback.ErrUpstream
		}
	}
	state := "stopped"
	if activate {
		state = "playing"
	}
	if out.ID == "" {
		out.ID = jobs.UUID()
		err = tx.QueryRow(ctx, `INSERT INTO native_playback_sessions(id,user_id,device_id,item_id,media_source_id,state,ended_at)VALUES($1,$2::uuid,$3::uuid,$4::bigint,$5,$6,CASE WHEN $6='stopped' THEN now() ELSE NULL END) RETURNING last_progress_at`, out.ID, a.User.ID, a.Device.ID, item, source, state).Scan(&out.At)
	} else {
		err = tx.QueryRow(ctx, `UPDATE native_playback_sessions SET media_source_id=$2,state=$3,last_progress_at=clock_timestamp(),ended_at=CASE WHEN $3='stopped' THEN started_at ELSE NULL END WHERE id=$1::uuid RETURNING last_progress_at`, out.ID, source, state).Scan(&out.At)
	}
	if err != nil {
		return out, nativeplayback.ErrUpstream
	}
	// Retain active/history rows. Only old, never-activated drafts are disposable.
	if _, err = tx.Exec(ctx, `DELETE FROM native_playback_sessions WHERE id IN(SELECT id FROM native_playback_sessions WHERE user_id=$1::uuid AND state='stopped' AND ended_at=started_at AND last_progress_at<clock_timestamp()-interval '5 minutes' LIMIT 200)`, a.User.ID); err != nil {
		return out, nativeplayback.ErrUpstream
	}
	if err = tx.Commit(ctx); err != nil {
		return out, nativeplayback.ErrUpstream
	}
	return out, nil
}
func (s *Service) release(parent context.Context, r reservation) error {
	if !r.New {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
	defer cancel()
	// A concurrent seek may already have refreshed this slot: its timestamp fence
	// prevents a late failed resolver from stopping that newer request.
	_, err := s.DB.Exec(ctx, `UPDATE native_playback_sessions SET state='stopped',ended_at=clock_timestamp() WHERE id=$1::uuid AND last_progress_at=$2 AND state<>'stopped'`, r.ID, r.At)
	return err
}
