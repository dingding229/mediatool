package sitemetrics

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/sites"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"strconv"
	"time"
)

type Engine struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
}

var ErrChanged = errors.New("site configuration changed while collecting metrics")

// The short save transaction locks the current account/definition, not the network
// call. Outdated or slower collections cannot overwrite newer snapshots.
func (e Engine) Save(ctx context.Context, id string, revision int64, fingerprint string, started time.Time, m sites.Metrics, collectionErr error) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var current int64
	var enabled bool
	var definitionID string
	if err = tx.QueryRow(ctx, `SELECT revision,enabled,coalesce(settings->>'definitionId','') FROM integrations WHERE id=$1 AND kind='ptsite' FOR SHARE`, id).Scan(&current, &enabled, &definitionID); err != nil {
		return ErrChanged
	}
	if !enabled || current != revision {
		return ErrChanged
	}
	if fingerprint != "" {
		var raw, common []byte
		var commit string
		if err = tx.QueryRow(ctx, `SELECT definition,common_definition,source_commit FROM site_definitions WHERE id=$1 FOR SHARE`, definitionID).Scan(&raw, &common, &commit); err != nil {
			return ErrChanged
		}
		if fingerprint != sites.DefinitionFingerprint(raw, common, commit) {
			return ErrChanged
		}
	}
	var changed pgconn.CommandTag
	if collectionErr != nil {
		changed, err = tx.Exec(ctx, `INSERT INTO site_metrics(account_id,state,message,account_revision,attempted_at) VALUES($1,'failed','采集失败：请检查站点登录、挑战和字段适配',$2,$3) ON CONFLICT(account_id) DO UPDATE SET state='failed',message=excluded.message,attempted_at=excluded.attempted_at WHERE site_metrics.attempted_at<excluded.attempted_at`, id, revision, started)
	} else {
		raw, _ := json.Marshal(m)
		changed, err = tx.Exec(ctx, `INSERT INTO site_metrics(account_id,metrics,collected_at,attempted_at,state,account_revision,definition_fingerprint,message) VALUES($1,$2,now(),$3,$4,$5,$6,'') ON CONFLICT(account_id) DO UPDATE SET metrics=excluded.metrics,collected_at=now(),attempted_at=excluded.attempted_at,state=excluded.state,account_revision=excluded.account_revision,definition_fingerprint=excluded.definition_fingerprint,message='' WHERE site_metrics.attempted_at<excluded.attempted_at`, id, raw, started, sites.MetricsState(m), revision, fingerprint)
	}
	if err != nil {
		return err
	}
	// Each successful, newer collection is a separate fact; failed/stale/replayed attempts emit nothing.
	if collectionErr == nil && changed.RowsAffected() == 1 {
		if err = outbox.EmitVersionTx(ctx, tx, "site.metrics.refreshed", id, strconv.FormatInt(revision, 10)+":"+started.UTC().Format(time.RFC3339Nano), contracts.SiteMetricsRefreshed{AccountID: id, AccountRevision: revision, State: sites.MetricsState(m), AttemptedAt: started}); err != nil {
			return err
		}
	}
	return dbutil.Commit(ctx, tx)
}
