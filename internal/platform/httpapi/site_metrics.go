package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/sitemetrics"
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
		iconURL := ""
		var m any
		if len(raw) > 0 && json.Unmarshal(raw, &m) != nil {
			fail(w, 503, "operation_failed", "读取或写入失败，请稍后重试")
			return
		}
		if cfg, e := sites.DecodeStored(definition, common); e == nil {
			if t, ok := sites.GoTemplate(cfg.ID); ok {
				name = t.Name
				iconURL = "/api/v1/pt/templates/" + t.ID + "/favicon?v=" + t.Version
			}
		}
		out = append(out, map[string]any{"accountId": id, "name": name, "iconUrl": iconURL, "enabled": enabled, "metrics": m, "collectedAt": collected, "attemptedAt": attempted, "state": state, "message": message, "stale": collected != nil && (rev != saved || state == "failed" || fingerprint != sites.DefinitionFingerprint(definition, common, sourceCommit))})
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
	done, failed := 0, 0
	for _, p := range profiles {
		if p.Kind != "ptsite" || !p.Enabled || in.AccountID != "" && in.AccountID != p.ID {
			continue
		}
		done++
		if done > 16 {
			break
		}
		err = (sitemetrics.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}).Collect(ctx, p.ID)
		if errors.Is(err, sitemetrics.ErrBusy) || errors.Is(err, sitemetrics.ErrChanged) {
			continue
		}
		if errors.Is(err, sitemetrics.ErrCollection) {
			failed++
			continue
		}
		if err != nil {
			fail(w, 503, "metrics_save_failed", "采集或保存失败，请刷新核对站点结果")
			return
		}
	}
	write(w, 200, map[string]any{"failed": failed, "attempted": min(done, 16), "remainingMayExist": done > 16})
}
