package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/settings"
)

type nativeManagementInfo struct {
	settings.NativeManagement
	PublicURL     string `json:"publicUrl"`
	ListenPort    string `json:"listenPort"`
	AddressSource string `json:"addressSource"`
}

func (a *API) nativeServerSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var input struct {
			ServerName string `json:"serverName"`
			Enabled    *bool  `json:"enabled"`
			Revision   int64  `json:"revision"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Enabled == nil || mediausers.ManagementLabel(input.ServerName) != input.ServerName {
			fail(w, 400, "invalid_native_settings", "服务名称或启用状态无效")
			return
		}
		in := settings.NativeManagement{ServerName: input.ServerName, Enabled: *input.Enabled, Revision: input.Revision}
		out, err := settings.SaveNativeManagement(r.Context(), a.Deps.DB, in)
		if errors.Is(err, settings.ErrNativeInput) {
			fail(w, 400, "invalid_native_settings", "服务名称需为1–100个字符")
			return
		}
		if errors.Is(err, settings.ErrNativeConflict) {
			fail(w, 409, "native_settings_conflict", "设置已变化，请刷新后重试")
			return
		}
		if err != nil {
			fail(w, 503, "native_settings_unavailable", "内置服务设置暂不可用")
			return
		}
		write(w, 200, out)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if a.Deps.DB == nil {
		fail(w, 503, "native_settings_unavailable", "内置服务设置暂不可用")
		return
	}
	var out nativeManagementInfo
	var configured string
	err := a.Deps.DB.QueryRow(ctx, `SELECT n.server_name,n.enabled,n.revision,coalesce(a.value#>>'{access,memberUrl}','') FROM native_server_settings n LEFT JOIN application_settings a ON a.singleton WHERE n.singleton`).Scan(&out.ServerName, &out.Enabled, &out.Revision, &configured)
	if err != nil {
		fail(w, 503, "native_settings_unavailable", "内置服务设置暂不可用")
		return
	}
	_, out.ListenPort, err = net.SplitHostPort(a.Config.MemberAddress)
	if err != nil {
		out.ListenPort = ""
	}
	out.AddressSource = "未配置"
	// Do not infer the media origin from an administrator Host/forwarded header.
	// Blank means the operator must configure memberUrl/MEMBER_PUBLIC_URL first.
	for _, candidate := range []struct{ value, source string }{{configured, "后台访问设置"}, {a.Config.MemberPublicURL, "MEMBER_PUBLIC_URL"}} {
		if strings.TrimSpace(candidate.value) == "" {
			continue
		}
		if safe, ok := safeNativeAddress(candidate.value); ok {
			out.PublicURL = safe
			out.AddressSource = candidate.source
		}
		break
	}
	out.ServerName = mediausers.ManagementLabel(out.ServerName)
	write(w, 200, out)
}

// This is the same origin-only policy as the media listener. In particular,
// never reflect credentials, signing parameters, or a configured source path.
func safeNativeAddress(raw string) (string, bool) {
	raw = strings.TrimRight(raw, "/")
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 512 || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || raw != u.Scheme+"://"+u.Host {
		return "", false
	}
	return raw, true
}
