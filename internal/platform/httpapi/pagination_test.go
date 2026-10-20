package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestSharedPaginationRejectsAmbiguityAcrossProfiles(t *testing.T) {
	for _, raw := range []string{"page=", "page=2&page=3", "pageSize=", "pageSize=1&pageSize=2", "page=-1", "page=no", "page=%ZZ"} {
		r := httptest.NewRequest("GET", "/?"+raw, nil)
		if _, e := pagination(r); e == nil {
			t.Fatal("general accepted", raw)
		}
		if _, _, e := pluginPage(r); e == nil {
			t.Fatal("plugin accepted", raw)
		}
	}
	for _, raw := range []string{"startIndex=", "startIndex=1&startIndex=2", "limit=", "limit=1&limit=2", "limit=201", "startIndex=%ZZ"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/?"+raw, nil)
		if _, _, ok := managementPagination(w, r); ok || w.Code != 400 {
			t.Fatal(raw, w.Code)
		}
	}
	if p, e := pagination(httptest.NewRequest("GET", "/", nil)); e != nil || p.Page != 1 || p.Limit != 50 {
		t.Fatal(p, e)
	}
	if _, _, e := pluginPage(httptest.NewRequest("GET", "/?pageSize=200", nil)); e != nil {
		t.Fatal(e)
	}
	if _, e := pagination(httptest.NewRequest("GET", "/?pageSize=200", nil)); e == nil {
		t.Fatal("general bound changed")
	}
}
