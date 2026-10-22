package httpapi

import (
	"context"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"net/http"
	"time"
)

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
