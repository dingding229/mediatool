package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"media-tool/internal/modules/integrations"
)

func TestI13AssetListCreationTimeContract(t *testing.T) {
	ctx, s := pluginHTTPFixture(t)
	const id = "13131313-0000-4000-8000-000000000003"
	const stamp = "2026-10-28T10:00:00Z"
	_, err := s.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider) VALUES('13131313-0000-4000-8000-000000000001','fixture','clouddrive2');
 INSERT INTO cloud_files(id,storage_id,full_path,size_bytes) VALUES('13131313-0000-4000-8000-000000000002','13131313-0000-4000-8000-000000000001','/synthetic/movie.mkv',1);
 INSERT INTO media_assets(id,cloud_file_id,created_at) VALUES('13131313-0000-4000-8000-000000000003','13131313-0000-4000-8000-000000000002','2026-10-28T10:00:00Z');`)
	if err != nil {
		t.Fatal(err)
	}
	a := &API{Deps: Dependencies{DB: s.DB, Integrations: &integrations.Manager{}}}
	for _, query := range []string{"", "?page=1&pageSize=50", "?page=1&pageSize=10&assetId=" + id} {
		t.Run(query, func(t *testing.T) {
			w := httptest.NewRecorder()
			a.listAssets(w, httptest.NewRequest("GET", "/api/v1/media/assets"+query, nil).WithContext(ctx))
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var envelope struct{ Data json.RawMessage }
			if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			var items []map[string]json.RawMessage
			if query == "" {
				err = json.Unmarshal(envelope.Data, &items)
			} else {
				var page struct {
					Items []map[string]json.RawMessage
					Total int
				}
				err = json.Unmarshal(envelope.Data, &page)
				if page.Total != 1 {
					t.Fatal("asset total changed", page.Total)
				}
				items = page.Items
			}
			if err != nil || len(items) != 1 {
				t.Fatal("asset envelope", err, len(items))
			}
			var got time.Time
			if err := json.Unmarshal(items[0]["createdAt"], &got); err != nil || got.UTC().Format(time.RFC3339) != stamp {
				t.Fatal("creation time must come from the stored column", got, err)
			}
			if _, ok := items[0]["updatedAt"]; ok {
				t.Fatal("asset table has no updated_at column")
			}
		})
	}
}
