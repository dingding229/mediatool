package httpapi

import (
	"media-tool/internal/modules/subscriptions"
	"net/http"
)

func (a *API) subscriptionEngine() *subscriptions.Engine {
	if a.Deps.Integrations == nil {
		return nil
	}
	return &subscriptions.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}
}
func (a *API) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	engine := a.subscriptionEngine()
	if engine == nil {
		fail(w, 503, "subscriptions_unavailable", "请先连接数据库")
		return
	}
	list, err := engine.List(r.Context())
	if err != nil {
		fail(w, 503, "subscriptions_unavailable", "订阅记录暂不可用")
		return
	}
	write(w, 200, list)
}
func (a *API) saveSubscription(w http.ResponseWriter, r *http.Request) {
	engine := a.subscriptionEngine()
	if engine == nil {
		fail(w, 503, "subscriptions_unavailable", "请先连接数据库")
		return
	}
	var in subscriptions.Input
	if !decode(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	record, err := engine.Save(r.Context(), id, in)
	if err != nil {
		fail(w, 400, "invalid_subscription", err.Error())
		return
	}
	status := 200
	if id == "" {
		status = 201
	}
	write(w, status, record)
}
func (a *API) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	engine := a.subscriptionEngine()
	if engine == nil {
		fail(w, 503, "subscriptions_unavailable", "请先连接数据库")
		return
	}
	if err := engine.Delete(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 409, "subscription_delete_failed", "订阅正在评估或记录暂不可用，请停用并等待后重试")
		return
	}
	write(w, 200, map[string]bool{"deleted": true})
}
func (a *API) queueSubscription(w http.ResponseWriter, r *http.Request) {
	engine := a.subscriptionEngine()
	if engine == nil {
		fail(w, 503, "subscriptions_unavailable", "请先连接数据库")
		return
	}
	var body struct {
		Preview bool `json:"preview"`
	}
	if !decode(w, r, &body) {
		return
	}
	id, err := engine.Queue(r.Context(), r.PathValue("id"), body.Preview)
	if err != nil {
		fail(w, 409, "subscription_run_failed", "订阅任务无法创建，请刷新状态")
		return
	}
	write(w, 202, map[string]string{"id": id})
}
func (a *API) getSubscriptionRun(w http.ResponseWriter, r *http.Request) {
	engine := a.subscriptionEngine()
	if engine == nil {
		fail(w, 503, "subscriptions_unavailable", "请先连接数据库")
		return
	}
	run, err := engine.Run(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, "subscription_run_not_found", "订阅评估记录不存在")
		return
	}
	write(w, 200, run)
}
