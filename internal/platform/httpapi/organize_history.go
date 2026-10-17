package httpapi

import (
	"context"
	"net/http"
	"time"
)

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
