package httpapi

import (
	"context"
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
	out, err := a.playerAccessRepo().Events(ctx, r.URL.Query().Get("q"), r.URL.Query().Get("observationId"), p.Page, p.Limit)
	if err != nil {
		playerAccessError(w, err)
		return
	}
	write(w, 200, out)
}
