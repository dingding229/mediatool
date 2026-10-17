package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/dataview"
	"media-tool/internal/modules/releasename"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/logging"
	"net/http"
	"strconv"
	"time"
)

func (a *API) getApplicationSettings(w http.ResponseWriter, r *http.Request) {
	v, err := (settings.Repository{DB: a.Deps.DB}).Get(r.Context())
	if err != nil {
		fail(w, 503, "settings_unavailable", "系统设置不可用")
		return
	}
	write(w, 200, v)
}
func (a *API) saveApplicationSettings(w http.ResponseWriter, r *http.Request) {
	var in settings.Record
	if !decode(w, r, &in) {
		return
	}
	if a.Deps.DB == nil {
		fail(w, 503, "settings_unavailable", "数据库不可用")
		return
	}
	current, readErr := (settings.Repository{DB: a.Deps.DB}).Get(r.Context())
	if readErr != nil {
		fail(w, 503, "settings_unavailable", "设置不可用")
		return
	}
	if current.Value.Access != in.Value.Access {
		fail(w, 400, "domain_endpoint_required", "请在访问与域名分组保存域名")
		return
	}
	v, err := (settings.Repository{DB: a.Deps.DB}).Save(r.Context(), in.Value, in.Revision)
	if errors.Is(err, settings.ErrRevision) {
		fail(w, 409, "settings_changed", "设置已更新，请刷新后重试")
		return
	}
	if err != nil {
		fail(w, 400, "invalid_settings", "模板或缓存有效期无效，请核对变量及范围")
		return
	}
	a.Deps.Logger.Info("application settings updated")
	write(w, 200, v)
}
func (a *API) parseReleaseName(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Name) > 2048 || in.Name == "" {
		fail(w, 400, "invalid_name", "文件名无效")
		return
	}
	// Retired WASM definitions are not queried. F2 introduces NameRecognize dispatch.
	hint, messages := releasename.Parse(in.Name), []string{}
	write(w, 200, map[string]any{"hint": hint, "pluginMessages": messages})
}
func (a *API) logsSummary(w http.ResponseWriter, r *http.Request) {
	if a.Deps.Logs == nil {
		fail(w, 503, "logs_unavailable", "日志存储未启动")
		return
	}
	v, err := a.Deps.Logs.Stats()
	if err != nil {
		fail(w, 503, "logs_unavailable", "日志状态不可用")
		return
	}
	write(w, 200, v)
}
func (a *API) readLogs(w http.ResponseWriter, r *http.Request) {
	if a.Deps.Logs == nil {
		fail(w, 503, "logs_unavailable", "日志存储未启动")
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil {
			fail(w, 400, "invalid_limit", "条数无效")
			return
		}
		limit = n
	}
	v, err := a.Deps.Logs.Read(r.URL.Query().Get("level"), limit)
	if err != nil {
		fail(w, 400, "logs_unavailable", "日志查询失败，级别或条数无效")
		return
	}
	write(w, 200, v)
}
func (a *API) queryLogs(w http.ResponseWriter, r *http.Request) {
	if a.Deps.Logs == nil {
		fail(w, 503, "logs_unavailable", "日志存储未启动")
		return
	}
	values := r.URL.Query()
	q := logging.Query{Instance: values.Get("instance"), Keyword: values.Get("keyword"), Level: values.Get("level"), Module: values.Get("module"), Page: 1, PageSize: 100}
	for key, target := range map[string]*int{"page": &q.Page, "pageSize": &q.PageSize} {
		if raw := values.Get(key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				fail(w, 400, "invalid_log_query", "日志页码或条数无效")
				return
			}
			*target = n
		}
	}
	for key, target := range map[string]*time.Time{"from": &q.From, "to": &q.To, "before": &q.Before} {
		if raw := values.Get(key); raw != "" {
			v, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				fail(w, 400, "invalid_log_query", "日志时间格式无效")
				return
			}
			*target = v
		}
	}
	if err := q.Validate(); err != nil {
		fail(w, 400, "invalid_log_query", "日志筛选、时间范围或页码无效")
		return
	}
	if !q.Before.IsZero() && q.Before.After(time.Now().Add(time.Minute)) {
		fail(w, 400, "invalid_log_query", "日志查询截止时间无效")
		return
	}
	result, err := a.Deps.Logs.Query(r.Context(), q)
	if err != nil {
		fail(w, 503, "logs_unavailable", "日志读取失败，请刷新重试")
		return
	}
	write(w, 200, result)
}
func (a *API) clearLogs(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || a.Deps.Logs == nil {
		fail(w, 400, "confirmation_required", "请明确确认清理程序文件日志")
		return
	}
	if err := a.Deps.Logs.Clear(); err != nil {
		fail(w, 503, "logs_clear_failed", "清理未确认成功")
		return
	}
	a.Deps.Logger.Info("application file logs cleared by administrator")
	write(w, 200, map[string]bool{"cleared": true, "auditPreserved": true})
}
func (a *API) setLogLevel(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Level string `json:"level"`
	}
	if !decode(w, r, &in) {
		return
	}
	if a.Deps.Logs == nil || a.Deps.Logs.SetLevel(in.Level) != nil {
		fail(w, 400, "invalid_log_level", "支持 debug/info/warn/error")
		return
	}
	write(w, 200, map[string]string{"level": in.Level, "persistence": "本次运行；默认级别在Compose中修改"})
}
func (a *API) dataTables(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	v, err := (dataview.Engine{DB: a.Deps.DB, Redis: a.Deps.Redis}).Tables(ctx)
	if err != nil {
		fail(w, 503, "data_unavailable", "表信息不可用")
		return
	}
	write(w, 200, v)
}
func (a *API) dataRows(w http.ResponseWriter, r *http.Request) {
	p, err := pagination(r)
	if err != nil {
		fail(w, 400, "invalid_page", "分页无效")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	v, err := (dataview.Engine{DB: a.Deps.DB}).Rows(ctx, r.PathValue("table"), p.Page, min(p.Limit, 50))
	if err != nil {
		fail(w, 400, "data_unavailable", "表不存在、查询超限或数据不可用")
		return
	}
	write(w, 200, v)
}
func (a *API) redisKeys(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	cursor := r.URL.Query().Get("cursor")
	if cursor == "" {
		cursor = "0"
	}
	v, err := (dataview.Engine{Redis: a.Deps.Redis}).RedisKeys(ctx, cursor, r.URL.Query().Get("pattern"))
	if err != nil {
		fail(w, 400, "redis_unavailable", "Redis查询不可用，游标或模式无效")
		return
	}
	write(w, 200, v)
}
func (a *API) redisValue(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	v, err := (dataview.Engine{Redis: a.Deps.Redis}).RedisValue(ctx, r.URL.Query().Get("key"))
	if err != nil {
		fail(w, 400, "redis_unavailable", "键不可用或超出显示上限")
		return
	}
	write(w, 200, v)
}
