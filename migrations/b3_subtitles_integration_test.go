package migrations_test

import (
	"encoding/json"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/jobs"
	"net/url"
	"path"
	"strings"
	"testing"
)

func TestB3ExternalSubtitlesAuthenticatedOfficial302AndUAGate(t *testing.T) {
	f := newPlaybackFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 74); e != nil {
		t.Fatal(e)
	}
	video := f.text(t, `SELECT full_path FROM cloud_files WHERE id=$1`, f.a.file)
	stem := strings.TrimSuffix(video, path.Ext(video))
	sub := jobs.UUID()
	f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)VALUES($1,$2,$3,1,'present')`, sub, f.storage, stem+".zh.forced.srt")
	// Wrong prefix, other directory and missing subtitles must not be associated.
	for _, name := range []string{stem + "0.srt", path.Dir(video) + "/other.srt", path.Dir(video) + "/else/" + path.Base(stem) + ".srt"} {
		f.exec(t, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes,state)VALUES($1,$2,$3,1,'present')`, jobs.UUID(), f.storage, name)
	}
	info := f.call("GET", "/Items/"+f.item+"/PlaybackInfo?MediaSourceId="+f.a.asset, f.token, "Infuse/8")
	var data dto.PlaybackInfoResponse
	if info.Code != 200 || json.Unmarshal(info.Body.Bytes(), &data) != nil {
		t.Fatal(info.Code, info.Body.String())
	}
	var delivery string
	count := 0
	for _, source := range data.MediaSources {
		for _, s := range source.MediaStreams {
			if s.IsExternal {
				count++
				delivery = s.DeliveryUrl
				if s.DeliveryMethod != "External" || !s.IsForced || s.Language != "zh" {
					t.Fatal(s)
				}
			}
		}
	}
	if count != 1 || delivery == "" {
		t.Fatal("external subtitle missing", info.Body.String())
	}
	u, e := url.Parse(delivery)
	if e != nil {
		t.Fatal(e)
	}
	for _, method := range []string{"GET", "HEAD"} {
		w := f.call(method, u.RequestURI(), f.token, "Infuse/8")
		if w.Code != 302 || w.Body.Len() != 0 || !strings.HasPrefix(w.Header().Get("Location"), "https://cdn.synthetic.invalid/") {
			t.Fatal(method, w.Code, w.Body.String())
		}
	}
	if _, ok := f.agents.Load("Infuse/8"); !ok {
		t.Fatal("real UA not passed to official resolver")
	}
	anon := *u
	query := anon.Query()
	query.Del("api_key")
	anon.RawQuery = query.Encode()
	if w := f.call("GET", anon.RequestURI(), "", "Infuse/8"); w.Code != 401 {
		t.Fatal("anonymous subtitle", w.Code)
	}
	if w := f.call("GET", strings.Replace(u.RequestURI(), "Stream.srt", "Stream.vtt", 1), f.token, "Infuse/8"); w.Code != 400 {
		t.Fatal("conversion accepted", w.Code)
	}
	f.exec(t, `UPDATE player_ua_policy SET enabled=true,mode='whitelist' WHERE id='main'`)
	f.exec(t, `INSERT INTO player_ua_rules(id,label,match_type,user_agent,enabled)VALUES(gen_random_uuid(),'B3','prefix','Infuse/',true)`)
	before := f.downloads.Load()
	if w := f.call("GET", u.RequestURI(), f.token, "OtherPlayer/1"); w.Code != 403 {
		t.Fatal("UA gate bypass", w.Code)
	}
	if before != f.downloads.Load() {
		t.Fatal("denied UA called provider")
	}
	f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=$1`, sub)
	if w := f.call("GET", u.RequestURI(), f.token, "Infuse/8"); w.Code != 404 {
		t.Fatal("stale subtitle exposed", w.Code)
	}
}
