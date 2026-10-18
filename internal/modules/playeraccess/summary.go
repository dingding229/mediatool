package playeraccess

import (
	"context"
	"media-tool/internal/platform/pluginactor"
)

type Counts struct{ Sampled, Allowed, Denied int64 }

// Summary never selects IPs, UA, request URLs, identity or policy rules. The
// event store is capped natively; LIMIT also bounds an oversized legacy store.
func (r Repository) Summary(ctx context.Context) (out Counts, err error) {
	if _, err = pluginactor.Require(ctx, "mediaserver.read"); err != nil {
		return out, err
	}
	err = r.DB.QueryRow(ctx, `SELECT count(*),count(*)FILTER(WHERE allowed),count(*)FILTER(WHERE NOT allowed)FROM(SELECT allowed FROM player_access_events ORDER BY created_at DESC,id LIMIT 10000)s`).Scan(&out.Sampled, &out.Allowed, &out.Denied)
	return out, err
}
