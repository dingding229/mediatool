package httpapi

import (
	"media-tool/internal/modules/integrations"
	"net/http/httptest"
	"testing"
)

func TestI29RejectsInvalidIdentityFilter(t *testing.T) {
	a := API{Deps: Dependencies{Integrations: &integrations.Manager{}}}
	for _, v := range []string{"1", "yes", "TRUE"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/search/fixture?ignoreMediaIdentity="+v, nil)
		a.getSearch(w, r)
		if w.Code != 400 {
			t.Fatal(v, w.Code)
		}
	}
}
