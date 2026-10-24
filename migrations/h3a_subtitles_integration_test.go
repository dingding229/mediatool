package migrations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"media-tool/internal/modules/accounts"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativeplayback"
	"media-tool/internal/modules/subtitles"
	"media-tool/internal/platform/auth"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
	"media-tool/internal/platform/mediaext"
)

const h3Subtitle = "1\n00:00:01,000 --> 00:00:02,000\nH3a 测试字幕\n"

type h3Admin struct{}

func (h3Admin) Session(context.Context, []byte) (accounts.Account, error) {
	return accounts.Account{ID: "synthetic-admin", Username: "h3-admin", Version: 1}, nil
}

func TestH3aSubtitleUpDownUpStoreStableIndicesAndIsolation(t *testing.T) {
	f := newPlaybackFixture(t)
	// Exercise exactly migration 82 down/up, preserving business rows/credentials.
	sentinel := f.text(t, `SELECT credentials_ciphertext::text FROM integrations WHERE id=$1`, f.storage)
	for _, up := range []bool{false, true} {
		var e error
		if up {
			_, e = f.provider.UpTo(f.ctx, 82)
		} else {
			_, e = f.provider.Down(f.ctx)
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	if got := f.text(t, `SELECT credentials_ciphertext::text FROM integrations WHERE id=$1`, f.storage); got != sentinel {
		t.Fatal("migration touched credentials")
	}
	repo := subtitles.Repository{DB: f.pool, Root: filepath.Join(t.TempDir(), "subtitles")}
	f.service.Subtitles = repo
	id := jobs.UUID()
	uploaded, e := repo.UploadID(f.ctx, f.a.asset, id, "film.chs.default.srt", "auto", []byte(h3Subtitle))
	if e != nil {
		t.Fatal(e)
	}
	if uploaded.Index < 65536 || uploaded.Language != "chs" || !uploaded.Default || uploaded.Size != int64(len(h3Subtitle)) {
		t.Fatal(uploaded)
	}

	// Only the exact registered, digest-checked text copy can be served.
	filename := filepath.Join(repo.Root, uploaded.StoragePath)
	if e = os.WriteFile(filename, []byte("tampered"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, _, e = repo.Read(f.ctx, f.a.asset, id); !errors.Is(e, subtitles.ErrUnavailable) {
		t.Fatal("digest/size bypass", e)
	}
	if e = os.WriteFile(filename, []byte(h3Subtitle), 0600); e != nil {
		t.Fatal(e)
	}
	outside := filepath.Join(t.TempDir(), "outside.srt")
	if e = os.WriteFile(outside, []byte(h3Subtitle), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(filename); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(outside, filename); e != nil {
		t.Fatal(e)
	}
	if _, _, e = repo.Read(f.ctx, f.a.asset, id); e == nil {
		t.Fatal("escaped subtitle root")
	}
	if e = os.Remove(filename); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filename, []byte(h3Subtitle), 0600); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := repo.UploadID(f.ctx, f.a.asset, id, "film.chs.default.srt", "auto", []byte(h3Subtitle))
			if e != nil || v.ID != uploaded.ID || v.Index != uploaded.Index {
				t.Error(v, e)
			}
		}()
	}
	wg.Wait()
	if _, e := repo.UploadID(f.ctx, f.a.asset, id, "film.srt", "auto", []byte(h3Subtitle+"different")); !errors.Is(e, subtitles.ErrConflict) {
		t.Fatal(e)
	}
	if _, e := repo.UploadID(f.ctx, f.b.asset, id, "film.srt", "auto", []byte(h3Subtitle)); e == nil {
		t.Fatal("ID reused across assets")
	}
	if _, e := repo.Get(f.ctx, f.b.asset, id); !errors.Is(e, subtitles.ErrNotFound) {
		t.Fatal("cross-version row", e)
	}
	if _, _, e := repo.Read(f.ctx, f.b.asset, id); !errors.Is(e, subtitles.ErrNotFound) {
		t.Fatal("cross-version body", e)
	}
	video := f.text(t, `SELECT full_path FROM cloud_files WHERE id=$1`, f.a.file)
	stem := strings.TrimSuffix(video, path.Ext(video))
	cloud := jobs.UUID()
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)VALUES($1,$2,$3,1,'present')`, cloud, f.storage, stem+".eng.srt")
	f.probeSnapshot(t, f.a, 600000000, 1920)
	page, e := repo.List(f.ctx, f.a.asset)
	if e != nil {
		t.Fatal(e)
	}
	if len(page.Items) != 3 || len(page.Versions) != 2 {
		t.Fatal(page)
	}
	var automatic, embedded subtitles.Entry
	for _, v := range page.Items {
		switch v.Source {
		case "automatic":
			automatic = v
		case "embedded":
			embedded = v
		}
	}
	if automatic.SourceKey != cloud || embedded.Index != 5 {
		t.Fatal(page)
	}
	// Concurrent discovery never remaps existing identities or allocates on reads.
	seq := f.text(t, `SELECT last_value::text FROM managed_subtitle_index`)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := repo.Groups(f.ctx, []string{f.a.asset, f.b.asset}, mediaext.SubtitleExtensions()); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if got := f.text(t, `SELECT last_value::text FROM managed_subtitle_index`); got != seq {
		t.Fatal("read consumed indices", seq, got)
	}
	before := f.downloads.Load()
	m := automatic.Metadata
	m.Hidden = true
	hidden, e := repo.Save(f.ctx, f.a.asset, automatic.ID, m)
	if e != nil {
		t.Fatal(e)
	}
	if e := repo.Delete(f.ctx, f.a.asset, automatic.ID, hidden.Revision); !errors.Is(e, subtitles.ErrInvalid) {
		t.Fatal("automatic deletion allowed", e)
	}
	if f.downloads.Load() != before || f.text(t, `SELECT state FROM cloud_files WHERE id=$1`, cloud) != "present" {
		t.Fatal("hide touched provider")
	}
	m = embedded.Metadata
	m.Hidden = true
	if _, e := repo.Save(f.ctx, f.a.asset, embedded.ID, m); !errors.Is(e, subtitles.ErrInvalid) {
		t.Fatal("embedded hidden", e)
	}
	info := f.call("GET", "/Items/"+f.item+"/PlaybackInfo?MediaSourceId="+f.a.asset, f.token, "Infuse/8")
	if info.Code != 200 || strings.Contains(info.Body.String(), ".eng.srt") {
		t.Fatal("hidden stream", info.Code, info.Body.String())
	}
	// Adding a name sorted before the original must not renumber existing entries.
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)VALUES($1,$2,$3,1,'present')`, jobs.UUID(), f.storage, stem+".aaa.srt")
	page, e = repo.List(f.ctx, f.a.asset)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range page.Items {
		if v.ID == automatic.ID && v.Index != automatic.Index || v.ID == id && v.Index != uploaded.Index {
			t.Fatal("index shifted", v)
		}
	}
	m = hidden.Metadata
	m.Hidden = false
	if _, e = repo.Save(f.ctx, f.a.asset, automatic.ID, m); e != nil {
		t.Fatal(e)
	}
	if _, e = repo.Save(f.ctx, f.a.asset, automatic.ID, m); !errors.Is(e, subtitles.ErrConflict) {
		t.Fatal("revision fence", e)
	}
	// Failed disk cleanup retains an unservable tombstone, then explicit retry cleans it.
	copyPath := filepath.Join(repo.Root, uploaded.StoragePath)
	if e = os.Remove(copyPath); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(copyPath, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(copyPath, "blocked"), []byte("test"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = repo.Delete(f.ctx, f.a.asset, id, uploaded.Revision); !errors.Is(e, subtitles.ErrUnavailable) {
		t.Fatal(e)
	}
	pending, e := repo.Get(f.ctx, f.a.asset, id)
	if e != nil || !pending.Deleting {
		t.Fatal(pending, e)
	}
	if _, _, e = repo.Read(f.ctx, f.a.asset, id); !errors.Is(e, subtitles.ErrNotFound) {
		t.Fatal("tombstone served", e)
	}
	if e = os.RemoveAll(copyPath); e != nil {
		t.Fatal(e)
	}
	if e = repo.Delete(f.ctx, f.a.asset, id, uploaded.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = repo.Get(f.ctx, f.a.asset, id); !errors.Is(e, subtitles.ErrNotFound) {
		t.Fatal(e)
	}
	if f.downloads.Load() != before {
		t.Fatal("delete touched provider")
	}
}

func TestH3aUploadedPlaybackInfoGoldenAuthenticationAndOriginal302(t *testing.T) {
	f := newPlaybackFixture(t)
	repo := subtitles.Repository{DB: f.pool, Root: t.TempDir()}
	f.service.Subtitles = repo
	f.probeSnapshot(t, f.a, 600000000, 1920)
	uploaded, e := repo.Upload(f.ctx, f.a.asset, "film.chs.default.forced.srt", "auto", []byte(h3Subtitle))
	if e != nil {
		t.Fatal(e)
	}
	info := f.call("GET", "/Items/"+f.item+"/PlaybackInfo?MediaSourceId="+f.a.asset, f.token, "Infuse/8")
	var response dto.PlaybackInfoResponse
	if info.Code != 200 || json.Unmarshal(info.Body.Bytes(), &response) != nil || len(response.MediaSources) != 1 {
		t.Fatal(info.Code, info.Body.String())
	}
	source := response.MediaSources[0]
	var external dto.MediaStream
	for _, s := range source.MediaStreams {
		if s.IsExternal {
			external = s
		}
	}
	if external.Index == nil || *external.Index != uploaded.Index || external.Language != "chs" || external.DisplayTitle != "film.chs.default.forced.srt" || !external.IsDefault || !external.IsForced || external.DeliveryMethod != "External" {
		t.Fatal(external)
	}
	delivery, e := url.Parse(external.DeliveryUrl)
	if e != nil {
		t.Fatal(e)
	}
	projection := struct {
		Streams []dto.MediaStream `json:"MediaStreams"`
		Default *int              `json:"DefaultSubtitleStreamIndex"`
	}{source.MediaStreams, source.DefaultSubtitleStreamIndex}
	for i := range projection.Streams {
		s := &projection.Streams[i]
		if s.IsExternal {
			if !strings.Contains(s.DeliveryUrl, uploaded.ID) || !strings.Contains(s.DeliveryUrl, f.a.asset) || !strings.Contains(s.DeliveryUrl, "api_key=") {
				t.Fatal(s)
			}
			s.DeliveryUrl, e = nativeplayback.SubtitleURL("https://native.synthetic.invalid", "100001", "00000000-0000-4000-8000-000000000001", "synthetic-token", "00000000-0000-4000-8000-000000000002", uploaded.Index, "srt")
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	raw, e := json.MarshalIndent(projection, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	raw = append(raw, '\n')
	file := filepath.Join("testdata", "h3a-playbackinfo.json")
	if os.Getenv("UPDATE_H3A_GOLDEN") == "1" {
		if e = os.MkdirAll(filepath.Dir(file), 0755); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(file, raw, 0644); e != nil {
			t.Fatal(e)
		}
	}
	want, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(raw, want) {
		t.Fatalf("PlaybackInfo golden mismatch: %v\n%s", e, raw)
	}
	before := f.downloads.Load()
	for _, method := range []string{"GET", "HEAD"} {
		w := f.call(method, delivery.RequestURI(), "", "Infuse/8")
		if w.Code != 200 || w.Header().Get("Content-Type") != "application/x-subrip; charset=utf-8" || w.Header().Get("X-Content-Type-Options") != "nosniff" || method == "HEAD" && w.Body.Len() != 0 || method == "GET" && w.Body.String() != h3Subtitle {
			t.Fatal(method, w.Code, w.Body.String())
		}
	}
	if f.downloads.Load() != before {
		t.Fatal("upload contacted provider")
	}
	anon := *delivery
	q := anon.Query()
	q.Del("api_key")
	anon.RawQuery = q.Encode()
	if w := f.call("GET", anon.RequestURI(), "", "Infuse/8"); w.Code != 401 {
		t.Fatal("anonymous upload", w.Code)
	}
	if w := f.call("GET", strings.ReplaceAll(delivery.RequestURI(), f.a.asset, f.b.asset), f.token, "Infuse/8"); w.Code != 404 {
		t.Fatal("cross-version", w.Code, w.Body.String())
	}
	f.exec(t, `UPDATE player_ua_policy SET enabled=true,mode='blacklist'`)
	f.exec(t, `INSERT INTO player_ua_rules(id,label,match_type,user_agent,enabled,list_type)VALUES(gen_random_uuid(),'H3','exact','Blocked/1',true,'blacklist')`)
	if w := f.call("GET", delivery.RequestURI(), f.token, "Blocked/1"); w.Code != 403 {
		t.Fatal("UA bypass", w.Code)
	}
	m := uploaded.Metadata
	m.Hidden = true
	if _, e = repo.Save(f.ctx, f.a.asset, uploaded.ID, m); e != nil {
		t.Fatal(e)
	}
	if w := f.call("GET", delivery.RequestURI(), f.token, "Infuse/8"); w.Code != 404 {
		t.Fatal("hidden upload served", w.Code)
	}
	if e = repo.Delete(f.ctx, f.a.asset, uploaded.ID, uploaded.Revision+1); e != nil {
		t.Fatal(e)
	}
	if w := f.call("GET", delivery.RequestURI(), f.token, "Infuse/8"); w.Code != 404 {
		t.Fatal("deleted upload served", w.Code)
	}
	for _, method := range []string{"GET", "HEAD"} {
		w := f.call(method, "/Videos/"+f.item+"/stream.mkv?MediaSourceId="+f.a.asset, f.token, "Infuse/8")
		if w.Code != 302 || w.Body.Len() != 0 || !strings.HasPrefix(w.Header().Get("Location"), "https://cdn.synthetic.invalid/") {
			t.Fatal("video no longer original 302", method, w.Code, w.Body.String())
		}
	}
}

func TestH3aSubtitleAdminMultipartAuthMetadataAndDelete(t *testing.T) {
	f := newPlaybackFixture(t)
	base := t.TempDir()
	repo := subtitles.Repository{DB: f.pool, Root: filepath.Join(base, "subtitles")}
	c := config.Config{Environment: "development", SessionKey: strings.Repeat("s", 64), PublicURL: "https://admin.synthetic.invalid", NativeStillsRoot: filepath.Join(base, "stills")}
	h := httpapi.New(c, httpapi.Dependencies{DB: f.pool, AuthSessions: h3Admin{}})
	token, e := (auth.Auth{Key: c.SessionKey}).Issue(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	root := "/api/v1/media/assets/" + f.a.asset + "/subtitles"
	call := func(method, target string, body io.Reader, content string, authenticated, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, body)
		r.Header.Set("Content-Type", content)
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
	for _, route := range []struct{ method, path string }{{"GET", root}, {"POST", root}, {"PUT", root + "/" + jobs.UUID()}, {"DELETE", root + "/" + jobs.UUID()}, {"GET", root + "/" + jobs.UUID() + "/download"}} {
		if w := call(route.method, route.path, nil, "", false, true); w.Code != 401 {
			t.Fatal("anonymous", route, w.Code)
		}
	}
	if w := call("POST", root, nil, "", true, false); w.Code != 403 {
		t.Fatal("CSRF", w.Code)
	}
	id := jobs.UUID()
	upload := func(name string, body []byte, duplicate bool) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, e := writer.CreateFormFile("file", name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = part.Write(body); e != nil {
			t.Fatal(e)
		}
		if e = writer.WriteField("encoding", "auto"); e != nil {
			t.Fatal(e)
		}
		if e = writer.WriteField("uploadId", id); e != nil {
			t.Fatal(e)
		}
		if duplicate {
			if e = writer.WriteField("uploadId", id); e != nil {
				t.Fatal(e)
			}
		}
		if e = writer.Close(); e != nil {
			t.Fatal(e)
		}
		return call("POST", root, &buf, writer.FormDataContentType(), true, true)
	}
	if w := upload("bad.exe", []byte(h3Subtitle), false); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := upload("big.srt", bytes.Repeat([]byte("x"), subtitles.MaxBytes+1), false); w.Code != 413 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := upload("f.srt", []byte(h3Subtitle), true); w.Code != 400 {
		t.Fatal("duplicate part", w.Code)
	}
	w := upload("film.chs.srt", []byte(h3Subtitle), false)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = upload("film.chs.srt", []byte(h3Subtitle), false); w.Code != 201 {
		t.Fatal("idempotent HTTP upload", w.Code, w.Body.String())
	}
	v, e := repo.Get(f.ctx, f.a.asset, id)
	if e != nil {
		t.Fatal(e)
	}
	target := root + "/" + id
	if w = call("GET", root, nil, "", true, true); w.Code != 200 || !strings.Contains(w.Body.String(), id) || strings.Contains(w.Body.String(), repo.Root) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, method := range []string{"GET", "HEAD"} {
		w = call(method, target+"/download", nil, "", true, true)
		if w.Code != 200 || method == "GET" && w.Body.String() != h3Subtitle || method == "HEAD" && w.Body.Len() != 0 || w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w = call("PUT", target, strings.NewReader(`{"language":"jpn","title":"日本語字幕","default":true,"forced":true,"hidden":false,"revision":1}`), "application/json", true, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("DELETE", target, strings.NewReader(`{"revision":2,"confirm":false}`), "application/json", true, true); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = call("DELETE", target, strings.NewReader(`{"revision":2,"confirm":true}`), "application/json", true, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e = os.Stat(filepath.Join(repo.Root, v.StoragePath)); !os.IsNotExist(e) {
		t.Fatal("upload copy remains", e)
	}
	if w = call("GET", target+"/download", nil, "", true, true); w.Code != 404 {
		t.Fatal("deleted admin download", w.Code)
	}
	if f.downloads.Load() != 0 {
		t.Fatal("admin uploaded workflow contacted provider")
	}
}
