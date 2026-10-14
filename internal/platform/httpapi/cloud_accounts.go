package httpapi

import (
	"context"
	"net/http"
	"path"
	"strings"
	"time"
)

func (a *API) cloudAccountInfo(w http.ResponseWriter, r *http.Request) {
	if !a.integrationReady(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	c, err := a.Deps.Integrations.Repository.Default(ctx, "clouddrive2")
	if err != nil {
		fail(w, 409, "cloud_not_configured", "先在本页添加并启用CloudDrive2连接")
		return
	}
	root := c.String("cloudRoot")
	if root == "" && len(a.Config.STRMAllowedRoots) > 0 {
		root = a.Config.STRMAllowedRoots[0]
	}
	if !strings.HasPrefix(root, "/") || path.Clean(root) != root || root == "/" {
		fail(w, 409, "cloud_root_invalid", "CloudDrive2媒体根目录未设置")
		return
	}
	client, err := a.Deps.Integrations.CloudByID(ctx, c.Record.ID)
	if err != nil {
		fail(w, 503, "cloud_unavailable", "CloudDrive2暂不可用")
		return
	}
	out, err := client.AccountInfo(ctx, root)
	if err != nil {
		fail(w, 503, "cloud_unavailable", "CloudDrive2空间/会员信息暂不可用")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, map[string]any{"connectionId": c.Record.ID, "connectionRevision": c.Record.Revision, "info": out, "note": "数据对应此CloudDrive2挂载；115官方OAuth授权与挂载账号分别配置，不自动假定同一账号。未返回的日期/空间保持未知。"})
}
