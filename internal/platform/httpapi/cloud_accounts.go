package httpapi

import (
	"context"
	"net/http"
	"time"
)

func (a *API) cloudAccountInfo(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	out, c, err := a.Deps.Integrations.OfficialAccountInfo(ctx)
	if err != nil {
		fail(w, 503, "official_account_unavailable", "115官方账户信息不可用，请检查已启用的115授权；不通过CloudDrive2获取或兜底空间数据")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, map[string]any{"connectionId": c.ID, "connectionRevision": c.Revision, "info": out, "note": "空间/会员属于当前115 OAuth授权账号，与CloudDrive2挂载独立；未返回的空间或日期保持未知。"})
}
