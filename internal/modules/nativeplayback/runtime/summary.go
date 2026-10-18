package runtime

import (
	"context"
	"media-tool/internal/platform/pluginactor"
)

type ActiveCounts struct{ Playing, Paused int64 }

func (r SessionRepository) Summary(ctx context.Context) (out ActiveCounts, err error) {
	if _, err = pluginactor.Require(ctx, "mediaserver.read"); err != nil {
		return out, err
	}
	err = r.DB.QueryRow(ctx, `SELECT count(*)FILTER(WHERE state='playing'),count(*)FILTER(WHERE state='paused')FROM native_playback_sessions WHERE state<>'stopped'AND last_progress_at>=statement_timestamp()-interval '5 minutes'`).Scan(&out.Playing, &out.Paused)
	return out, err
}
