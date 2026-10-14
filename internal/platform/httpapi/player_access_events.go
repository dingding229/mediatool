package httpapi

import (
	"context"
	"media-tool/internal/modules/playeraccess"
	"net/http"
	"time"
)

type playerVisit struct {
	ID, UserID, UserName string
	Verified             bool
}
type playerVisitKey struct{}

func (a *API) playerAccessEvents(w http.ResponseWriter, r *http.Request) {
	p, err := pagination(r)
	if err != nil {
		fail(w, 400, "invalid_page", "分页无效")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	out, err := a.playerAccessRepo().FilteredEvents(ctx, r.URL.Query().Get("q"), r.URL.Query().Get("observationId"), r.URL.Query().Get("embyUserId"), p.Page, p.Limit)
	if err != nil {
		playerAccessError(w, err)
		return
	}
	write(w, 200, out)
}

func (a *API) playerAccessUsers(w http.ResponseWriter, r *http.Request) {
	if a.Deps.DB == nil {
		playerAccessError(w, playeraccess.ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	rows, err := a.Deps.DB.Query(ctx, `SELECT DISTINCT ON(emby_user_id) emby_user_id,emby_user_name FROM player_access_events WHERE emby_verified AND emby_user_id<>'' ORDER BY emby_user_id,created_at DESC`)
	if err != nil {
		playerAccessError(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var id, name string
		if err = rows.Scan(&id, &name); err != nil {
			playerAccessError(w, err)
			return
		}
		out = append(out, map[string]string{"id": id, "name": name})
	}
	if rows.Err() != nil {
		playerAccessError(w, rows.Err())
		return
	}
	write(w, 200, out)
}
