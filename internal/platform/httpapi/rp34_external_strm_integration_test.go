package httpapi

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"media-tool/internal/modules/externalapi"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
)

func TestRP34ExternalSTRMNeverProjectsOrReadsNFO(t *testing.T) {
	ctx, store := pluginHTTPFixture(t)
	c := cfg()
	c.STRMOutputDir = t.TempDir()
	signer, err := externalapi.NewSigner(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	a := &API{Config: c, Deps: Dependencies{DB: store.DB, Integrations: &integrations.Manager{Config: c}, ExternalSigner: signer}}
	key, err := a.apiKeys().Create(ctx, "fixture", []string{"library.read"})
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := store.DB.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	connection, asset, onlyNFO := jobs.UUID(), jobs.UUID(), jobs.UUID()
	exec(`INSERT INTO storage_connections(id,name,provider) VALUES($1,'fixture','clouddrive2')`, connection)
	for _, id := range []string{asset, onlyNFO} {
		exec(`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes) VALUES($1,$2,$3,1)`, id, connection, "/fixture/"+id)
		exec(`INSERT INTO media_assets(id,cloud_file_id,scrape_report) VALUES($1,$1,'{"nfo":"report-private.nfo"}')`, id)
	}
	exec(`INSERT INTO strm_entries(id,asset_id,output_path,key_reference) VALUES($1,$2,'owned.strm','fixture-only')`, jobs.UUID(), asset)
	for _, file := range []struct{ id, path, kind, body string }{
		{asset, "owned.strm", "strm", "https://owned.example/original\n"},
		{asset, "private.nfo", "nfo", "<private>fixture</private>"},
		{onlyNFO, "extra.nfo", "nfo", "NFO-only fixture"},
	} {
		sum := sha256.Sum256([]byte(file.body))
		exec(`INSERT INTO managed_media_artifacts(asset_id,relative_path,artifact_kind,owner_scope,content_digest) VALUES($1,$2,$3,$4,$5)`, file.id, file.path, file.kind, "asset:"+file.id, sum[:])
		if err = os.WriteFile(filepath.Join(c.STRMOutputDir, file.path), []byte(file.body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	a.registerExternal(mux)
	get := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+key.Token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, query := range []string{"", "?kind=nfo", "?kind=strm&kind=nfo"} {
		w := get("GET", "/api/external/v1/strm"+query)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "owned.strm") || strings.Contains(w.Body.String(), ".nfo") || strings.Contains(w.Body.String(), onlyNFO) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	base := "/api/external/v1/strm/" + asset + "/content?path="
	if w := get("GET", base+"owned.strm"); w.Code != 200 || !strings.Contains(w.Body.String(), "https://owned.example/original") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, method := range []string{"GET", "HEAD"} {
		if w := get(method, base+url.QueryEscape("private.nfo")+"&kind=nfo"); w.Code != 409 || strings.Contains(w.Body.String(), "<private>") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "/admin?path=private.nfo", nil).WithContext(ctx)
	r.SetPathValue("id", asset)
	w := httptest.NewRecorder()
	a.managedOutputContent(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `\u003cprivate\u003efixture`) {
		t.Fatal("admin NFO behavior changed", w.Code, w.Body.String())
	}
}
