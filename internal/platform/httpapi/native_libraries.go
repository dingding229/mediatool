package httpapi

import (
	"context"
	"media-tool/internal/modules/nativelibrary/catalog"
	"net/http"
	"strconv"
)

type NativeLibraryReader interface {
	List(context.Context, int, int) (catalog.Page, error)
}

func (a *API) listNativeLibraries(w http.ResponseWriter, r *http.Request) {
	start, limit := 0, 200
	for _, field := range []struct {
		name   string
		target *int
	}{{"startIndex", &start}, {"limit", &limit}} {
		values, ok := r.URL.Query()[field.name]
		if !ok {
			continue
		}
		if len(values) != 1 {
			fail(w, 400, "invalid_native_library_query", "页码或条数无效")
			return
		}
		n, err := strconv.Atoi(values[0])
		if err != nil {
			fail(w, 400, "invalid_native_library_query", "页码或条数无效")
			return
		}
		*field.target = n
	}
	if start < 0 || start > 1000000 || limit < 1 || limit > 200 {
		fail(w, 400, "invalid_native_library_query", "页码或条数无效")
		return
	}
	var reader NativeLibraryReader = a.Deps.NativeLibraries
	if reader == nil {
		reader = catalog.Repository{DB: a.Deps.DB}
	}
	page, err := reader.List(r.Context(), start, limit)
	if err != nil {
		fail(w, 503, "native_libraries_unavailable", "原生媒体库列表暂不可用")
		return
	}
	write(w, 200, page)
}
