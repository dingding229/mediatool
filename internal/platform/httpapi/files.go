package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"media-tool/internal/modules/filemanager"
)

func (a *API) files() (*filemanager.Engine, error) {
	if a.Deps.Integrations == nil || a.Deps.DB == nil {
		return nil, filemanager.ErrUnavailable
	}
	return &filemanager.Engine{DB: a.Deps.DB, Connections: a.Deps.Integrations}, nil
}
func fileError(w http.ResponseWriter, err error) {
	status, code, message := 400, "invalid_file_operation", "文件操作参数无效，请核对目录、名称和模板"
	switch {
	case errors.Is(err, filemanager.ErrLimit):
		status, code, message = 413, "files_limit", "当前目录或变更超过安全上限，请缩小子目录范围"
	case errors.Is(err, filemanager.ErrMissing):
		status, code, message = 404, "file_not_found", "文件、目录或启用的连接不存在"
	case errors.Is(err, filemanager.ErrProtected):
		status, code, message = 409, "file_protected", "做种源、托管输出、根目录或符号链接受保护，不能直接修改"
	case errors.Is(err, filemanager.ErrBusy):
		status, code, message = 409, "file_busy", "扫描、传输或文件操作未结束，请先核对未完成的操作"
	case errors.Is(err, filemanager.ErrChanged):
		status, code, message = 409, "file_changed", "文件或连接已变化，请刷新并重新预览"
	case errors.Is(err, filemanager.ErrConflict):
		status, code, message = 409, "file_conflict", "目标文件或目录已存在，不会覆盖，请改名或重新选择目录"
	case errors.Is(err, filemanager.ErrUnavailable):
		status, code, message = 503, "files_unavailable", "文件服务不可用，请检查连接、目录权限和数据库"
	}
	fail(w, status, code, message)
}
func (a *API) listFileRoots(w http.ResponseWriter, r *http.Request) {
	e, err := a.files()
	if err != nil {
		fileError(w, err)
		return
	}
	items, err := e.Roots(r.Context())
	if err != nil {
		fileError(w, err)
		return
	}
	write(w, 200, items)
}
func (a *API) listFiles(w http.ResponseWriter, r *http.Request) {
	e, err := a.files()
	if err != nil {
		fileError(w, err)
		return
	}
	p, err := pagination(r)
	if err != nil {
		fileError(w, filemanager.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	items, err := e.List(ctx, r.URL.Query().Get("connectionId"), r.URL.Query().Get("path"), p.Limit, p.Offset)
	if err != nil {
		fileError(w, err)
		return
	}
	write(w, 200, items)
}
func (a *API) previewFileOperation(w http.ResponseWriter, r *http.Request) {
	e, err := a.files()
	if err != nil {
		fileError(w, err)
		return
	}
	var in filemanager.Input
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	plan, err := e.Preview(ctx, in)
	if err != nil {
		fileError(w, err)
		return
	}
	write(w, 200, plan)
}
func (a *API) executeFileOperation(w http.ResponseWriter, r *http.Request) {
	e, err := a.files()
	if err != nil {
		fileError(w, err)
		return
	}
	var in struct {
		Digest  string `json:"digest"`
		Confirm bool   `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	out, err := e.Execute(ctx, r.PathValue("id"), in.Digest, in.Confirm)
	if err != nil {
		fileError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) reconcileFileOperation(w http.ResponseWriter, r *http.Request) {
	e, err := a.files()
	if err != nil {
		fileError(w, err)
		return
	}
	var in struct{}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	out, err := e.Reconcile(ctx, r.PathValue("id"))
	if err != nil {
		fileError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *API) listFileOperations(w http.ResponseWriter, r *http.Request) {
	e, err := a.files()
	if err != nil {
		fileError(w, err)
		return
	}
	out, err := e.Operations(r.Context())
	if err != nil {
		fileError(w, err)
		return
	}
	write(w, 200, out)
}
