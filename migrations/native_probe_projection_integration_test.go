package migrations_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/linksource"
	"media-tool/internal/modules/mediaprobe/probequeue"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
)

func (f browseFixture) probeSnapshot(t *testing.T, a movieAsset, duration int64, width int) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"schema_version": 1, "durationTicks": duration, "container": "mp4", "size": 1234, "bitRate": 100000, "streams": []map[string]any{{"index": 0, "type": "Video", "codec": "hevc", "width": width, "height": 1080, "videoRange": "HDR", "videoRangeType": "HDR10"}, {"index": 5, "type": "Subtitle", "codec": "srt", "language": "eng", "isTextSubtitleStream": true}}})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO media_probe_results(cloud_file_id,file_fingerprint,result_fingerprint,state,result,probed_at)
 SELECT f.id,native_probe_fingerprint(f.size_bytes,f.provider_file_id,f.metadata),native_probe_fingerprint(f.size_bytes,f.provider_file_id,f.metadata),'ready',$2,now() FROM cloud_files f WHERE f.id=$1`, a.file, raw)
}
func assertSourceProbe(t *testing.T, items []dto.BaseItemDto, id string, width int) {
	t.Helper()
	for _, item := range items {
		for _, s := range item.MediaSources {
			if s.Id == id {
				if len(s.MediaStreams) != 2 || s.Width == nil || *s.Width != width || s.MediaStreams[1].Index == nil || *s.MediaStreams[1].Index != 5 || s.Path != "" || s.SupportsTranscoding() {
					t.Fatal("wrong per-asset probe projection", s)
				}
				return
			}
		}
	}
	t.Fatal("source not found")
}
func TestNativeProbePageSnapshotReadOnceAndScopedAcrossServices(t *testing.T) {
	f := newBrowseFixture(t)
	movie := f.media(t, 240001, "Synthetic probe film", "2026", `{"genres":["Drama"]}`)
	first := f.asset(t, movie, "g:test-a", time.Now(), "mkv")
	second := f.asset(t, movie, "g:test-a", time.Now().Add(time.Hour), "mkv")
	third := f.asset(t, movie, "g:test-b", time.Now(), "mkv")
	f.probeSnapshot(t, first, 600000000, 1920)
	f.probeSnapshot(t, second, 700000000, 3840)
	f.probeSnapshot(t, third, 800000000, 1280)
	q := nativelibrary.ItemQuery{ParentID: f.parents["g:test-a"], Limit: 200}
	before := f.trace.probes.Load()
	items, total := f.query(t, q)
	if total != 1 || len(items) != 1 || f.trace.probes.Load()-before != 1 || len(items[0].MediaSources) != 2 {
		t.Fatal("unified page probe batch")
	}
	assertSourceProbe(t, items, first.asset, 1920)
	assertSourceProbe(t, items, second.asset, 3840)
	if *items[0].Width != 3840 || *items[0].RunTimeTicks != 700000000 {
		t.Fatal("first version must control item technical fields")
	}
	before = f.trace.probes.Load()
	movies, _, err := (catalog.MovieRepository{DB: f.pool}).Query(f.ctx, catalog.MovieQuery{LibraryID: "g:test-a", Visible: nativelibrary.VisibleLibraries{RequestedID: "g:test-a", AllowedIDs: []string{"g:test-a"}}, Limit: 200})
	if err != nil || f.trace.probes.Load()-before != 1 {
		t.Fatal("movie service probe reads", err)
	}
	assertSourceProbe(t, movies, first.asset, 1920)
	tvID := f.library(t, "g:probe-tv", "tvshows", true, false)
	show := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',240002,'Synthetic probe show','{}')`, show)
	ep1 := f.episode(t, show, "g:probe-tv", 0, 1, 2)
	ep2 := f.episode(t, show, "g:probe-tv", 0, 1, 1)
	ep3 := f.episode(t, show, "g:probe-tv", 1, 1, 1)
	f.probeSnapshot(t, ep1, 900000000, 1920)
	f.probeSnapshot(t, ep2, 1000000000, 3840)
	f.probeSnapshot(t, ep3, 1100000000, 1280)
	shows, _ := f.query(t, nativelibrary.ItemQuery{ParentID: tvID, Limit: 200})
	seriesID := shows[0].Id
	before = f.trace.probes.Load()
	episodes, _, err := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Episode"}, Limit: 200})
	if err != nil || f.trace.probes.Load()-before != 1 {
		t.Fatal("unified episodes probe reads", err)
	}
	assertSourceProbe(t, episodes, ep2.asset, 3840)
	visible := nativelibrary.VisibleLibraries{RequestedID: "g:probe-tv", AllowedIDs: []string{"g:probe-tv"}}
	repo := catalog.HierarchyRepository{DB: f.pool}
	before = f.trace.probes.Load()
	direct, total, err := repo.ListEpisodes(f.ctx, nativelibrary.EpisodeQuery{Visible: visible, SeriesID: seriesID, Limit: 200})
	if err != nil || total != 2 || f.trace.probes.Load()-before != 1 {
		t.Fatal("hierarchy page probe reads", total, err)
	}
	assertSourceProbe(t, direct, ep1.asset, 1920)
	// A single-item direct lookup still issues only one page snapshot read.
	before = f.trace.probes.Load()
	detail, err := repo.GetEpisode(f.ctx, nativelibrary.EpisodeQuery{Visible: visible, Limit: 1}, direct[1].Id)
	if err != nil || f.trace.probes.Load()-before != 1 {
		t.Fatal("detail probe reads", err)
	}
	assertSourceProbe(t, []dto.BaseItemDto{detail}, ep3.asset, 1280)

	related := f.media(t, 240004, "Synthetic related film", "2026", `{"genres":["Drama"]}`)
	relatedAsset := f.asset(t, related, "g:test-a", time.Now(), "mkv")
	f.probeSnapshot(t, relatedAsset, 100000000, 720)
	if _, err := catalog.PreallocateIDBatch(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	before = f.trace.probes.Load()
	similar, err := f.repo.Discover(f.ctx, f.user, f.permissions(), nativelibrary.DiscoveryQuery{Mode: nativelibrary.Similar, ItemID: items[0].Id, Items: nativelibrary.ItemQuery{Limit: 20}})
	if err != nil || len(similar.Items) != 1 || f.trace.probes.Load()-before != 1 {
		t.Fatal("Similar preflight added a probe read", len(similar.Items), f.trace.probes.Load()-before, err)
	}
	assertSourceProbe(t, similar.Items, relatedAsset.asset, 720)
	if _, err := f.provider.UpTo(f.ctx, 60); err != nil {
		t.Fatal(err)
	}
	// Result is intentionally older than the new file fingerprint and attempt.
	f.exec(t, `UPDATE media_probe_results SET state='pending',file_fingerprint='synthetic-new-file',next_attempt_at=now()+interval '1 day' WHERE cloud_file_id=$1`, second.file)
	retained, _ := f.query(t, q)
	assertSourceProbe(t, retained, second.asset, 3840)
	// A corrupt snapshot degrades only that version; other versions remain intact.
	f.exec(t, `UPDATE media_probe_results SET result='{"schema_version":1,"durationTicks":-1}' WHERE cloud_file_id=$1`, first.file)
	retained, _ = f.query(t, q)
	assertSourceProbe(t, retained, second.asset, 3840)
	for _, s := range retained[0].MediaSources {
		if s.Id == first.asset && len(s.MediaStreams) != 0 {
			t.Fatal("invalid snapshot leaked")
		}
	}
	f.exec(t, `DELETE FROM media_user_library_grants WHERE user_id=$1`, f.user.ID)
	denied, total, err := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{Recursive: boolPtr(true), IncludeItemTypes: []string{"Movie", "Episode"}, Limit: 200})
	if err != nil || total != 0 || len(denied) != 0 {
		t.Fatal("ungranted probed assets visible", err)
	}
}

func TestNativeProbeSchemaSweepOnceRetainsSnapshotAndBackoff(t *testing.T) {
	f := newProbeQueueFixture(t)
	f.enqueue(t, false)
	f.succeed(t, f.claim(t), 770000000, true)
	f.exec(t, `UPDATE media_probe_results SET result='{"durationTicks":770000000}' WHERE cloud_file_id=$1`, f.a.file)
	_, n, err := f.r.ScheduleBatch(f.ctx, "")
	if err != nil || n != 1 || f.state(t) != "pending" {
		t.Fatal("legacy not swept", n, err)
	}
	next := f.text(t, `SELECT next_attempt_at::text FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file)
	if _, _, err = f.r.ScheduleBatch(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if got := f.text(t, `SELECT next_attempt_at::text FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file); got != next {
		t.Fatal("schema sweep reset pending")
	}
	l := f.claim(t)
	if _, n, err = f.r.ScheduleBatch(f.ctx, ""); err != nil || n != 0 {
		t.Fatal("schema sweep reset running", err)
	}
	if _, err := f.r.Enqueue(f.ctx, f.a.asset); err != nil {
		t.Fatal(err)
	}
	if f.text(t, `SELECT lease_token::text FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file) != l.Token {
		t.Fatal("schema enqueue invalidated lease")
	}
	f.fail(t, l, linksource.Retryable, string(probequeue.ProcessFailed), true)
	next = f.text(t, `SELECT next_attempt_at::text FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file)
	if _, n, err = f.r.ScheduleBatch(f.ctx, ""); err != nil || n != 0 {
		t.Fatal("schema bypassed backoff", err)
	}
	if _, err = f.r.Enqueue(f.ctx, f.a.asset); err != nil {
		t.Fatal(err)
	}
	if f.text(t, `SELECT next_attempt_at::text FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file) != next || f.integer(t, `SELECT attempts FROM media_probe_results WHERE cloud_file_id=$1`, f.a.file) != 1 {
		t.Fatal("schema retrigger reset retry")
	}
	old, err := f.r.Current(f.ctx, f.a.file)
	if err != nil || old.Result.DurationTicks != 770000000 || old.Result.SchemaVersion != 0 {
		t.Fatal("old success lost", err)
	}
	f.due(t)
	f.succeed(t, f.claim(t), 880000000, true)
	if _, n, err = f.r.ScheduleBatch(f.ctx, ""); err != nil || n != 0 {
		t.Fatal("current schema requeued", err)
	}
	// Terminal/manual old-result attempts also retain T23's explicit retry gate.
	for _, class := range []linksource.Class{linksource.Permanent, linksource.Manual} {
		f.enqueue(t, true)
		l = f.claim(t)
		reason := string(linksource.FileMissing)
		if class == linksource.Manual {
			reason = string(linksource.Authorization)
		}
		f.fail(t, l, class, reason, true)
		f.exec(t, `UPDATE media_probe_results SET result='{"durationTicks":880000000}' WHERE cloud_file_id=$1`, f.a.file)
		if _, n, err = f.r.ScheduleBatch(f.ctx, ""); err != nil || n != 0 {
			t.Fatal("schema reset terminal", class, err)
		}
	}
}

func TestNativeProbePageSnapshotByteBudget(t *testing.T) {
	f := newBrowseFixture(t)
	movie := f.media(t, 240003, "Synthetic bounded snapshots", "2026", `{}`)
	raw, err := json.Marshal(map[string]any{"durationTicks": 1, "unusedSyntheticPadding": strings.Repeat("x", 3<<20)})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		a := f.asset(t, movie, "g:test-a", time.Now(), "mkv")
		f.exec(t, `INSERT INTO media_probe_results(cloud_file_id,file_fingerprint,result_fingerprint,state,result,probed_at) SELECT f.id,native_probe_fingerprint(f.size_bytes,f.provider_file_id,f.metadata),native_probe_fingerprint(f.size_bytes,f.provider_file_id,f.metadata),'ready',$2,now() FROM cloud_files f WHERE f.id=$1`, a.file, raw)
	}
	before := f.trace.probes.Load()
	items, _, err := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{ParentID: f.parents["g:test-a"], Limit: 200})
	if err == nil || len(items) > 0 || f.trace.probes.Load()-before != 1 {
		t.Fatal("oversized probe page must fail atomically", err)
	}
}

var updateProbeHTTP = flag.Bool("update-probe-http", false, "update synthetic probed HTTP response golden")

func assertProbeHTTPGolden(t *testing.T, response dto.QueryResult[dto.BaseItemDto]) {
	t.Helper()
	if len(response.Items) != 1 {
		t.Fatal("expected one probed HTTP item")
	}
	item := response.Items[0]
	item.Id, item.ParentId, item.ServerId = "100101", "100001", strings.Repeat("a", 32)
	if item.UserData != nil {
		data := *item.UserData
		data.Key = item.Id
		item.UserData = &data
	}
	item.MediaSources = append(dto.Array[dto.MediaSourceInfo]{}, item.MediaSources...)
	for i := range item.MediaSources {
		item.MediaSources[i].Id = "synthetic-source"
	}
	response.Items = dto.Array[dto.BaseItemDto]{item}
	raw, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	file := filepath.Join("..", "internal", "platform", "httpapi", "testdata", "emby", "items-probed.json")
	if *updateProbeHTTP {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(want) {
		t.Fatalf("probed HTTP contract differs\n%s", raw)
	}
}
