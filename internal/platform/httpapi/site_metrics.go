package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/sites"
	"net/http"
	"time"
)

func (a *API) siteMetrics(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	rows, err := a.Deps.DB.Query(r.Context(), `SELECT i.id::text,i.name,i.enabled,i.revision,s.metrics,s.collected_at,s.attempted_at,coalesce(s.state,'not_synced'),coalesce(s.message,''),coalesce(s.account_revision,0),coalesce(s.definition_fingerprint,''),coalesce(d.definition,'{}'::jsonb),coalesce(d.common_definition,'{}'::jsonb),coalesce(d.source_commit,'') FROM integrations i LEFT JOIN site_metrics s ON s.account_id=i.id LEFT JOIN site_definitions d ON d.id=i.settings->>'definitionId' WHERE i.kind='ptsite' ORDER BY i.name`)
	if err != nil {
		fail(w, 503, "metrics_unavailable", "站点指标暂不可用")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, state, message, fingerprint, sourceCommit string
		var enabled bool
		var rev, saved int64
		var raw, definition, common []byte
		var collected, attempted *time.Time
		if err = rows.Scan(&id, &name, &enabled, &rev, &raw, &collected, &attempted, &state, &message, &saved, &fingerprint, &definition, &common, &sourceCommit); err != nil {
			fail(w, 503, "metrics_unavailable", "指标读取失败")
			return
		}
		var m any
		json.Unmarshal(raw, &m)
		out = append(out, map[string]any{"accountId": id, "name": name, "enabled": enabled, "metrics": m, "collectedAt": collected, "attemptedAt": attempted, "state": state, "message": message, "stale": collected != nil && (rev != saved || state == "failed" || fingerprint != sites.DefinitionFingerprint(definition, common, sourceCommit))})
	}
	if rows.Err() != nil {
		fail(w, 503, "metrics_unavailable", "指标读取失败")
		return
	}
	write(w, 200, out)
}
func (a *API) syncSiteMetrics(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var in struct {
		AccountID string `json:"accountId"`
	}
	if !decode(w, r, &in) {
		return
	}
	profiles, err := a.Deps.Integrations.Repository.List(r.Context())
	if err != nil {
		fail(w, 503, "metrics_unavailable", "账号读取失败")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	done := 0
	for _, p := range profiles {
		if p.Kind != "ptsite" || !p.Enabled || in.AccountID != "" && in.AccountID != p.ID {
			continue
		}
		done++
		if done > 16 {
			break
		}
		started := time.Now()
		bounded, stop := context.WithTimeout(ctx, 15*time.Second)
		client, e := a.Deps.Integrations.Site(bounded, p.ID)
		var m sites.Metrics
		if e == nil {
			m, e = client.Metrics(bounded)
		}
		stop()
		revision, fingerprint := p.Revision, ""
		if client.SourceRevision > 0 {
			revision, fingerprint = client.SourceRevision, client.DefinitionFingerprint
		}
		err = a.saveSiteMetrics(ctx, p.ID, revision, fingerprint, started, m, e)
		if errors.Is(err, errMetricsChanged) {
			continue
		}

		if err != nil {
			fail(w, 503, "metrics_save_failed", "采集结果保存失败，请刷新核对")
			return
		}
	}
	write(w, 200, map[string]any{"attempted": min(done, 16), "remainingMayExist": done > 16})
}

var errMetricsChanged = errors.New("site configuration changed while collecting metrics")

// The short save transaction locks the current account/definition, not the network
// call. Outdated or slower collections cannot overwrite newer snapshots.
func (a *API) saveSiteMetrics(ctx context.Context, id string, revision int64, fingerprint string, started time.Time, m sites.Metrics, collectionErr error) error {
	tx, err := a.Deps.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current int64
	var enabled bool
	var definitionID string
	if err = tx.QueryRow(ctx, `SELECT revision,enabled,coalesce(settings->>'definitionId','') FROM integrations WHERE id=$1 AND kind='ptsite' FOR SHARE`, id).Scan(&current, &enabled, &definitionID); err != nil {
		return errMetricsChanged
	}
	if !enabled || current != revision {
		return errMetricsChanged
	}
	if fingerprint != "" {
		var raw, common []byte
		var commit string
		if err = tx.QueryRow(ctx, `SELECT definition,common_definition,source_commit FROM site_definitions WHERE id=$1 FOR SHARE`, definitionID).Scan(&raw, &common, &commit); err != nil {
			return errMetricsChanged
		}
		if fingerprint != sites.DefinitionFingerprint(raw, common, commit) {
			return errMetricsChanged
		}
	}
	if collectionErr != nil {
		_, err = tx.Exec(ctx, `INSERT INTO site_metrics(account_id,state,message,account_revision,attempted_at) VALUES($1,'failed','采集失败：请检查站点登录、挑战和字段适配',$2,$3) ON CONFLICT(account_id) DO UPDATE SET state='failed',message=excluded.message,attempted_at=excluded.attempted_at WHERE site_metrics.attempted_at<=excluded.attempted_at`, id, revision, started)
	} else {
		raw, _ := json.Marshal(m)
		_, err = tx.Exec(ctx, `INSERT INTO site_metrics(account_id,metrics,collected_at,attempted_at,state,account_revision,definition_fingerprint,message) VALUES($1,$2,now(),$3,$4,$5,$6,'') ON CONFLICT(account_id) DO UPDATE SET metrics=excluded.metrics,collected_at=now(),attempted_at=excluded.attempted_at,state=excluded.state,account_revision=excluded.account_revision,definition_fingerprint=excluded.definition_fingerprint,message='' WHERE site_metrics.attempted_at<=excluded.attempted_at`, id, raw, started, sites.MetricsState(m), revision, fingerprint)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
