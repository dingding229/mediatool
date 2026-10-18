package httpapi

import (
	"encoding/json"
	"media-tool/internal/modules/catalog"
	"net/http/httptest"
	"testing"
)

func TestCachedSourceHTTPPaginationAndClosedDTO(t *testing.T) {
	a := &API{}
	for _, query := range []string{"kind=discover.source", "kind=recommend.source&page=2&limit=200"} {
		w := httptest.NewRecorder()
		a.catalogSources(w, httptest.NewRequest("GET", "/?"+query, nil))
		if w.Code != 200 {
			t.Fatal(query, w.Code)
		}
		var response struct {
			Data catalog.SourcePage `json:"data"`
		}
		if json.Unmarshal(w.Body.Bytes(), &response) != nil {
			t.Fatal("invalid envelope")
		}
		page := response.Data
		if page.Items == nil || len(page.Items) != 0 {
			t.Fatal(w.Body.String())
		}
		if query == "kind=discover.source" && (page.Page != 1 || page.Size != 20) {
			t.Fatal("documented default", page)
		}
	}
	for _, query := range []string{"", "kind=other", "kind=discover.source&kind=recommend.source", "kind=discover.source&page=0", "kind=discover.source&page=1001", "kind=discover.source&limit=201", "kind=discover.source&limit=bad", "kind=discover.source&limit=20&limit=200"} {
		w := httptest.NewRecorder()
		a.catalogSources(w, httptest.NewRequest("GET", "/?"+query, nil))
		if w.Code != 400 {
			t.Fatal(query, w.Code)
		}
	}
}
