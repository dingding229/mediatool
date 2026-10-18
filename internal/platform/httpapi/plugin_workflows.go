package httpapi

import (
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"net/http"
)

// Management-listener only, protected admin session + non-GET CSRF middleware.
func (a *API) pluginWorkflow(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Operation    string             `json:"operation"`
		DAG          contracts.Workflow `json:"dag"`
		WorkflowID   string             `json:"workflowId"`
		RunID        string             `json:"runId"`
		OperationKey string             `json:"operationKey"`
		Revision     int64              `json:"revision"`
		Fingerprint  string             `json:"fingerprint"`
		Confirm      bool               `json:"confirm"`
	}
	if !decode(w, r, &q) {
		return
	}
	if !q.Confirm {
		pluginV2Error(w, pluginv2.ErrInvalid)
		return
	}
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	actor, e := a.sessionAccount(r)
	if e != nil {
		fail(w, 401, "unauthorized", "请登录管理员")
		return
	}
	runner := pluginv2.WorkflowRunner{Store: s}
	var out any
	switch q.Operation {
	case "save":
		out, e = runner.Save(r.Context(), actor.ID, q.DAG, q.Revision)
	case "approve":
		out, e = runner.Approve(r.Context(), actor.ID, q.WorkflowID, q.Fingerprint)
	case "run":
		out, e = runner.Start(r.Context(), actor.ID, q.WorkflowID, q.OperationKey)
	case "status":
		out, e = runner.Status(r.Context(), q.WorkflowID, q.RunID)
	case "cancel":
		out, e = runner.Cancel(r.Context(), actor.ID, q.WorkflowID, q.RunID)
	default:
		e = pluginv2.ErrInvalid
	}
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 200, out)
}
func (a *API) pluginAuthBinding(w http.ResponseWriter, r *http.Request) {
	var q struct {
		InstanceID, ProviderID, Subject, AccountID, Fingerprint string
		Confirm                                                 bool
	}
	if !decode(w, r, &q) {
		return
	}
	if !q.Confirm {
		pluginV2Error(w, pluginv2.ErrInvalid)
		return
	}
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	actor, e := a.sessionAccount(r)
	if e != nil {
		fail(w, 401, "unauthorized", "请登录管理员")
		return
	}
	e = (pluginv2.Broker{Store: s}).Bind(r.Context(), actor.ID, q.InstanceID, q.ProviderID, q.Subject, q.AccountID, q.Fingerprint)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 200, map[string]bool{"bound": true})
}
func (a *API) pluginAuthUnsupported(w http.ResponseWriter, r *http.Request) {
	fail(w, 501, "unsupported", "真实认证 provider UI/会话签发尚未开放；请使用内置密码登录")
}
