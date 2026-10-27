package httpapi

import (
	"encoding/json"
	"errors"
	"media-tool/internal/modules/metadataedit"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetadataInputRejectsOversizeWithChineseMessage(t *testing.T) {
	for _, body := range []string{`{"targetKey":"fixture","revision":0,"set":{"year":1799}}`, `{"targetKey":"fixture","revision":0,"set":{"title":"` + strings.Repeat("中", 501) + `"}}`, `{"targetKey":"fixture","revision":0,"set":{"premiereDate":"2025-02-29"}}`} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/", strings.NewReader(body))
		var p metadataedit.Patch
		if !decodeLimit(w, r, &p, 256<<10) {
			t.Fatal(w.Body.String())
		}
		if !errors.Is(p.Validate(), metadataedit.ErrInvalid) {
			t.Fatal("invalid input accepted")
		}
	}
}

func TestT3MetadataHTTPRevisionValidationAndAuthentication(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	_, e := s.DB.Exec(ctx, `INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES('aaaaaaaa-1000-4000-8000-000000000004','movie',934010,'HTTP base','2026','{"overview":"Base"}')`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.DB.Exec(ctx, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata) VALUES('aaaaaaaa-1000-4000-8000-000000000005','series',934011,'I1 pagination','{}'); INSERT INTO media_metadata_overrides(media_id,entity_type,season_number,episode_number,overrides) SELECT 'aaaaaaaa-1000-4000-8000-000000000005','episode',1,n,'{"title":"I1 retained"}'::jsonb FROM generate_series(1,65)n`)
	if e != nil {
		t.Fatal(e)
	}
	c := cfg()
	h := New(c, Dependencies{DB: s.DB, AuthSessions: testSessionReader{}})
	path := "/api/v1/media/library/managed/movie_aaaaaaaa-1000-4000-8000-000000000004/metadata"
	get := invoke(h, "GET", path, "", c.SessionKey)
	if get.Code != 200 {
		t.Fatal(get.Code, get.Body.String())
	}
	var result struct {
		Data metadataedit.Record `json:"data"`
	}
	if e = json.Unmarshal(get.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	p := metadataedit.Patch{TargetKey: result.Data.TargetKey, Revision: result.Data.Revision, Set: metadataedit.Values{"title": json.RawMessage(`"HTTP changed"`)}}
	raw, _ := json.Marshal(p)
	saved := invoke(h, "PATCH", path, string(raw), c.SessionKey)
	if saved.Code != 200 {
		t.Fatal(saved.Code, saved.Body.String())
	}
	conflict := invoke(h, "PATCH", path, string(raw), c.SessionKey)
	if conflict.Code != 409 {
		t.Fatal(conflict.Code, conflict.Body.String())
	}
	for _, set := range []string{`{"title":"` + strings.Repeat("中", 501) + `"}`, `{"year":2101}`, `{"premiereDate":"2025-02-29"}`, `{"countries":["TW"]}`} {
		body := `{"targetKey":"` + p.TargetKey + `","revision":1,"set":` + set + `}`
		w := invoke(h, "PATCH", path, body, c.SessionKey)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "元数据输入无效") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, size := range []string{"", "0", "20", "200", "bad", "10&unmatchedPageSize=25"} {
		if w := invoke(h, "GET", path+"?unmatchedPageSize="+size, "", c.SessionKey); w.Code != 400 {
			t.Fatal(size, w.Code, w.Body.String())
		}
	}
	for _, size := range []string{"10", "25", "50", "100"} {
		if w := invoke(h, "GET", path+"?unmatchedPageSize="+size, "", c.SessionKey); w.Code != 200 {
			t.Fatal(size, w.Code, w.Body.String())
		}
	}
	orphanPath := "/api/v1/media/library/managed/series_aaaaaaaa-1000-4000-8000-000000000005/metadata"
	for _, cse := range []struct {
		query        string
		count, first int
	}{{"", 50, 1}, {"?unmatchedPage=2&unmatchedPageSize=10", 10, 11}, {"?unmatchedPage=3&unmatchedPageSize=25", 15, 51}, {"?unmatchedPageSize=100", 65, 1}} {
		w := invoke(h, "GET", orphanPath+cse.query, "", c.SessionKey)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var got struct {
			Data metadataedit.Record `json:"data"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &got); e != nil {
			t.Fatal(e)
		}
		if len(got.Data.Unmatched) != cse.count || got.Data.Unmatched[0].Episode != cse.first || got.Data.UnmatchedTotal != 65 {
			t.Fatal(cse, got.Data.Unmatched)
		}
	}
	if w := invoke(h, "GET", path+"?unmatchedPage=0", "", c.SessionKey); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := invoke(h, "GET", path, "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
}
