package migrations_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	managed "media-tool/internal/modules/catalog"
	"media-tool/internal/modules/embyapi"
	"media-tool/internal/modules/imageedit"
	"media-tool/internal/modules/metadataedit"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func t4Image(t *testing.T) imageedit.Normalized {
	t.Helper()
	var b bytes.Buffer
	pic := image.NewNRGBA(image.Rect(0, 0, 20, 30))
	pic.Set(3, 4, color.NRGBA{255, 0, 0, 128})
	if e := png.Encode(&b, pic); e != nil {
		t.Fatal(e)
	}
	out, e := imageedit.Normalize(t.Context(), "poster.png", b.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestT4MigrationRoundTrip(t *testing.T) {
	f := newNativeFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 84); e != nil {
		t.Fatal(e)
	}
	if _, e := f.provider.Down(f.ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := f.provider.UpTo(f.ctx, 84); e != nil {
		t.Fatal(e)
	}
}
func TestT4ImagesLockRestoreTagsPermissionAndCollection(t *testing.T) {
	f := newBrowseFixture(t)
	m := f.media(t, 935001, "Image fixture", "2026", `{"officialRating":"G","posterPath":"/scraped.jpg"}`)
	asset := f.asset(t, m, "g:test-a", time.Now(), "mkv")
	root := filepath.Join(t.TempDir(), "native-media/images")
	repo := imageedit.Repository{DB: f.pool, Root: root}
	pic := t4Image(t)
	r, e := repo.Read(f.ctx, asset.asset)
	if e != nil {
		t.Fatal(e)
	}
	if r.Revision != 0 || r.Overrides == nil || !r.Matched {
		t.Fatal(r)
	}
	original := r
	change := imageedit.Change{TargetKey: r.TargetKey, Revision: r.Revision, Kind: "Primary"}
	r, e = repo.Save(f.ctx, asset.asset, "fixture-admin", change, &pic, "upload")
	if e != nil {
		t.Fatal(e)
	}
	fid := r.Overrides["Primary"]
	if fid == "" || r.Revision != 1 || r.Effective["Primary"].Tag == "" {
		t.Fatal(r)
	}
	body, ct, tag, e := repo.File(f.ctx, fid)
	if e != nil || ct != "image/png" || !bytes.Equal(body, pic.Body) {
		t.Fatal(ct, e)
	}
	items, n := f.query(t, nativelibrary.ItemQuery{IncludeItemTypes: []string{"Movie"}, Recursive: boolPtr(true), Limit: 10})
	if n != 1 || len(items) != 1 || items[0].ImageTags["Primary"] != tag {
		t.Fatal(items, n)
	}
	nativeID := items[0].Id
	resolver := httpapi.NewNativeImageResolver(f.pool, nil, nil)
	resolver.ImageRoot = root
	perms := f.permissions()
	request := embyapi.ImageRequest{ItemID: nativeID, Type: "Primary", User: &f.user, Permissions: perms}
	got, e := resolver.ResolveImage(f.ctx, request)
	if e != nil || got.Tag != tag || !bytes.Equal(got.Body, pic.Body) {
		t.Fatal(got.Tag, e)
	}
	request.User = nil
	if _, e = resolver.ResolveImage(f.ctx, request); !errors.Is(e, embyapi.ErrImageNotFound) {
		t.Fatal(e)
	}
	page, e := (managed.Engine{DB: f.pool}).ListClassified(f.ctx, "Image fixture", "", "", "", 1, 24)
	if e != nil || len(page.Items) != 1 || page.Items[0].Poster != r.Effective["Primary"].URL && page.Items[0].Poster != imageedit.URL(asset.asset, "Primary", tag) {
		t.Fatal(page, e)
	}
	detail, e := (managed.Engine{DB: f.pool}).ManagedDetail(f.ctx, asset.asset)
	if e != nil || detail.Poster != page.Items[0].Poster {
		t.Fatal(detail, e)
	}
	// Scrape/scan writers only change their base; image and text revisions are independent.
	f.exec(t, `UPDATE media_items SET metadata=metadata||'{"posterPath":"/new-scrape.jpg"}'::jsonb WHERE id=$1`, m)
	text, e := (metadataedit.Repository{DB: f.pool}).Read(f.ctx, asset.asset)
	if e != nil || text.Revision != 0 {
		t.Fatal(text, e)
	}
	r, e = repo.Read(f.ctx, asset.asset)
	if e != nil || r.Effective["Primary"].Tag != tag {
		t.Fatal(r, e)
	}
	if _, e = repo.Save(f.ctx, asset.asset, "fixture-admin", change, &pic, "upload"); !errors.Is(e, metadataedit.ErrConflict) {
		t.Fatal(e)
	}
	files, e := os.ReadDir(root)
	if e != nil || len(files) != 1 {
		t.Fatal(files, e)
	}
	// Two simultaneous restores at the same revision cannot overwrite each other.
	change.Revision = r.Revision
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := repo.Save(f.ctx, asset.asset, "fixture-admin", change, nil, "")
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for e := range results {
		if e == nil {
			ok++
		} else if errors.Is(e, metadataedit.ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal(ok, conflict)
	}
	r, e = repo.Read(f.ctx, asset.asset)
	if e != nil || len(r.Overrides) != 0 || r.Revision != 2 || reflect.DeepEqual(r.Base, original.Base) {
		t.Fatal(r, e)
	}
	files, e = os.ReadDir(root)
	if e != nil || len(files) != 0 {
		t.Fatal("orphan image", files, e)
	}
	items, _ = f.query(t, nativelibrary.ItemQuery{IncludeItemTypes: []string{"Movie"}, Recursive: boolPtr(true), Limit: 10})
	if items[0].ImageTags["Primary"] == tag {
		t.Fatal("restored tag did not change")
	}
	if count := f.text(t, `SELECT count(*)::text FROM audit_logs WHERE action='media.images.save'`); count != "2" {
		t.Fatal(count)
	}
}
func TestT4NumberedScopeUnmatchedAndParentImages(t *testing.T) {
	f := newBrowseFixture(t)
	show := f.media(t, 935002, "Series fixture", "2026", `{"officialRating":"G"}`)
	f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, show)
	f.library(t, "g:t4-tv", "tvshows", true, false)
	f.episode(t, show, "g:t4-tv", 1, 2, 2)
	repo := imageedit.Repository{DB: f.pool, Root: filepath.Join(t.TempDir(), "native-media/images")}
	pic := t4Image(t)
	for _, id := range []string{"series_" + show, "season_" + show + "_1", "episode_" + show + "_1_2"} {
		r, e := repo.Read(f.ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		for _, kind := range []string{"Primary", "Backdrop", "Logo"} {
			r, e = repo.Save(f.ctx, id, "fixture-admin", imageedit.Change{TargetKey: r.TargetKey, Revision: r.Revision, Kind: kind}, &pic, "upload")
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	items, _ := f.query(t, nativelibrary.ItemQuery{Recursive: boolPtr(true), Limit: 20})
	for _, item := range items {
		if item.Type == "CollectionFolder" {
			continue
		}
		if item.ImageTags["Primary"] == "" || item.ImageTags["Logo"] == "" || len(item.BackdropImageTags) != 1 {
			raw, _ := json.Marshal(item)
			t.Fatal(string(raw))
		}
		if item.Type == "Episode" || item.Type == "Season" {
			if item.SeriesPrimaryImageTag == "" || item.ParentLogoImageTag == "" {
				t.Fatal("parent tags", item)
			}
		}
	}
	f.exec(t, `UPDATE media_asset_bindings SET season=2,episode=3,episodes='[3]' WHERE media_id=$1`, show)
	r, e := repo.Read(f.ctx, "episode_"+show+"_1_2")
	if e != nil || r.Matched || len(r.Overrides) != 3 || len(r.Unmatched) != 2 {
		t.Fatal(r, e)
	}
}

func TestT4ImageHTTPMultipartAuthenticationAndCache(t *testing.T) {
	f := newBrowseFixture(t)
	m := f.media(t, 935003, "HTTP image fixture", "2026", `{"officialRating":"G","posterPath":"/base.jpg"}`)
	asset := f.asset(t, m, "g:test-a", time.Now(), "mkv")
	root := filepath.Join(t.TempDir(), "native-media/images")
	c := config.Config{Environment: "development", SessionKey: strings.Repeat("s", 64), PublicURL: "https://admin.synthetic.invalid"}
	h := httpapi.New(c, httpapi.Dependencies{DB: f.pool, AuthSessions: h3Admin{}, NativeImageRoot: root})
	token, e := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	path := "/api/v1/media/library/managed/" + asset.asset + "/images"
	call := func(method, target string, body io.Reader, ct string, authenticated, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, body)
		r.Header.Set("Content-Type", ct)
		if authenticated {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if csrf {
			r.Header.Set("X-Requested-With", "media-tool")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, op := range []struct{ method, path string }{{"GET", path}, {"POST", path}, {"PATCH", path}, {"GET", path + "/candidates"}, {"GET", path + "/Primary"}} {
		if w := call(op.method, op.path, nil, "", false, true); w.Code != 401 {
			t.Fatal(op, w.Code)
		}
	}
	if w := call("POST", path, nil, "", true, false); w.Code != 403 {
		t.Fatal(w.Code)
	}
	repo := imageedit.Repository{DB: f.pool, Root: root}
	record, e := repo.Read(f.ctx, asset.asset)
	if e != nil {
		t.Fatal(e)
	}
	pic := t4Image(t)
	upload := func(name string, raw []byte) *httptest.ResponseRecorder {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		for k, v := range map[string]string{"targetKey": record.TargetKey, "revision": "0", "kind": "Primary"} {
			if e := mw.WriteField(k, v); e != nil {
				t.Fatal(e)
			}
		}
		part, e := mw.CreateFormFile("file", name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = part.Write(raw); e != nil {
			t.Fatal(e)
		}
		if e = mw.Close(); e != nil {
			t.Fatal(e)
		}
		return call("POST", path, &b, mw.FormDataContentType(), true, true)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
	}{{"fake.jpg", pic.Body}, {"text.png", []byte("not an image")}, {"big.png", make([]byte, imageedit.MaxBytes+1)}} {
		if w := upload(tc.name, tc.raw); w.Code != 400 {
			t.Fatal(tc.name, w.Code, w.Body.String())
		}
	}
	w := upload("poster.png", pic.Body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var reply struct {
		Data imageedit.Record `json:"data"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &reply); e != nil {
		t.Fatal(e)
	}
	tag := reply.Data.Effective["Primary"].Tag
	w = call("GET", path+"/Primary?tag="+tag, nil, "", true, false)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), pic.Body) || w.Header().Get("Cache-Control") != "private, max-age=0, must-revalidate" {
		t.Fatal(w.Code, w.Body.String())
	}
	req := httptest.NewRequest("GET", path+"/Primary", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	req.Header.Set("If-None-Match", w.Header().Get("ETag"))
	notModified := httptest.NewRecorder()
	h.ServeHTTP(notModified, req)
	if notModified.Code != 304 || notModified.Body.Len() != 0 {
		t.Fatal(notModified.Code)
	}
	patch, _ := json.Marshal(imageedit.Change{TargetKey: record.TargetKey, Revision: reply.Data.Revision, Kind: "Primary"})
	w = call("PATCH", path, bytes.NewReader(patch), "application/json", true, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call("GET", path+"/Primary", nil, "", true, false)
	if w.Code != 302 || !strings.HasPrefix(w.Header().Get("Location"), "/api/v1/media/tmdb/image?") {
		t.Fatal(w.Code)
	}
	files, e := os.ReadDir(root)
	if e != nil || len(files) != 0 {
		t.Fatal(files, e)
	}
}
