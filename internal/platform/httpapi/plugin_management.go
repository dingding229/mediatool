package httpapi

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/jobs"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"net/http"
	"time"
)

func (a *API) pluginExtensions(w http.ResponseWriter, r *http.Request) {
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := s.Snapshot(r.Context(), r.PathValue("id"))
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 200, out)
}
func (a *API) pluginRuns(w http.ResponseWriter, r *http.Request) {
	start, limit, e := pluginPage(r)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := s.History(r.Context(), r.PathValue("id"), start, limit)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 200, out)
}
func (a *API) pluginRunExtension(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind         string          `json:"kind"`
		ID           string          `json:"id"`
		OperationKey string          `json:"operationKey"`
		Args         json.RawMessage `json:"args"`
		Confirm      bool            `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || len(in.OperationKey) < 8 || len(in.OperationKey) > 100 || len(in.Args) > 16<<10 {
		fail(w, 400, "invalid_plugin_request", "需要确认、幂等键及有界参数")
		return
	}
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	account, e := a.sessionAccount(r)
	if e != nil {
		fail(w, 401, "unauthorized", "请登录管理员")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if in.Kind == "service" {
		out, e := s.RunService(ctx, account.ID, r.PathValue("id"), in.ID, in.OperationKey)
		if e != nil {
			pluginV2Error(w, e)
			return
		}
		write(w, 202, map[string]string{"jobId": out})
		return
	}
	if in.Kind != "command" && in.Kind != "action" {
		fail(w, 400, "unsupported_plugin_kind", "此入口只允许命令、动作与服务")
		return
	}
	snap, d, e := s.Descriptor(ctx, r.PathValue("id"), in.Kind, in.ID)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	if len(in.Args) == 0 {
		in.Args = d.Data
		if in.Kind == "action" {
			in.Args = json.RawMessage(`{"data":{}}`)
		}
		if len(in.Args) == 0 {
			in.Args = json.RawMessage(`{}`)
		}
	}
	run, e := s.Submit(ctx, snap, d, in.Args, "admin:"+account.ID+":"+in.OperationKey, "", nil, time.Now().Add(10*time.Minute))
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	details, _ := json.Marshal(map[string]string{"instanceId": snap.InstanceID, "runId": run, "kind": in.Kind})
	if _, e = s.DB.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,'plugins.v2.extension.run',$2)`, account.ID, details); e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 202, map[string]string{"runId": run})
}
func (a *API) pluginKeyCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string    `json:"name"`
		Routes    []string  `json:"routes"`
		ExpiresAt time.Time `json:"expiresAt"`
		Confirm   bool      `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		fail(w, 400, "confirmation_required", "需要确认插件凭据权限")
		return
	}
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	account, e := a.sessionAccount(r)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := s.CreateAPIKey(r.Context(), account.ID, r.PathValue("id"), in.Name, in.Routes, in.ExpiresAt)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 201, out)
}
func (a *API) pluginKeyRevoke(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		fail(w, 400, "confirmation_required", "需要确认撤销")
		return
	}
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	account, e := a.sessionAccount(r)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	if e = s.RevokeAPIKey(r.Context(), account.ID, r.PathValue("id"), r.PathValue("key")); e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 200, map[string]bool{"revoked": true})
}
func (a *API) pluginReplay(w http.ResponseWriter, r *http.Request) {
	var in struct {
		After          int64 `json:"after"`
		Limit          int   `json:"limit"`
		Confirm        bool  `json:"confirm"`
		ConfirmUnknown bool  `json:"confirmUnknown"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm {
		fail(w, 400, "confirmation_required", "需要确认回放，unknown可能已产生副作用")
		return
	}
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	account, e := a.sessionAccount(r)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := s.Replay(r.Context(), account.ID, r.PathValue("id"), in.After, in.Limit, in.ConfirmUnknown)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 202, map[string]any{"count": out, "requestId": jobs.UUID()})
}

func (a *API) pluginUI(w http.ResponseWriter, r *http.Request) {
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	if len(r.URL.RawQuery) > 256 {
		pluginV2Error(w, pluginv2.ErrInvalid)
		return
	}
	out, e := s.UI(r.Context(), r.PathValue("id"), r.PathValue("kind"), r.URL.Query().Get("key"))
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}

func (a *API) pluginUISurfaces(w http.ResponseWriter, r *http.Request) {
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := s.UISurfaces(r.Context())
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
