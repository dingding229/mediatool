package httpapi

import (
	"context"
	"media-tool/internal/modules/messaging"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"net/http"
	"time"
)

// Host-only callback provisioning. Raw chat/user IDs and the signed callback
// never enter plugin event DTOs; this route exists only on the admin listener.
func (a *API) issueCommandAction(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ChatID    int64  `json:"chatId"`
		UserID    int64  `json:"userId"`
		Command   string `json:"command"`
		Arguments string `json:"arguments"`
	}
	if !decode(w, r, &in) {
		return
	}
	if a.Deps.DB == nil || a.Deps.Integrations == nil {
		fail(w, 503, "ingress_unavailable", "入站未配置")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	ingress := messaging.CommandIngress{DB: a.Deps.DB, Connections: a.Deps.Integrations, Plugins: pluginv2.Store{DB: a.Deps.DB, Vault: a.Deps.Integrations.Repository.Vault}}
	value, e := ingress.IssueAction(ctx, in.ChatID, in.UserID, in.Command, in.Arguments)
	if e != nil {
		fail(w, 400, "action_rejected", "命令或管理员允许列表不匹配")
		return
	}
	// No credentials, arguments, raw actor IDs or signature are written to audit.
	if _, e = a.Deps.DB.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','plugins.messaging.action.issue','{}')`); e != nil {
		fail(w, 503, "action_unavailable", "无法记录审计")
		return
	}
	write(w, 200, map[string]any{"callbackData": value, "expiresIn": 300})
}
