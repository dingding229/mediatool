package httpapi

import (
	"encoding/json"
	"io"
	"media-tool/internal/modules/plugins"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"strconv"
)

func (a *API) pluginPackage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, plugins.MaxPackageBytes+(128<<10))
	if r.ParseMultipartForm(plugins.MaxPackageBytes) != nil {
		fail(w, 400, "invalid_package", "ZIP超过8MiB或上传格式无效")
		return
	}
	if r.MultipartForm != nil {
		defer deferutil.Do(func() error { return r.MultipartForm.RemoveAll() }, "Temporary cleanup or final unlock only; resource close also releases locks and the primary result is authoritative")
	}
	file, _, e := r.FormFile("package")
	if e != nil {
		fail(w, 400, "invalid_package", "请选择ZIP插件包")
		return
	}
	defer deferutil.Close(file, "Read/descriptor teardown only; primary operations handle their own I/O result")
	raw, e := io.ReadAll(io.LimitReader(file, plugins.MaxPackageBytes+1))
	if e != nil {
		fail(w, 400, "invalid_package", "压缩包读取失败")
		return
	}
	p, e := plugins.ParsePackage(raw)
	if e != nil {
		fail(w, 400, "invalid_package", "插件包无效：必须包含manifest.json与有效plugin.wasm；不支持旧Webhook/rules格式、链接或越界路径")
		return
	}
	if r.URL.Query().Get("preview") == "true" {
		write(w, 200, p)
		return
	}
	revision, e := strconv.ParseInt(r.FormValue("revision"), 10, 64)
	if e != nil || revision < 0 || r.FormValue("digest") != p.Digest || r.FormValue("confirm") != "true" {
		fail(w, 400, "package_confirmation_required", "请先预览并确认同一个ZIP和插件修订")
		return
	}
	if e = a.pluginRegistry().Install(r.Context(), p, revision); e != nil {
		fail(w, 409, "package_install_conflict", "安装未完成；插件可能已存在、修订改变或存储不可用，请刷新核对")
		return
	}
	write(w, 200, map[string]any{"installed": true, "enabled": false, "id": p.Manifest.ID})
}
func (a *API) pluginSettings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	values, secretKeys, rev, e := a.pluginRegistry().Settings(r.Context(), id)
	if e != nil {
		fail(w, 404, "plugin_not_found", "插件设置不存在")
		return
	}
	masked := []string{}
	for _, key := range secretKeys {
		if values[key] != "" {
			masked = append(masked, key)
		}
	}

	write(w, 200, map[string]any{"values": values, "secretFields": masked, "revision": rev})
}
func (a *API) savePluginSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Values   map[string]string `json:"values"`
		Clear    []string          `json:"clear"`
		Revision int64             `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if e := a.pluginRegistry().SaveSettings(r.Context(), r.PathValue("id"), in.Values, in.Clear, in.Revision); e != nil {
		fail(w, 409, "plugin_settings_conflict", "设置无效或修订已改变，请刷新核对")
		return
	}
	write(w, 200, map[string]bool{"saved": true, "disabledUntilReauthorized": true})
}
func (a *API) testPlugin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Filename string `json:"filename"`
		Confirm  bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirm || len(in.Filename) > 1024 {
		fail(w, 400, "test_confirmation_required", "请明确确认测试，不会修改真实媒体")
		return
	}
	raw, e := a.pluginRegistry().Test(r.Context(), r.PathValue("id"), in.Filename)
	if e != nil {
		fail(w, 502, "plugin_test_failed", "插件未启用、设置不完整或执行失败；不会自动重试")
		return
	}
	var data any
	if len(raw) > 0 {
		if checkedErr := json.Unmarshal(raw, &data); checkedErr != nil {
			fail(w, 503, "operation_failed", "读取或写入失败，请稍后重试")
			return
		}
	}

	write(w, 200, map[string]any{"result": data, "mediaModified": false})
}
func (a *API) pluginRuns(w http.ResponseWriter, r *http.Request) {
	if a.Deps.DB == nil {
		fail(w, 503, "plugins_unavailable", "记录不可用")
		return
	}
	rows, e := a.Deps.DB.Query(r.Context(), `SELECT kind,state,message,created_at FROM extension_runs WHERE plugin_id=$1 ORDER BY created_at DESC LIMIT 100`, r.PathValue("id"))
	if e != nil {
		fail(w, 503, "plugins_unavailable", "记录读取失败")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var kind, state, message string
		var created any
		if e = rows.Scan(&kind, &state, &message, &created); e != nil {
			fail(w, 503, "plugins_unavailable", "记录读取失败")
			return
		}
		items = append(items, map[string]any{"kind": kind, "state": state, "message": message, "createdAt": created})
	}
	if rows.Err() != nil {
		fail(w, 503, "plugins_unavailable", "记录读取失败")
		return
	}
	write(w, 200, items)
}
