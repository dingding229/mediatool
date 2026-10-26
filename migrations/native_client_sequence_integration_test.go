package migrations_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/nativelibrary/catalog"
	playbackruntime "media-tool/internal/modules/nativeplayback/runtime"
	"media-tool/internal/modules/storage"
	"media-tool/internal/modules/strm"
	"media-tool/internal/platform/httpapi"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var updateClientSequence = flag.Bool("update-native-sequences", false, "update synthetic client protocol replay goldens")

type sequenceFrame struct {
	Status   int    `json:"status"`
	Bytes    int    `json:"bytes"`
	Location string `json:"location,omitempty"`
	Body     any    `json:"body,omitempty"`
}

// These are synthetic protocol replays, NOT captures from proprietary apps.
// Infuse-Direct headers follow Firecore docs; the unquoted MediaBrowser variant
// follows Jellyfin's Swift SDK. Sources and coverage are recorded in the matrix.
func TestNativeClientSequenceInfuse(t *testing.T)    { runNativeClientSequence(t, false) }
func TestNativeClientSequenceSenPlayer(t *testing.T) { runNativeClientSequence(t, true) }
func runNativeClientSequence(t *testing.T, sen bool) {
	f := newReportFixture(t)
	f.exec(t, `UPDATE media_assets SET created_at='2026-01-03T00:00:00Z' WHERE id=$1`, f.other.asset)
	f.library(t, "g:sequence-tv", "tvshows", true, false)
	m := f.media(t, 3536100, "Synthetic Sequence Show", "2026", `{"officialRating":"G","genres":["Drama"]}`)
	f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, m)
	first := f.episode(t, m, "g:sequence-tv", 1, 1, 1)
	second := f.episode(t, m, "g:sequence-tv", 1, 2, 2)
	for {
		n, e := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if e != nil {
			t.Fatal(e)
		}
		if n < 500 {
			break
		}
	}
	show := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='series' AND internal_key='tmdb:3536100'`)
	ep1 := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='episode' AND internal_key='tmdb:3536100:s1e1'`)
	ep2 := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='episode' AND internal_key='tmdb:3536100:s1e2'`)
	f.probeSnapshot(t, first, 6000000000, 1920)
	c := f.service.Links.(*playbackruntime.Resolver).Config
	relative := f.text(t, `SELECT output_path FROM strm_entries WHERE asset_id=$1`, first.asset)
	cloud := f.text(t, `SELECT full_path FROM cloud_files WHERE id=$1`, first.file)
	stable, e := (strm.Builder{BaseURL: c.PublicURL, APIKey: c.STRMKey, AllowedRoots: c.STRMAllowedRoots}).Build(storage.FileRef{Path: cloud})
	if e != nil {
		t.Fatal(e)
	}
	if e = strm.WriteAtomic(f.root, relative, stable); e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256([]byte(stable + "\n"))
	f.exec(t, `INSERT INTO managed_media_artifacts(relative_path,asset_id,artifact_kind,owner_scope,content_digest)VALUES($1,$2::uuid,'strm','asset:'||($2::uuid)::text,$3)`, relative, first.asset, digest[:])
	// Use actual HTTP sockets. DirectStream URLs point to this server; no redirect
	// is followed to the fake CDN, and the 115 transport remains the T31 fake.
	server := httptest.NewUnstartedServer(nil)
	c.MemberPublicURL = "http://" + server.Listener.Addr().String()
	logger := slog.New(slog.NewJSONHandler(&f.logs, nil))
	f.handler, e = httpapi.NativeMediaHandler(c, httpapi.Dependencies{IPBlock: fixtureIPBlock(t), DB: f.pool, Integrations: f.manager, NativePlayback: f.service, Logger: logger})
	if e != nil {
		if closeErr := server.Listener.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal(e)
	}
	server.Config.Handler = f.handler
	server.Start()
	defer server.Close()

	client := &http.Client{Timeout: 10e9, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	name, ua, prefix, device := "infuse", "Infuse-Direct/8.0", "/emby", "infuse-sequence-device"
	if sen {
		name, ua, prefix, device = "senplayer", "SenPlayer/1.0", "/mediabrowser", "senplayer-sequence-device"
	}
	token := ""
	frames := map[string]sequenceFrame{}
	calls := 0
	call := func(phase, method, path, body string, status int) []byte {
		t.Helper()
		calls++
		target := server.URL + path
		if strings.HasPrefix(path, "http") {
			target = path
		}
		req, e := http.NewRequest(method, target, strings.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Content-Type", "application/json")
		if sen {
			req.Header.Set("Authorization", `mEdIaBrOwSeR  cLiEnT = SenPlayer , Device = Synthetic iPad , DeviceId = `+device+` , Version = 1.0`)
		} else {
			req.Header.Set("X-Emby-Authorization", `MediaBrowser Client="Infuse-Direct", Device="Synthetic iPad", DeviceId="`+device+`", Version="8.0"`)
		}
		if token != "" {
			switch calls % 3 {
			case 0:
				q := req.URL.Query()
				q.Set("ApiKey", token)
				req.URL.RawQuery = q.Encode()
			case 1:
				req.Header.Set("x-emby-token", token)
			case 2:
				if sen {
					req.Header.Set("Authorization", req.Header.Get("Authorization")+` , Token = `+token)
				} else {
					req.Header.Set("X-Emby-Authorization", req.Header.Get("X-Emby-Authorization")+`, Token="`+token+`"`)
				}
			}
		}
		resp, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		closeErr := resp.Body.Close()
		if readErr != nil || closeErr != nil {
			t.Fatal(readErr, closeErr)
		}
		// Scan ALL response bodies and header values, including login/error/redirect.
		check := func(value string) {
			for i := 0; i < 3; i++ {
				lower := strings.ToLower(value)
				for _, secret := range []string{"/115/", "pickcode", "pick_code", "abcdefgh3132", strm.Endpoint, c.STRMKey, stable} {
					if secret != "" && strings.Contains(lower, strings.ToLower(secret)) {
						t.Fatalf("D8 forbidden marker at phase %s", phase)
					}
				}
				decoded, e := url.QueryUnescape(value)
				if e != nil || decoded == value {
					break
				}
				value = decoded
			}
		}
		check(string(raw))
		for _, values := range resp.Header {
			for _, value := range values {
				check(value)
			}
		}
		if resp.StatusCode != status {
			t.Fatalf("phase=%s status=%d want=%d", phase, resp.StatusCode, status)
		}
		frame := sequenceFrame{Status: status, Bytes: len(raw), Location: resp.Header.Get("Location")}
		if len(raw) > 0 {
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			if e = dec.Decode(&frame.Body); e != nil {
				t.Fatal(phase, e)
			}
		}
		frames[phase] = frame
		return raw
	}
	call("public", "GET", "/System/Info/Public", "", 200)
	loginBody := `{"Username":"browse.member","Pw":"Browse123"}`
	if sen {
		loginBody = `{"username":"browse.member","Password":"Browse123"}`
	}
	raw := call("authenticate", "POST", prefix+"/Users/AuthenticateByName", loginBody, 200)
	var login dto.AuthenticationResult
	if e = json.Unmarshal(raw, &login); e != nil {
		t.Fatal(e)
	}
	token = login.AccessToken
	if token == "" {
		t.Fatal("no user token")
	}
	user := login.User.Id
	call("user", "GET", prefix+"/uSeRs/"+user, "", 200)
	call("views", "GET", prefix+"/Users/"+user+"/Views", "", 200)
	call("latest", "GET", prefix+"/Users/"+user+"/Items/Latest?Limit=20", "", 200)
	list := func(phase, path string) dto.QueryResult[dto.BaseItemDto] {
		t.Helper()
		raw := call(phase, "GET", path, "", 200)
		var p dto.QueryResult[dto.BaseItemDto]
		if e := json.Unmarshal(raw, &p); e != nil {
			t.Fatal(e)
		}
		return p
	}
	if p := list("resume-before", prefix+"/Users/"+user+"/Items/Resume?MediaTypes=Video"); p.TotalRecordCount != 0 {
		t.Fatal("initial resume")
	}
	if p := list("next-before", prefix+"/Shows/NextUp?UserId="+user); p.TotalRecordCount != 0 {
		t.Fatal("initial nextup")
	}
	list("items", prefix+"/Users/"+user+"/Items?ParentId="+f.parents["g:test-a"]+"&Limit=200&SortBy=SortName")
	seasons := list("seasons", prefix+"/Shows/"+show+"/Seasons?UserId="+user)
	if len(seasons.Items) != 1 || seasons.Items[0].UserData == nil || *seasons.Items[0].UserData.UnplayedItemCount != 2 {
		t.Fatal("season personalization")
	}
	list("episodes", prefix+"/Shows/"+show+"/Episodes?SeasonId="+seasons.Items[0].Id)
	call("series-detail", "GET", prefix+"/Items/"+show, "", 200)
	call("season-detail", "GET", prefix+"/Items/"+seasons.Items[0].Id, "", 200)
	call("counts", "GET", prefix+"/Items/Counts", "", 200)
	call("detail", "GET", prefix+"/Users/"+user+"/Items/"+ep1, "", 200)
	raw = call("playback-info", "POST", prefix+"/iTeMs/"+ep1+"/PlaybackInfo", `{"UserId":"`+user+`","MediaSourceId":"`+first.asset+`","DeviceProfile":{},"StartTimeTicks":0}`, 200)
	var playback dto.PlaybackInfoResponse
	if e = json.Unmarshal(raw, &playback); e != nil || len(playback.MediaSources) != 1 {
		t.Fatal(e)
	}
	source := playback.MediaSources[0]
	if !strings.HasPrefix(source.Path, server.URL) || source.DirectStreamUrl == "" || playback.PlaySessionId == "" {
		t.Fatal("native stream URL")
	}
	call("stream-head", "HEAD", source.Path, "", 302)
	call("stream-get", "GET", source.Path, "", 302)
	for _, phase := range []string{"stream-head", "stream-get"} {
		if frames[phase].Bytes != 0 || frames[phase].Location != "https://cdn.synthetic.invalid/original?signature=SYNTHETIC_VISIBLE" {
			t.Fatal("redirect contract", phase)
		}
	}
	payload := `"ItemId":"` + ep1 + `","MediaSourceId":"` + first.asset + `","PlaySessionId":"` + playback.PlaySessionId + `"`
	call("playing", "POST", prefix+"/Sessions/Playing", "{"+payload+"}", 204)
	call("progress", "POST", prefix+"/Sessions/Playing/Progress", "{"+payload+`,"PositionTicks":1200000000}`, 204)
	call("ping", "POST", prefix+"/Sessions/Playing/Ping?PlaySessionId="+playback.PlaySessionId, "", 204)
	raw = call("userdata-progress", "GET", prefix+"/Users/"+user+"/Items/"+ep1+"/UserData", "", 200)
	var data dto.UserItemDataDto
	if e = json.Unmarshal(raw, &data); e != nil || data.Played || *data.PlaybackPositionTicks != 1200000000 || *data.PlayedPercentage != 20 {
		t.Fatal("progress data", e, data)
	}
	if p := list("resume-progress", prefix+"/Users/"+user+"/Items/Resume?MediaTypes=Video"); p.TotalRecordCount != 1 || p.Items[0].Id != ep1 {
		t.Fatal("resume progress")
	}
	if p := list("next-progress", prefix+"/Shows/NextUp?UserId="+user+"&SeriesId="+show); p.TotalRecordCount != 1 || p.Items[0].Id != ep1 {
		t.Fatal("next progress")
	}
	call("stopped", "POST", prefix+"/Sessions/Playing/Stopped", "{"+payload+`,"PositionTicks":5800000000}`, 204)
	raw = call("userdata-stopped", "GET", prefix+"/Users/"+user+"/Items/"+ep1+"/UserData", "", 200)
	if e = json.Unmarshal(raw, &data); e != nil || !data.Played || *data.PlayCount != 1 || *data.PlaybackPositionTicks != 0 {
		t.Fatal("stopped data", e, data)
	}
	if p := list("resume-stopped", prefix+"/Users/"+user+"/Items/Resume"); p.TotalRecordCount != 0 {
		t.Fatal("completed resume")
	}
	if p := list("next-stopped", prefix+"/Shows/NextUp?UserId="+user); p.TotalRecordCount != 1 || p.Items[0].Id != ep2 {
		t.Fatal("next did not advance")
	}
	call("favorite-add", "POST", prefix+"/Users/"+user+"/FavoriteItems/"+f.item, "", 200)
	call("favorite-delete", "DELETE", prefix+"/Users/"+user+"/FavoriteItems/"+f.item, "", 200)
	call("userdata-update", "POST", prefix+"/Users/"+user+"/Items/"+f.item+"/UserData", `{"PlaybackPositionTicks":1200000000}`, 200)
	call("played-add", "POST", prefix+"/Users/"+user+"/PlayedItems/"+f.item+"?DatePlayed=2026-10-16T00:00:00Z", "", 200)
	call("played-delete", "DELETE", prefix+"/Users/"+user+"/PlayedItems/"+f.item, "", 200)
	if sen {
		legacy := prefix + "/Users/" + user + "/PlayingItems/" + ep1
		call("legacy-playing", "POST", legacy+"?MediaSourceId="+first.asset, "", 204)
		call("legacy-progress", "POST", legacy+"/Progress?PositionTicks=1200000000&IsPaused=false", `{"EventName":"Unpause"}`, 204)
		call("legacy-stopped", "DELETE", legacy+"?PositionTicks=5800000000", "", 204)
	}
	call("cross-user", "GET", prefix+"/Users/aaaaaaaa000040008000000000000036/Items/"+ep1+"/UserData", "", 403)
	if _, ok := f.agents.Load(ua); !ok {
		t.Fatal("player UA was not used for provider link")
	}
	// Deterministic golden normalization: IDs, local port, user token and volatile
	// activity dates only. Payload fields, counts, types and arrays are untouched.
	replacements := map[string]string{server.URL: "http://native.synthetic.invalid", token: "synthetic-user-token", user: strings.Repeat("a", 32), login.ServerId: strings.Repeat("b", 32), login.SessionInfo.Id: "cccccccc-0000-4000-8000-000000000036", playback.PlaySessionId: "dddddddd-0000-4000-8000-000000000036", first.asset: "eeeeeeee-0000-4000-8000-000000000036", second.asset: "ffffffff-0000-4000-8000-000000000036", f.a.asset: "eeeeeeee-0000-4000-8000-000000000031", f.b.asset: "eeeeeeee-0000-4000-8000-000000000032", f.other.asset: "eeeeeeee-0000-4000-8000-000000000033"}
	rows, e := f.pool.Query(f.ctx, `SELECT id::text FROM native_item_ids ORDER BY entity_type,internal_key`)
	if e != nil {
		t.Fatal(e)
	}
	i := 0
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			t.Fatal(e)
		}
		i++
		replacements[id] = strconv.Itoa(100000 + i)
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	rows.Close()
	var normalize func(any, string) any
	normalize = func(v any, key string) any {
		switch x := v.(type) {
		case string:
			if key == "LastPlayedDate" || key == "LastActivityDate" || key == "LastLoginDate" {
				return "2026-10-17T00:00:00.0000000Z"
			}
			if y, ok := replacements[x]; ok {
				return y
			}
			for _, from := range []string{server.URL, token, first.asset, second.asset, f.a.asset, f.b.asset, f.other.asset} {
				x = strings.ReplaceAll(x, from, replacements[from])
			}
			if strings.HasPrefix(x, "http://") || strings.HasPrefix(x, "https://") {
				u, e := url.Parse(x)
				if e != nil {
					t.Fatal(e)
				}
				parts := strings.Split(u.Path, "/")
				for i, part := range parts {
					if replacement, ok := replacements[part]; ok {
						parts[i] = replacement
					}
				}
				u.Path = strings.Join(parts, "/")
				x = u.String()
			}
			return x
		case map[string]any:
			for k, y := range x {
				x[k] = normalize(y, k)
			}
			return x
		case []any:
			for i, y := range x {
				x[i] = normalize(y, key)
			}
			return x
		}
		return v
	}
	for k, frame := range frames {
		frame.Body = normalize(frame.Body, "")
		frame.Bytes = 0
		if frame.Body != nil {
			b, e := json.Marshal(frame.Body)
			if e != nil {
				t.Fatal(e)
			}
			frame.Bytes = len(b)
		}
		frames[k] = frame
	}
	out, e := json.MarshalIndent(frames, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	out = append(out, '\n')
	file := filepath.Join("..", "internal", "modules", "embyapi", "testdata", "sequences", name+".json")
	if *updateClientSequence {
		if e = os.MkdirAll(filepath.Dir(file), 0755); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(file, out, 0600); e != nil {
			t.Fatal(e)
		}
	}
	want, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(want, out) {
		if writeErr := os.WriteFile("/private/tmp/media-tool-t3536-"+name+"-actual.json", out, 0600); writeErr != nil {
			t.Fatal(writeErr)
		}
		t.Fatal("client sequence golden mismatch", name, e)
	}
	t.Logf("T36 %s: %d real HTTP requests; all bodies/headers D8-scanned; progress->resume->completion->nextup verified", name, calls)
}
