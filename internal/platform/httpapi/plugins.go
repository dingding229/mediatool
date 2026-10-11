package httpapi

import (
	"context"
	"media-tool/internal/modules/plugins"
	"net/http"
	"time"
)

func (a *API) pluginList(w http.ResponseWriter, r *http.Request) {
	items, err := (plugins.Registry{DB: a.Deps.DB}).List(r.Context())
	if err != nil {
		fail(w, 503, "plugins_unavailable", "扩展列表不可用")
		return
	}
	write(w, 200, items)
}
func (a *API) pluginSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Manifest plugins.Manifest `json:"manifest"`
		Enabled  bool             `json:"enabled"`
		Consent  bool             `json:"consent"`
		Revision int64            `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := (plugins.Registry{DB: a.Deps.DB}).Save(r.Context(), in.Manifest, in.Enabled, in.Consent, in.Revision); err != nil {
		fail(w, 400, "invalid_plugin", "Manifest无效、修订已变化或未明确授权数据传输；新插件必须停用导入")
		return
	}
	a.Deps.Logger.Info("extension configuration saved", "pluginId", in.Manifest.ID, "enabled", in.Enabled)
	write(w, 200, map[string]bool{"saved": true})
}
func (a *API) pluginDelete(w http.ResponseWriter, r *http.Request) {
	if err := (plugins.Registry{DB: a.Deps.DB}).Delete(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 400, "invalid_plugin", "内置插件不可删除，ID无效或删除未确认")
		return
	}
	write(w, 200, map[string]bool{"deleted": true})
}
func (a *API) organizeHistory(w http.ResponseWriter, r *http.Request) {
	if a.Deps.DB == nil {
		fail(w, 503, "history_unavailable", "数据库不可用")
		return
	}
	p, err := pagination(r)
	if err != nil {
		fail(w, 400, "invalid_page", "分页无效")
		return
	}
	state := r.URL.Query().Get("state")
	if len(state) > 40 {
		fail(w, 400, "invalid_state", "状态无效")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	rows, err := a.Deps.DB.Query(ctx, `SELECT id::text,operation,state,message,completed_steps,created_at,updated_at,plan->'items' FROM file_operations WHERE operation IN('organize','rename','move') AND state<>'preview' AND ($1='' OR state=$1) ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`, state, p.Limit, p.Offset)
	if err != nil {
		fail(w, 503, "history_unavailable", "整理历史不可用")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, operation, status, message string
		var done int
		var created, updated time.Time
		var details any
		if err = rows.Scan(&id, &operation, &status, &message, &done, &created, &updated, &details); err != nil {
			fail(w, 503, "history_unavailable", "历史读取失败")
			return
		}
		items = append(items, map[string]any{"id": id, "operation": operation, "state": status, "message": message, "completedSteps": done, "createdAt": created, "updatedAt": updated, "items": details})
	}
	if rows.Err() != nil {
		fail(w, 503, "history_unavailable", "历史读取失败")
		return
	}
	rows.Close()
	var total int
	if a.Deps.DB.QueryRow(ctx, `SELECT count(*) FROM file_operations WHERE operation IN('organize','rename','move') AND state<>'preview' AND ($1='' OR state=$1)`, state).Scan(&total) != nil {
		fail(w, 503, "history_unavailable", "历史计数失败")
		return
	}
	write(w, 200, map[string]any{"items": items, "total": total})
}
