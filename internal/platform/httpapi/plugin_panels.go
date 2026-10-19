package httpapi

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"net/http"
	"time"
)

func (a *API) pluginTools(w http.ResponseWriter, r *http.Request) {
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	snap, e := s.Snapshot(r.Context(), r.PathValue("id"))
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out := []contracts.Descriptor{}
	for _, d := range snap.Metadata.Descriptors {
		if d.Kind != "tool" {
			continue
		}
		if _, _, e = s.Descriptor(r.Context(), snap.InstanceID, "tool", d.ID); e == nil {
			out = append(out, d)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
func (a *API) pluginToolCall(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Name         string          `json:"name"`
		Input        json.RawMessage `json:"input"`
		OperationKey string          `json:"operationKey"`
		Confirm      bool            `json:"confirm"`
	}
	if !decode(w, r, &q) {
		return
	}
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	actor, e := a.sessionAccount(r)
	if e != nil {
		fail(w, 401, "unauthorized", "需要管理员会话")
		return
	}
	out, e := s.ToolCall(r.Context(), actor.ID, r.PathValue("id"), q.Name, q.OperationKey, q.Input, q.Confirm)
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 200, out)
}
func (a *API) pluginAgentUnsupported(w http.ResponseWriter, r *http.Request) {
	fail(w, 501, "unsupported", "不提供LLM对话或自动Agent循环；AgentLLMProvider为N/A")
}
func (a *API) pluginWorkflowRead(w http.ResponseWriter, r *http.Request) {
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	ctx, stop := context.WithTimeout(r.Context(), 5*time.Second)
	defer stop()
	runner := pluginv2.WorkflowRunner{Store: s}
	var out any
	id, view := r.URL.Query().Get("workflowId"), r.URL.Query().Get("view")
	switch view {
	case "runs":
		page, limit, err := pluginPage(r)
		if err != nil {
			pluginV2Error(w, err)
			return
		}
		out, e = runner.Runs(ctx, id, page/limit+1)
	default:
		if view != "" && view != "definition" {
			fail(w, 404, "not_found", "未知工作流视图")
			return
		}
		if id != "" {
			out, e = runner.Definition(ctx, id)
		} else {
			start, limit, err := pluginPage(r)
			if err != nil {
				pluginV2Error(w, err)
				return
			}
			out, e = runner.List(ctx, start/limit+1)
		}
	}
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
func (a *API) pluginAuthStatus(w http.ResponseWriter, r *http.Request) {
	s, e := a.pluginExtensionStore()
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	out, e := (pluginv2.Broker{Store: s}).Status(r.Context(), r.PathValue("id"))
	if e != nil {
		pluginV2Error(w, e)
		return
	}
	write(w, 200, out)
}
