package httpapi

import (
	"context"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/sites"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *API) ptTemplates(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	loaded, err := (sites.Repository{DB: a.Deps.DB}).LoadedGoTemplates(r.Context())
	if err != nil {
		fail(w, 503, "templates_unavailable", "模板状态不可用")
		return
	}
	write(w, 200, map[string]any{"templates": sites.GoTemplateCatalog(loaded)})
}
func (a *API) loadPTTemplates(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var in struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > len(sites.GoTemplates()) {
		fail(w, 400, "invalid_templates", "请选择站点模板")
		return
	}
	for _, id := range in.IDs {
		if _, ok := sites.GoTemplate(id); !ok {
			fail(w, 400, "invalid_templates", "未知站点模板")
			return
		}
	}
	if err := (sites.Repository{DB: a.Deps.DB}).LoadGoTemplates(r.Context(), in.IDs); err != nil {
		fail(w, 503, "templates_unavailable", "模板加载失败")
		return
	}
	a.ptTemplates(w, r)
}
func (a *API) ptUA(w http.ResponseWriter, r *http.Request) {
	write(w, 200, map[string]string{"userAgent": r.UserAgent()})
}
func (a *API) ptAccounts(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	rows, e := a.Deps.Integrations.Repository.List(r.Context())
	if e != nil {
		fail(w, 503, "pt_unavailable", "站点信息不可用")
		return
	}
	out := []map[string]any{}
	for _, row := range rows {
		if row.Kind != "ptsite" {
			continue
		}
		creds, e := a.Deps.Integrations.Repository.Get(r.Context(), row.ID)
		if e != nil {
			fail(w, 503, "pt_unavailable", "站点信息不可用")
			return
		}
		name := row.Name
		if t, ok := sites.GoTemplate(creds.String("definitionId")); ok {
			name = t.Name
		}
		out = append(out, map[string]any{"id": row.ID, "definitionId": creds.String("definitionId"), "name": name, "settings": row.Settings, "values": creds.Values, "revision": row.Revision, "state": row.LastCheckState})
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
func (a *API) savePTAccount(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	var in struct {
		DefinitionID string            `json:"definitionId"`
		Values       map[string]string `json:"values"`
		Revision     int64             `json:"revision"`
		UserID       string            `json:"userId"`
		RSSURL       string            `json:"rssUrl"`
		Rediscover   bool              `json:"rediscover"`
	}
	if !decode(w, r, &in) {
		return
	}
	t, ok := sites.GoTemplate(in.DefinitionID)
	if !ok {
		fail(w, 400, "invalid_site", "请选择内置站点模板")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	id := r.PathValue("id")
	values := map[string]string{}
	priorUserID := ""
	if id != "" {
		old, e := a.Deps.Integrations.Repository.Get(ctx, id)
		if e != nil || old.Record.Kind != "ptsite" || old.String("definitionId") != in.DefinitionID || old.Record.Revision != in.Revision {
			fail(w, 409, "pt_changed", "请刷新站点信息，编辑时不可更换站点")
			return
		}
		priorUserID = old.String("userId")
		for k, v := range old.Values {
			values[k] = v
		}
	}
	for k, v := range in.Values {
		if k != "cookie" && k != "userAgent" {
			fail(w, 400, "invalid_field", "只接受模板认证字段")
			return
		}
		if k == "cookie" {
			values[k] = v
		}
	}
	ua := strings.TrimSpace(in.Values["userAgent"])
	if ua == "" {
		ua = r.UserAgent()
	}
	if values["cookie"] == "" || ua == "" || len(ua) > 4096 || strings.ContainsAny(ua, "\r\n\x00") {
		fail(w, 400, "credentials_required", "请填写Cookie及有效UA")
		return
	}
	// Duplicate check is backed by a database unique index for concurrent requests.
	var exists bool
	e := a.Deps.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE kind='ptsite' AND settings->>'definitionId'=$1 AND id::text<>$2)`, in.DefinitionID, id).Scan(&exists)
	if e != nil || exists {
		fail(w, 409, "duplicate_site", "该站点已添加，请编辑现有站点")
		return
	}
	loaded, e := (sites.Repository{DB: a.Deps.DB}).LoadedGoTemplates(ctx)
	if e != nil {
		fail(w, 503, "templates_unavailable", "模板状态不可用")
		return
	}
	if !loaded[t.ID] {
		fail(w, 400, "template_not_loaded", "请先在站点模板页面加载该模板")
		return
	}
	d, e := (sites.Repository{DB: a.Deps.DB}).Get(ctx, t.ID)
	if e != nil {
		fail(w, 503, "templates_unavailable", "模板不可用")
		return
	}
	cfg, e := sites.DecodeStored(d.RawJSON, d.CommonJSON)
	if e != nil {
		fail(w, 503, "templates_unavailable", "模板不可用")
		return
	}
	manualID := strings.TrimSpace(in.UserID)
	if manualID != "" {
		if n, e := strconv.ParseUint(manualID, 10, 64); e != nil || n == 0 {
			fail(w, 400, "invalid_user_id", "用户ID须为站点数字ID")
			return
		} else {
			manualID = strconv.FormatUint(n, 10)
		}
	}
	seedID := manualID
	if seedID == "" {
		seedID = priorUserID
	}
	transport, e := a.Deps.Integrations.HTTPClient(ctx, "ptsite")
	if e != nil {
		fail(w, 503, "proxy_unavailable", "站点代理配置不可用")
		return
	}
	client := sites.Client{Config: cfg, Account: sites.Account{Endpoint: t.Domain, Cookie: values["cookie"], UserAgent: ua, UserID: seedID}, HTTP: transport}
	if e = client.Test(ctx); e != nil {
		fail(w, 400, "site_validation_failed", "保存前站点登录验证失败，请核对Cookie/UA/代理")
		return
	}
	autoID, autoErr := client.AuthenticatedUserID(ctx)
	if manualID != "" && autoErr == nil && autoID != "" && manualID != autoID {
		fail(w, 400, "user_id_mismatch", "手动用户ID与当前登录身份不一致")
		return
	}
	if manualID == "" && autoID != "" {
		seedID = autoID
	}
	rss, uid, rssErr := "", seedID, error(nil)
	source := "auto"
	if strings.TrimSpace(in.RSSURL) != "" && !in.Rediscover {
		rss, rssErr = client.ValidateRSS(ctx, in.RSSURL)
		source = "manual"
		if strings.TrimSpace(in.RSSURL) == values["rssUrl"] && id != "" {
			if old, e := a.Deps.Integrations.Repository.Get(ctx, id); e == nil && old.String("rssSource") != "" {
				source = old.String("rssSource")
			}
		}
		if rssErr != nil {
			fail(w, 400, "invalid_rss", "手动RSS必须是同站可读取的有效订阅地址，未覆盖原值")
			return
		}
	} else {
		rss, uid, rssErr = client.DiscoverRSS(ctx)
		if manualID != "" && uid != "" && manualID != uid {
			fail(w, 400, "user_id_mismatch", "手动用户ID与当前登录身份不一致")
			return
		}
		if rssErr != nil && values["rssUrl"] != "" {
			if existing, e := client.ValidateRSS(ctx, values["rssUrl"]); e == nil {
				rss = existing
				rssErr = nil
				source = "retained"
			}
		}
	}
	if manualID != "" {
		uid = manualID
	}
	rssState := "ready"
	if rssErr != nil {
		rssState = "unavailable"
	} else {
		values["rssUrl"] = rss
	}
	settings := map[string]any{"definitionId": t.ID, "endpoint": t.Domain, "userAgent": ua, "rssState": rssState, "rssSource": source}
	if uid != "" {
		settings["userId"] = uid
	} else if priorUserID != "" {
		settings["userId"] = priorUserID
	}

	record, e := a.Deps.Integrations.Repository.Save(ctx, id, integrations.Input{Kind: "ptsite", Name: t.Name, Enabled: true, Settings: settings, Secrets: values, Revision: in.Revision})
	if e != nil {
		integrationError(w, e)
		return
	}
	if checkedErr := a.Deps.Integrations.Repository.RecordCheck(ctx, record.ID, record.Revision, "ready", "站点登录已验证"); checkedErr != nil {
		fail(w, 503, "operation_failed", "读取或写入失败，请稍后重试")
		return
	}
	write(w, 200, map[string]any{"id": record.ID, "saved": true, "rssState": rssState, "rssMessage": func() string {
		if rssErr != nil {
			return "RSS未获取，已保留原值；可编辑后再次保存获取"
		}
		return "RSS已自动获取"
	}()})
}
