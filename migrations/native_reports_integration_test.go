package migrations_test

import (
	"encoding/json"
	"fmt"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediausers"
	"media-tool/internal/modules/nativelibrary/catalog"
	playbackruntime "media-tool/internal/modules/nativeplayback/runtime"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func newReportFixture(t *testing.T) *playbackFixture {
	t.Helper()
	f := newPlaybackFixture(t)
	if _, err := f.provider.UpTo(f.ctx, 61); err != nil {
		t.Fatal(err)
	}
	return f
}
func reportHTTP(f *playbackFixture, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Emby-Token", f.token)
	r.Header.Set("User-Agent", "Infuse/8")
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}
func reportInfo(t *testing.T, f *playbackFixture) string {
	t.Helper()
	w := f.call("GET", "/Items/"+f.item+"/PlaybackInfo?MediaSourceId="+f.a.asset, f.token, "Infuse/8")
	var v dto.PlaybackInfoResponse
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || w.Code != 200 {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	return v.PlaySessionId
}
func mustReport(t *testing.T, f *playbackFixture, path, body string) {
	t.Helper()
	w := reportHTTP(f, "POST", path, body)
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestNativeReportsFullSequenceThrottleThresholds(t *testing.T) {
	f := newReportFixture(t)
	f.probeSnapshot(t, f.a, 6000000000, 1920)
	id := reportInfo(t, f)
	if w := f.call("GET", "/Videos/"+f.item+"/stream.mkv?MediaSourceId="+f.a.asset, f.token, "Infuse/8"); w.Code != 302 {
		t.Fatal(w.Code, w.Body.String())
	}
	base := `"ItemId":"` + f.item + `","MediaSourceId":"` + f.a.asset + `","PlaySessionId":"` + id + `"`
	mustReport(t, f, "/Sessions/Playing", "{"+base+`}`)
	if n := f.integer(t, `SELECT count(*) FROM native_playback_sessions WHERE state='playing'`); n != 1 {
		t.Fatal(n)
	}
	f.exec(t, `UPDATE native_playback_sessions SET started_at=now()-interval '1 minute',last_progress_at=now()-interval '11 seconds' WHERE id=$1`, id)
	mustReport(t, f, "/Sessions/Playing/Progress", "{"+base+`,"PositionTicks":240000000}`)
	if f.integer(t, `SELECT position_ticks FROM media_user_item_data WHERE item_id=$1::bigint`, f.item) != 0 {
		t.Fatal("under 5 percent saved resume")
	}
	mustReport(t, f, "/Sessions/Playing/Progress", "{"+base+`,"PositionTicks":1200000000}`)
	before := f.text(t, `SELECT last_progress_at::text FROM native_playback_sessions WHERE id=$1`, id)
	mustReport(t, f, "/Sessions/Playing/Progress", "{"+base+`,"PositionTicks":1210000000}`)
	if f.text(t, `SELECT last_progress_at::text FROM native_playback_sessions WHERE id=$1`, id) != before {
		t.Fatal("throttle wrote DB")
	}
	if f.integer(t, `SELECT position_ticks FROM media_user_item_data WHERE item_id=$1::bigint`, f.item) != 1200000000 {
		t.Fatal("resume")
	}
	mustReport(t, f, "/emby/SESSIONS/Playing/Progress", "{"+base+`,"EventName":"Pause","PositionTicks":1210000000}`)
	if f.text(t, `SELECT state FROM native_playback_sessions WHERE id=$1`, id) != "paused" {
		t.Fatal("pause")
	}
	mustReport(t, f, "/Sessions/Playing/Progress", "{"+base+`,"IsPaused":false}`)
	mustReport(t, f, "/Sessions/Playing/Progress", "{"+base+`,"PositionTicks":2000000000}`)
	if f.integer(t, `SELECT position_ticks FROM native_playback_sessions WHERE id=$1`, id) != 2000000000 {
		t.Fatal("seek throttled")
	}
	f.exec(t, `UPDATE native_playback_sessions SET started_at=now()-interval '10 minutes',last_progress_at=now()-interval '4 minutes' WHERE id=$1`, id)
	beforePing := f.text(t, `SELECT row_to_json(d)::text FROM media_user_item_data d WHERE item_id=$1::bigint`, f.item)
	mustReport(t, f, "/Sessions/Playing/Ping?playSESSIONid="+id, "")
	if f.text(t, `SELECT row_to_json(d)::text FROM media_user_item_data d WHERE item_id=$1::bigint`, f.item) != beforePing {
		t.Fatal("Ping changed user data")
	}
	if n, err := playbackruntime.Reap(f.ctx, f.pool); err != nil || n != 0 {
		t.Fatal("heartbeat lease", n, err)
	}
	mustReport(t, f, "/Sessions/Playing/Progress", "{"+base+`,"PositionTicks":5700000000}`)
	mustReport(t, f, "/Sessions/Playing/Stopped", "{"+base+`,"PositionTicks":5800000000}`)
	for i := 0; i < 3; i++ {
		mustReport(t, f, "/Sessions/Playing/Stopped", "{"+base+`,"PositionTicks":5800000000}`)
	}
	if f.integer(t, `SELECT play_count FROM media_user_item_data WHERE item_id=$1::bigint`, f.item) != 1 || f.integer(t, `SELECT position_ticks FROM media_user_item_data WHERE item_id=$1::bigint`, f.item) != 0 || f.text(t, `SELECT played::text FROM media_user_item_data WHERE item_id=$1::bigint`, f.item) != "true" {
		t.Fatal("completion not idempotent")
	}
	mustReport(t, f, "/Sessions/Playing/Progress", "{"+base+`,"PositionTicks":5900000000}`)
	if f.text(t, `SELECT state FROM native_playback_sessions WHERE id=$1`, id) != "stopped" {
		t.Fatal("late report resurrected")
	}
	// A replay WITHOUT a stale explicit session ID is a fresh counted session.
	mustReport(t, f, "/Sessions/Playing", `{"ItemId":"`+f.item+`","MediaSourceId":"`+f.a.asset+`"}`)
	mustReport(t, f, "/Sessions/Playing/Stopped", `{"ItemId":"`+f.item+`","PositionTicks":5800000000}`)
	if f.integer(t, `SELECT play_count FROM media_user_item_data WHERE item_id=$1::bigint`, f.item) != 2 {
		t.Fatal("replay not counted")
	}
}
func TestNativeReportsLegacyFallbackUnknownAndAuthorization(t *testing.T) {
	f := newReportFixture(t)
	id := reportInfo(t, f)
	// Draft can become playing without a preceding stream and is not duplicated.
	user := strings.ReplaceAll(f.user.ID, "-", "")
	legacy := "/emby/Users/" + user + "/PlayingItems/" + f.item
	mustReport(t, f, legacy+"?mediaSOURCEid="+f.a.asset, "")
	if f.text(t, `SELECT state FROM native_playback_sessions WHERE id=$1`, id) != "playing" {
		t.Fatal("draft not activated")
	}
	mustReport(t, f, legacy+"/Progress?PositionTicks=1230000000&IsPaused=true", `{"EventName":"Pause"}`)
	if f.integer(t, `SELECT position_ticks FROM media_user_item_data WHERE item_id=$1::bigint`, f.item) != 1230000000 || f.integer(t, `SELECT play_count FROM media_user_item_data WHERE item_id=$1::bigint`, f.item) != 0 {
		t.Fatal("unknown runtime should only save position")
	}
	if w := reportHTTP(f, "DELETE", legacy+"?PositionTicks=1250000000", ""); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, body := range []string{`{"ItemId":"` + f.item + `","PositionTicks":-1}`, `{"PlaySessionId":"bad"}`, `{"ItemId":"` + f.item + `","UserId":"aaaaaaaa000040008000000000000034"}`} {
		if w := reportHTTP(f, "POST", "/Sessions/Playing", body); w.Code != 400 && w.Code != 403 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	otherToken := f.login(t, "different-report-device")
	w := reportHTTP(f, "POST", "/Sessions/Playing/Progress", `{"PlaySessionId":"`+id+`","PositionTicks":1}`)
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	saved := f.token
	f.token = otherToken
	w = reportHTTP(f, "POST", "/Sessions/Playing/Progress", `{"PlaySessionId":"`+id+`","PositionTicks":1}`)
	if w.Code != 403 {
		t.Fatal("foreign device session", w.Code, w.Body.String())
	}
	f.token = saved
	w = reportHTTP(f, "POST", "/Users/aaaaaaaa000040008000000000000034/PlayingItems/"+f.item, "")
	if w.Code != 403 {
		t.Fatal("cross user path", w.Code)
	}
	// Unknown ID, but valid item/source, may create a session; it remains bound to this device.
	mustReport(t, f, "/Sessions/Playing", `{"ItemId":"`+f.item+`","MediaSourceId":"`+f.a.asset+`","PlaySessionId":"aaaaaaaa-0000-4000-8000-000000000034"}`)
	if n := f.integer(t, `SELECT count(*) FROM native_playback_sessions WHERE state<>'stopped'`); n != 1 {
		t.Fatal("duplicate slot", n)
	}
}
func TestNativeReportsReapBatchAndAdminPage(t *testing.T) {
	f := newReportFixture(t)
	id := reportInfo(t, f)
	mustReport(t, f, "/Sessions/Playing", `{"PlaySessionId":"`+id+`","PositionTicks":1000000000}`)
	before := f.text(t, `SELECT row_to_json(d)::text FROM media_user_item_data d WHERE item_id=$1::bigint`, f.item)
	device := f.text(t, `SELECT device_id::text FROM native_playback_sessions WHERE id=$1`, id)
	f.exec(t, `INSERT INTO native_playback_sessions(id,user_id,device_id,item_id,media_source_id,state,started_at,last_progress_at)SELECT md5('stale-report-'||i)::uuid,$1::uuid,$2::uuid,$3::bigint,$4,'playing',now()-interval '10 minutes',now()-interval '6 minutes' FROM generate_series(1,201)i`, f.user.ID, device, f.item, f.a.asset)
	for _, want := range []int64{200, 1, 0} {
		if n, e := playbackruntime.Reap(f.ctx, f.pool); e != nil || n != want {
			t.Fatal(n, want, e)
		}
	}
	if after := f.text(t, `SELECT row_to_json(d)::text FROM media_user_item_data d WHERE item_id=$1::bigint`, f.item); after != before {
		t.Fatal("reaper changed user data")
	}
	page, e := playbackruntime.ListActive(f.ctx, f.pool, 0, 1)
	if e != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ItemID != f.item {
		t.Fatal(page, e)
	}
	end, e := playbackruntime.ListActive(f.ctx, f.pool, 1, 1)
	if e != nil || end.Total != 1 || len(end.Items) != 0 {
		t.Fatal(end, e)
	}
}
func TestNativeReportsConcurrentCompletionOnce(t *testing.T) {
	f := newReportFixture(t)
	f.probeSnapshot(t, f.a, 6000000000, 1920)
	id := reportInfo(t, f)
	mustReport(t, f, "/Sessions/Playing", `{"PlaySessionId":"`+id+`"}`)
	var wg sync.WaitGroup
	codes := make(chan int, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := reportHTTP(f, "POST", "/Sessions/Playing/Stopped", `{"PlaySessionId":"`+id+`","PositionTicks":5800000000}`)
			codes <- w.Code
		}()
	}
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != 204 {
			t.Fatal(c)
		}
	}
	if f.integer(t, `SELECT play_count FROM media_user_item_data WHERE item_id=$1::bigint`, f.item) != 1 {
		t.Fatal("duplicate credit")
	}
}
func TestNativeSubtitleEmbedProjectionAnd404(t *testing.T) {
	f := newReportFixture(t)
	f.probeSnapshot(t, f.a, 6000000000, 1920)
	f.exec(t, `UPDATE media_probe_results SET result=jsonb_set(result,'{streams}',(result->'streams')||'[{"index":7,"type":"Subtitle","codec":"srt","language":"zho","isTextSubtitleStream":true}]') WHERE cloud_file_id=$1`, f.a.file)
	w := f.call("GET", "/Items/"+f.item+"/PlaybackInfo?MediaSourceId="+f.a.asset, f.token, "Infuse/8")
	var v dto.PlaybackInfoResponse
	if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil || w.Code != 200 {
		t.Fatal(w.Code, e, w.Body.String())
	}
	streams := v.MediaSources[0].MediaStreams
	stream := streams[len(streams)-1]
	if stream.Type != "Subtitle" || stream.Index == nil || *stream.Index != 7 || stream.IsExternal || stream.DeliveryMethod != "Embed" || strings.Contains(w.Body.String(), "DeliveryUrl") {
		t.Fatal("embedded delivery", w.Body.String())
	}
	for _, path := range []string{"/Videos/" + f.item + "/subtitles/7/stream", "/emby/Videos/" + f.item + "/" + f.a.asset + "/Subtitles/7/Stream.srt"} {
		for _, method := range []string{"GET", "HEAD"} {
			w = f.call(method, path, f.token, "Infuse/8")
			if w.Code != 404 {
				t.Fatal(path, w.Code)
			}
		}
	}
	if f.integer(t, `SELECT count(*) FROM native_playback_sessions WHERE state<>'stopped'`) != 0 {
		t.Fatal("subtitle consumed slot")
	}
}
func TestPlaybackReportsMigrationRoundTrip(t *testing.T) {
	// A historical migration test seeds its historical row directly. Do not run
	// today's UA runtime against schema 61 or cross irreversible migration 68.
	f := newBrowseFixture(t, 60)
	if _, e := f.provider.UpTo(f.ctx, 61); e != nil {
		t.Fatal(e)
	}
	id := jobs.UUID()
	device := f.text(t, `SELECT id::text FROM media_user_devices WHERE user_id=$1 LIMIT 1`, f.user.ID)
	f.exec(t, `INSERT INTO native_playback_sessions(id,user_id,device_id,item_id,media_source_id,play_counted)VALUES($1,$2,$3,$4,'synthetic-migration',true)`, id, f.user.ID, device, f.parents["g:test-a"])

	if _, e := f.provider.DownTo(f.ctx, 60); e != nil {
		t.Fatal(e)
	}
	if _, e := f.provider.UpTo(f.ctx, 61); e != nil {
		t.Fatal(e)
	}
	if f.text(t, `SELECT play_counted::text FROM native_playback_sessions WHERE id=$1`, id) != "false" {
		t.Fatal(fmt.Sprint("migration default"))
	}
}

func TestNativeReportsSlotRenewalAndCrossUserSession(t *testing.T) {
	f := newReportFixture(t)
	f.exec(t, `UPDATE media_users SET max_streams=1 WHERE id=$1::uuid`, f.user.ID)
	id := reportInfo(t, f)
	mustReport(t, f, "/Sessions/Playing", `{"PlaySessionId":"`+id+`"}`)
	f.exec(t, `UPDATE native_playback_sessions SET started_at=now()-interval '10 minutes',last_progress_at=now()-interval '4 minutes' WHERE id=$1`, id)
	mustReport(t, f, "/Sessions/Playing/Progress", `{"PlaySessionId":"`+id+`","PositionTicks":10}`)
	second := f.login(t, "second-slot-device")
	saved := f.token
	f.token = second
	w := reportHTTP(f, "POST", "/Sessions/Playing", `{"ItemId":"`+f.item+`","MediaSourceId":"`+f.a.asset+`"}`)
	if w.Code != 403 {
		t.Fatal("renewed slot lost", w.Code, w.Body.String())
	}
	f.token = saved
	mustReport(t, f, "/Sessions/Playing/Stopped", `{"PlaySessionId":"`+id+`"}`)
	f.token = second
	mustReport(t, f, "/Sessions/Playing", `{"ItemId":"`+f.item+`","MediaSourceId":"`+f.a.asset+`"}`)
	if _, e := (mediausers.Repository{DB: f.pool}).Save(f.ctx, "", mediausers.Input{Username: "reports.other", Password: "OtherReport123", Enabled: true}); e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", "/Users/AuthenticateByName", strings.NewReader(`{"Username":"reports.other","Pw":"OtherReport123"}`))
	r.Header.Set("X-Emby-Authorization", `MediaBrowser DeviceId="other-user-device"`)
	w = httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	var auth dto.AuthenticationResult
	if e := json.Unmarshal(w.Body.Bytes(), &auth); e != nil || w.Code != 200 {
		t.Fatal(w.Code, w.Body.String(), e)
	}
	f.token = auth.AccessToken
	w = reportHTTP(f, "POST", "/Sessions/Playing/Stopped", `{"PlaySessionId":"`+id+`"}`)
	if w.Code != 403 {
		t.Fatal("foreign user session", w.Code, w.Body.String())
	}
}
func TestNativeReportsEpisodeWritesOnlyEpisodeData(t *testing.T) {
	f := newReportFixture(t)
	f.library(t, "g:report-tv", "tvshows", true, false)
	media := f.media(t, 3334001, "Synthetic Report Show", "2026", `{}`)
	f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, media)
	ep := f.episode(t, media, "g:report-tv", 1, 1, 2)
	for {
		n, e := catalog.PreallocateIDBatch(f.ctx, f.pool)
		if e != nil {
			t.Fatal(e)
		}
		if n < 500 {
			break
		}
	}
	episodeID := f.text(t, `SELECT id::text FROM native_item_ids WHERE entity_type='episode' AND internal_key='tmdb:3334001:s1e1'`)
	f.probeSnapshot(t, ep, 6000000000, 1920)
	mustReport(t, f, "/Sessions/Playing", `{"ItemId":"`+episodeID+`","MediaSourceId":"`+ep.asset+`","PositionTicks":1200000000}`)
	if f.integer(t, `SELECT position_ticks FROM media_user_item_data WHERE item_id=$1::bigint`, episodeID) != 1200000000 {
		t.Fatal("episode resume")
	}
	if f.integer(t, `SELECT count(*) FROM media_user_item_data d JOIN native_item_ids i ON i.id=d.item_id WHERE i.entity_type IN('series','season')`) != 0 {
		t.Fatal("wrote series aggregate")
	}
}

func TestNativeReportsStartWithoutPositionPreservesResume(t *testing.T) {
	f := newReportFixture(t)
	id := reportInfo(t, f)
	f.exec(t, `INSERT INTO media_user_item_data(user_id,item_id,position_ticks)VALUES($1::uuid,$2::bigint,1000000000)`, f.user.ID, f.item)
	mustReport(t, f, "/Sessions/Playing", `{"PlaySessionId":"`+id+`"}`)
	if f.integer(t, `SELECT position_ticks FROM media_user_item_data WHERE item_id=$1::bigint`, f.item) != 1000000000 {
		t.Fatal("start without position cleared resume")
	}
}

func TestNativeReportsNewSessionDoesNotInheritExpiredCompletion(t *testing.T) {
	f := newReportFixture(t)
	f.probeSnapshot(t, f.a, 6000000000, 1920)
	id := reportInfo(t, f)
	mustReport(t, f, "/Sessions/Playing", `{"PlaySessionId":"`+id+`","PositionTicks":5800000000}`)
	f.exec(t, `UPDATE native_playback_sessions SET started_at=now()-interval '10 minutes',last_progress_at=now()-interval '6 minutes' WHERE id=$1`, id)
	// Crash/reaper lag must not transfer a previous expired play's credit to
	// a genuinely new play session (without an intervening PlaybackInfo call).
	mustReport(t, f, "/Sessions/Playing", `{"ItemId":"`+f.item+`","MediaSourceId":"`+f.a.asset+`","PlaySessionId":"bbbbbbbb-0000-4000-8000-000000000034","PositionTicks":5800000000}`)
	if f.integer(t, `SELECT play_count FROM media_user_item_data WHERE item_id=$1::bigint`, f.item) != 2 {
		t.Fatal("expired completion leaked into new session")
	}
}

func TestNativeReportsVersionSwitchMergesPlaybackInfoSession(t *testing.T) {
	for _, limit := range []int{1, 2} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			f := newReportFixture(t)
			f.exec(t, `UPDATE media_users SET max_streams=$2 WHERE id=$1::uuid`, f.user.ID, limit)
			id := reportInfo(t, f)
			if w := f.call("GET", "/Videos/"+f.item+"/stream?MediaSourceId="+f.a.asset, f.token, "Infuse/8"); w.Code != 302 {
				t.Fatal(w.Code, w.Body.String())
			}
			if limit == 2 {
				if w := f.call("GET", "/Videos/"+f.item+"/stream?MediaSourceId="+f.b.asset, f.token, "Infuse/8"); w.Code != 302 {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			bad := reportHTTP(f, "POST", "/Sessions/Playing", `{"PlaySessionId":"`+id+`","MediaSourceId":"`+f.other.asset+`"}`)
			if bad.Code != 404 {
				t.Fatal("foreign item version accepted", bad.Code, bad.Body.String())
			}
			mustReport(t, f, "/Sessions/Playing", `{"PlaySessionId":"`+id+`","MediaSourceId":"`+f.b.asset+`"}`)
			if f.integer(t, `SELECT count(*) FROM native_playback_sessions WHERE state<>'stopped'`) != 1 || f.text(t, `SELECT media_source_id FROM native_playback_sessions WHERE id=$1`, id) != f.b.asset {
				t.Fatal("version switch did not merge slot")
			}
			if w := f.call("GET", "/Videos/"+f.item+"/stream?MediaSourceId="+f.b.asset, f.token, "Infuse/8"); w.Code != 302 {
				t.Fatal(w.Code, w.Body.String())
			}
			mustReport(t, f, "/Sessions/Playing/Stopped", `{"PlaySessionId":"`+id+`","MediaSourceId":"`+f.b.asset+`"}`)
			if f.integer(t, `SELECT count(*) FROM native_playback_sessions WHERE state<>'stopped'`) != 0 {
				t.Fatal("stop left duplicate version slot")
			}
		})
	}
}
