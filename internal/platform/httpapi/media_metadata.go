package httpapi

import (
	"context"
	"errors"
	"media-tool/internal/modules/metadataedit"
	"net/http"
	"strconv"
	"time"
)

func (a *API) mediaMetadata(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	repo := metadataedit.Repository{DB: a.Deps.DB}
	var out metadataedit.Record
	var err error
	if r.Method == http.MethodPatch {
		var p metadataedit.Patch
		if !decodeLimit(w, r, &p, 256<<10) {
			return
		}
		account, e := a.sessionAccount(r)
		if e != nil {
			fail(w, 401, "unauthorized", "请重新登录")
			return
		}
		out, err = repo.Save(ctx, r.PathValue("id"), account.ID, p)
	} else {
		page := 1
		if text := r.URL.Query().Get("unmatchedPage"); text != "" {
			page, err = strconv.Atoi(text)
			if err != nil {
				fail(w, 400, "invalid_metadata_page", "未匹配记录页码无效")
				return
			}
		}
		size, sizeErr := paginationSize(r, "unmatchedPageSize", 50)
		if sizeErr != nil {
			fail(w, 400, "invalid_metadata_page_size", "未匹配记录条数无效")
			return
		}
		out, err = repo.ReadPage(ctx, r.PathValue("id"), page, size)
	}
	switch {
	case errors.Is(err, metadataedit.ErrInvalid):
		fail(w, 400, "invalid_metadata", err.Error())
	case errors.Is(err, metadataedit.ErrConflict):
		fail(w, 409, "metadata_conflict", err.Error())
	case errors.Is(err, metadataedit.ErrNotFound):
		fail(w, 404, "metadata_not_found", err.Error())
	case err != nil:
		fail(w, 503, "metadata_unavailable", "元数据操作暂不可用，请重新读取后核对")
	default:
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, out)
	}
}
