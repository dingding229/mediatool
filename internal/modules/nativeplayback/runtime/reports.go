package runtime

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/nativeplayback"
	"time"
)

type reportSession struct {
	id, user, device, item, source string
	state                          mediausers.PlaybackSessionState
	started, seen                  time.Time
	ended                          *time.Time
	position                       int64
	counted                        bool
}

const reportColumns = `id::text,replace(user_id::text,'-',''),device_id::text,item_id::text,media_source_id,state,started_at,last_progress_at,ended_at,position_ticks,play_counted`

func scanReport(row pgx.Row) (v reportSession, err error) {
	err = row.Scan(&v.id, &v.user, &v.device, &v.item, &v.source, &v.state, &v.started, &v.seen, &v.ended, &v.position, &v.counted)
	return
}
func validSessionID(id string) bool { var v pgtype.UUID; return v.Scan(id) == nil && v.Valid }

// All writes share T32's short per-user row lock; no filesystem/provider call
// occurs in this transaction. Each process applies the same persisted throttle.
func (s *Service) Report(parent context.Context, a nativeplayback.Actor, q nativeplayback.Report) (err error) {
	if s.DB == nil {
		return nativeplayback.ErrUpstream
	}
	if q.Kind != nativeplayback.ReportPlaying && q.Kind != nativeplayback.ReportProgress && q.Kind != nativeplayback.ReportStopped && q.Kind != nativeplayback.ReportPing || q.PositionTicks != nil && *q.PositionTicks < 0 || q.PlaySessionID != "" && !validSessionID(q.PlaySessionID) {
		return nativeplayback.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	// Resolve the server-owned session first so Ping and ID-only reports work.
	if q.PlaySessionID != "" {
		v, e := scanReport(s.DB.QueryRow(ctx, `SELECT `+reportColumns+` FROM native_playback_sessions WHERE id=$1::uuid`, q.PlaySessionID))
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return nativeplayback.ErrUpstream
		}
		if e == nil {
			if v.user != a.User.ID || v.device != a.Device.ID {
				return nativeplayback.ErrForbidden
			}
			if q.ItemID != "" && q.ItemID != v.item {
				return nativeplayback.ErrInvalid
			}
			q.ItemID = v.item
			if q.MediaSourceID == "" {
				q.MediaSourceID = v.source
			}
		}
	}
	if q.ItemID == "" {
		return nativeplayback.ErrInvalid
	}
	sources, e := s.sources(ctx, a, nativeplayback.Request{ItemID: q.ItemID, MediaSourceID: q.MediaSourceID})
	if e != nil {
		return e
	}
	// Without a session ID, preserve the recent session's version, not the first
	// version in the catalog. The lookup is rechecked under the user lock below.
	if q.PlaySessionID == "" && q.MediaSourceID == "" {
		var source string
		e = s.DB.QueryRow(ctx, `SELECT media_source_id FROM native_playback_sessions WHERE user_id=$1::uuid AND device_id=$2::uuid AND item_id=$3::bigint ORDER BY last_progress_at DESC,id LIMIT 1`, a.User.ID, a.Device.ID, q.ItemID).Scan(&source)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return nativeplayback.ErrUpstream
		}
		q.MediaSourceID = source
	}
	chosen, e := selectSource(sources, q.MediaSourceID, false)
	if e != nil {
		return e
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return nativeplayback.ErrUpstream
	}
	defer func() {
		if e := tx.Rollback(ctx); err == nil && e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = nativeplayback.ErrUpstream
		}
	}()
	streamLimit, e := lockReportActor(ctx, tx, a)
	if e != nil {
		return e
	}
	var v reportSession
	if q.PlaySessionID != "" {
		v, e = scanReport(tx.QueryRow(ctx, `SELECT `+reportColumns+` FROM native_playback_sessions WHERE id=$1::uuid FOR UPDATE`, q.PlaySessionID))
	} else {
		v, e = scanReport(tx.QueryRow(ctx, `SELECT `+reportColumns+` FROM native_playback_sessions WHERE user_id=$1::uuid AND device_id=$2::uuid AND item_id=$3::bigint AND media_source_id=$4 ORDER BY last_progress_at DESC,id LIMIT 1 FOR UPDATE`, a.User.ID, a.Device.ID, q.ItemID, chosen.Id))
	}
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return nativeplayback.ErrUpstream
	}
	found := e == nil
	if found && (v.user != a.User.ID || v.device != a.Device.ID || v.item != q.ItemID) {
		return nativeplayback.ErrForbidden
	}
	terminal := found && v.state == mediausers.PlaybackStopped && v.ended != nil && v.ended.After(v.started)
	// Stopped is terminal. Late Progress/Ping/Stopped cannot resurrect or count it.
	// A new Playing without an explicit ID starts a distinct replay.
	if terminal && (q.Kind != nativeplayback.ReportPlaying || q.PlaySessionID != "") {
		return commitReport(ctx, tx)
	}
	if terminal {
		found = false
	}
	now := time.Now()
	if !found {
		v = reportSession{id: jobs.UUID(), user: a.User.ID, device: a.Device.ID, item: q.ItemID, source: chosen.Id, state: mediausers.PlaybackStopped, started: now, seen: now}
		if q.PlaySessionID != "" {
			v.id = q.PlaySessionID
		}
	}
	state := mediausers.PlaybackPlaying
	if v.state == mediausers.PlaybackPaused && q.Kind != nativeplayback.ReportPlaying {
		state = mediausers.PlaybackPaused
	}
	if q.IsPaused != nil {
		if *q.IsPaused {
			state = mediausers.PlaybackPaused
		} else {
			state = mediausers.PlaybackPlaying
		}
	}
	if q.Kind == nativeplayback.ReportStopped {
		state = mediausers.PlaybackStopped
	}
	position := v.position
	if q.PositionTicks != nil {
		position = *q.PositionTicks
	}
	jump := position - v.position
	if jump < 0 {
		jump = -jump
	}
	if found && (q.Kind == nativeplayback.ReportProgress || q.Kind == nativeplayback.ReportPing) && state == v.state && chosen.Id == v.source && jump <= 300000000 && now.Sub(v.seen) < 10*time.Second {
		return commitReport(ctx, tx)
	}
	keys := []string{v.source, chosen.Id}
	if state != mediausers.PlaybackStopped {
		// Same device/source is a single slot, including a draft activated by a
		// client report before the first stream request. Merge an older duplicate.
		var active int
		if e = tx.QueryRow(ctx, `SELECT count(DISTINCT(device_id,media_source_id)) FROM native_playback_sessions WHERE user_id=$1::uuid AND state<>'stopped' AND last_progress_at>=clock_timestamp()-interval '5 minutes' AND NOT(device_id=$2::uuid AND media_source_id=ANY($3::text[]))`, a.User.ID, a.Device.ID, keys).Scan(&active); e != nil {
			return nativeplayback.ErrUpstream
		}
		if active >= streamLimit {
			return nativeplayback.ErrStreamLimit
		}
	}
	{
		var counted bool
		if e = tx.QueryRow(ctx, `SELECT coalesce(bool_or(play_counted),false) FROM native_playback_sessions WHERE user_id=$1::uuid AND device_id=$2::uuid AND media_source_id=ANY($3::text[]) AND state<>'stopped' AND last_progress_at>=clock_timestamp()-interval '5 minutes' AND id<>$4::uuid`, a.User.ID, a.Device.ID, keys, v.id).Scan(&counted); e != nil {
			return nativeplayback.ErrUpstream
		}
		v.counted = v.counted || counted
		if _, e = tx.Exec(ctx, `UPDATE native_playback_sessions SET state='stopped',ended_at=clock_timestamp() WHERE user_id=$1::uuid AND device_id=$2::uuid AND media_source_id=ANY($3::text[]) AND state<>'stopped' AND id<>$4::uuid`, a.User.ID, a.Device.ID, keys, v.id); e != nil {
			return nativeplayback.ErrUpstream
		}
	}
	var duration int64
	if q.Kind != nativeplayback.ReportPing {
		// Only a trusted successful snapshot supplies thresholds; raw ffprobe JSON,
		// client RunTimeTicks and estimated TMDB runtime are not used.
		if e = tx.QueryRow(ctx, `SELECT coalesce((SELECT (p.result->>'durationTicks')::bigint FROM media_assets a JOIN media_probe_results p ON p.cloud_file_id=a.cloud_file_id WHERE a.id=$1::uuid AND p.probed_at IS NOT NULL AND p.result->>'durationTicks' ~ '^[0-9]{1,18}$'),0)`, chosen.Id).Scan(&duration); e != nil {
			return nativeplayback.ErrUpstream
		}
		hasPosition := q.PositionTicks != nil || found && v.state != mediausers.PlaybackStopped && v.source == chosen.Id
		if !hasPosition {
			duration = 0
		}
		resume, complete := nativeplayback.ViewingProgress(position, duration)
		credit := complete && !v.counted
		if _, e = tx.Exec(ctx, `INSERT INTO media_user_item_data(user_id,item_id,position_ticks,played,play_count,last_played_at)VALUES($1::uuid,$2::bigint,$3,$4,$5,clock_timestamp()) ON CONFLICT(user_id,item_id) DO UPDATE SET position_ticks=CASE WHEN $6 THEN EXCLUDED.position_ticks ELSE media_user_item_data.position_ticks END,played=media_user_item_data.played OR EXCLUDED.played,play_count=media_user_item_data.play_count+EXCLUDED.play_count,last_played_at=EXCLUDED.last_played_at,updated_at=clock_timestamp()`, a.User.ID, q.ItemID, resume, complete, boolInt(credit), hasPosition); e != nil {
			return nativeplayback.ErrUpstream
		}
		v.counted = v.counted || complete
	}
	if !found {
		_, e = tx.Exec(ctx, `INSERT INTO native_playback_sessions(id,user_id,device_id,item_id,media_source_id,state,position_ticks,play_counted,ended_at)VALUES($1::uuid,$2::uuid,$3::uuid,$4::bigint,$5,$6,$7,$8,CASE WHEN $6='stopped' THEN clock_timestamp() ELSE NULL END)`, v.id, a.User.ID, a.Device.ID, q.ItemID, chosen.Id, state, position, v.counted)
	} else {
		var tag interface{ RowsAffected() int64 }
		tag, e = tx.Exec(ctx, `UPDATE native_playback_sessions SET state=$2,position_ticks=$3,play_counted=$4,media_source_id=$5,last_progress_at=clock_timestamp(),ended_at=CASE WHEN $2='stopped' THEN clock_timestamp() ELSE NULL END WHERE id=$1::uuid`, v.id, state, position, v.counted, chosen.Id)
		if e == nil && tag.RowsAffected() != 1 {
			return nativeplayback.ErrUpstream
		}
	}
	if e != nil {
		return nativeplayback.ErrUpstream
	}
	return commitReport(ctx, tx)
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
func commitReport(ctx context.Context, tx pgx.Tx) error {
	if tx.Commit(ctx) != nil {
		return nativeplayback.ErrUpstream
	}
	return nil
}

func lockReportActor(ctx context.Context, tx pgx.Tx, a nativeplayback.Actor) (limit int, err error) {
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT enabled AND NOT must_change_password AND (expires_at IS NULL OR expires_at>clock_timestamp()) AND credential_version=$2,max_streams FROM media_users WHERE id=$1::uuid FOR UPDATE`, a.User.ID, a.User.CredentialVersion).Scan(&allowed, &limit); err != nil {
		return 0, nativeplayback.ErrUpstream
	}
	if !allowed {
		return 0, nativeplayback.ErrForbidden
	}
	var owned bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_user_devices WHERE id=$1::uuid AND user_id=$2::uuid)`, a.Device.ID, a.User.ID).Scan(&owned); err != nil {
		return 0, nativeplayback.ErrUpstream
	}
	if !owned {
		return 0, nativeplayback.ErrForbidden
	}
	return limit, nil
}
