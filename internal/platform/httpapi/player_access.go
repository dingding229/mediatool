package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/playeraccess"
	"media-tool/internal/modules/settings"
	"net/http"
	"time"
)

type PlayerAdmission interface {
	Check(context.Context, playeraccess.Request) (playeraccess.Decision, error)
}

func (a *API) playerAccessRepo() playeraccess.Repository {
	return playeraccess.Repository{DB: a.Deps.DB}
}
func playerAccessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, playeraccess.ErrConflict):
		fail(w, 409, "player_ua_conflict", "播放器策略已被修改，请刷新后重试")
	case errors.Is(err, playeraccess.ErrNotFound):
		fail(w, 404, "player_ua_not_found", "访问记录已被清理或淘汰，请刷新后重试")
	case errors.Is(err, playeraccess.ErrUnavailable):
		fail(w, 503, "player_ua_unavailable", "播放器UA策略暂不可用，未放行播放请求")
	default:
		fail(w, 400, "invalid_player_ua", err.Error())
	}
}
func (a *API) admitPlayer(w http.ResponseWriter, r *http.Request) bool {
	entry := playeraccess.Entry(r.URL.Path)
	if entry == "" {
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	if len(r.Header.Values("User-Agent")) > 1 || !playeraccess.ValidUA(r.UserAgent()) {
		fail(w, 400, "invalid_player_ua", "User-Agent重复、含控制字符或超过4096字节")
		return false
	}
	var checker PlayerAdmission = a.Deps.PlayerAccess
	if checker == nil {
		// Pure development/unit-test mode has no durable policy. Real deployment
		// never treats an unavailable configured database as a disabled restriction.
		if a.Deps.DB == nil && a.Config.Development() {
			return true
		}
		checker = a.playerAccessRepo()
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	prefs, prefsErr := (settings.Repository{DB: a.Deps.DB}).Get(ctx)
	var trusted []string
	if prefsErr == nil {
		trusted = prefs.Value.TrustedProxyCIDRs
	}
	addr := playeraccess.ClientAddress(r, trusted)
	request := playeraccess.Request{UserAgent: r.UserAgent(), Entry: entry, Method: r.Method, ClientIP: addr.ClientIP, ConnectionIP: addr.ConnectionIP, IPSource: addr.Source, IPPublic: addr.Public}
	d, err := checker.Check(ctx, request)
	if err != nil {
		a.Deps.Logger.Error("player UA policy unavailable", "module", "playback", "entry", entry, "userAgent", r.UserAgent())
		playerAccessError(w, playeraccess.ErrUnavailable)
		return false
	}
	if a.Deps.DB != nil {
		id, eventErr := a.playerAccessRepo().RecordEvent(ctx, request, d)
		if eventErr != nil {
			playerAccessError(w, playeraccess.ErrUnavailable)
			return false
		}
		if visit, ok := r.Context().Value(playerVisitKey{}).(*playerVisit); ok {
			visit.ID = id
		}
	}
	fields := []any{"module", "playback", "entry", entry, "userAgent", r.UserAgent(), "clientIp", addr.ClientIP, "connectionIp", addr.ConnectionIP, "ipSource", addr.Source, "ipPublic", addr.Public, "uaAllowed", d.Allowed, "uaReason", d.Reason, "uaRuleId", d.RuleID, "uaPolicyRevision", d.PolicyRevision}
	if !d.Allowed {
		a.Deps.Logger.Warn("player UA denied", fields...)
		fail(w, 403, "player_ua_denied", "当前播放器User-Agent不在白名单中，请联系管理员添加")
		return false
	}
	a.Deps.Logger.Debug("player UA admitted; media authorization still required", fields...)
	return true
}
func (a *API) getPlayerAccess(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	p, err := a.playerAccessRepo().Policy(ctx)
	if err != nil {
		playerAccessError(w, err)
		return
	}
	write(w, 200, p)
}
func (a *API) savePlayerAccess(w http.ResponseWriter, r *http.Request) {
	var in playeraccess.Policy
	if !decodeLimit(w, r, &in, 600<<10) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	p, err := a.playerAccessRepo().Save(ctx, in)
	if err != nil {
		playerAccessError(w, err)
		return
	}
	write(w, 200, p)
}
func (a *API) playerObservations(w http.ResponseWriter, r *http.Request) {
	p, err := pagination(r)
	if err != nil {
		playerAccessError(w, playeraccess.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	out, err := a.playerAccessRepo().Observations(ctx, r.URL.Query().Get("q"), r.URL.Query().Get("filter"), p.Page, p.Limit)
	if err != nil {
		playerAccessError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) allowPlayerObservation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64  `json:"revision"`
		Label    string `json:"label"`
		Confirm  bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		playerAccessError(w, playeraccess.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	p, err := a.playerAccessRepo().AllowObservation(ctx, r.PathValue("id"), in.Label, in.Revision)
	if err != nil {
		playerAccessError(w, err)
		return
	}
	write(w, 200, p)
}
func (a *API) clearPlayerObservations(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
		Confirm  bool  `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := a.playerAccessRepo().Clear(ctx, in.Confirm, in.Revision); err != nil {
		playerAccessError(w, err)
		return
	}
	write(w, 200, map[string]bool{"cleared": true})
}
func (a *API) registerPlayerAccess(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v1/player-access/events", a.protected(a.playerAccessEvents))
	mux.HandleFunc("GET /api/v1/player-access/users", a.protected(a.playerAccessUsers))
	mux.HandleFunc("GET /api/v1/player-access/policy", a.protected(a.getPlayerAccess))
	mux.HandleFunc("PUT /api/v1/player-access/policy", a.protected(a.savePlayerAccess))
	mux.HandleFunc("GET /api/v1/player-access/observations", a.protected(a.playerObservations))
	mux.HandleFunc("POST /api/v1/player-access/observations/{id}/allow", a.protected(a.allowPlayerObservation))
	mux.HandleFunc("POST /api/v1/player-access/observations/clear", a.protected(a.clearPlayerObservations))
}
