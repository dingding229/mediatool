package migrations_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/mediaprobe/stillrunner"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/mediausers/nativeauth"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativelibrary/catalog"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/httpapi"
	"media-tool/internal/platform/secrets"
)

func stillRuntime(t *testing.T, f stillFixture) (*stillrunner.Service, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(root, "native-media", "stills")
	vault, err := secrets.New(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	m := integrations.NewManager(integrations.Repository{DB: f.pool, Vault: vault}, config.Config{PlaybackResolveTimeout: 30 * time.Second, WorkerPollInterval: 100 * time.Millisecond, WorkerLease: time.Minute})
	t.Cleanup(m.Close)
	service, err := stillrunner.New(m, slog.New(slog.NewTextHandler(io.Discard, nil)), root, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Error(err)
		}
	})
	return service, root
}

type frameTicket struct{}

func (*frameTicket) Claim() (string, string, error) {
	return "https://synthetic.invalid/frame?token=NEVER_LOG", "SyntheticProbe/1.0", nil
}

type frameLinks struct{ calls, reserves int }

func (l *frameLinks) Reserve(ctx context.Context) (context.Context, func(), error) {
	l.reserves++
	return ctx, func() {}, nil
}
func (l *frameLinks) Fetch(context.Context, string) (stillrunner.Ticket, error) {
	l.calls++
	return &frameTicket{}, nil
}

type frameExecutor struct {
	raw   []byte
	calls int
}

func (f *frameExecutor) Reserve(ctx context.Context) (context.Context, func(), error) {
	return ctx, func() {}, nil
}
func (f *frameExecutor) Still(_ context.Context, _ string, _ time.Duration, o stillrunner.FrameOptions) ([]byte, error) {
	f.calls++
	if o.UserAgent != "SyntheticProbe/1.0" {
		return nil, stillrunner.ErrImage
	}
	return f.raw, nil
}
func TestNativeStillRuntimeQueueSweepEngineAndSwitches(t *testing.T) {
	f := newStillFixture(t)
	service, _ := stillRuntime(t, f)
	if queued, err := service.Repo.EnqueueAsset(f.ctx, f.source.asset); err != nil || !queued {
		t.Fatal("probe-ready enqueue", queued, err)
	}
	if _, count, err := service.Repo.ScheduleBatch(f.ctx, 0); err != nil || count == 0 || count > 200 {
		t.Fatal(count, err)
	}
	payload := []byte(f.text(t, `SELECT payload::text FROM jobs WHERE kind='native.still' LIMIT 1`))
	frames := &frameExecutor{raw: stillJPEG(t)}
	links := &frameLinks{}
	engine := stillrunner.Engine{Store: service.Repo, Links: links, Frames: frames, Files: service, Config: service.Repo.Limits()}
	if err := engine.Process(f.ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Repo.Current(f.ctx, f.itemID)
	if err != nil || snapshot == nil || snapshot.SizeBytes == 0 || frames.calls != 3 || links.calls != 3 || links.reserves != 3 {
		t.Fatal("pipeline incomplete", snapshot, frames.calls, links.calls, links.reserves, err)
	}
	for _, flag := range []string{"enabled", "probe_enabled", "stills_enabled"} {
		if _, err := service.Repo.Retry(f.ctx, f.itemID); err != nil {
			t.Fatal(err)
		}
		f.exec(t, `UPDATE native_server_settings SET `+flag+`=false`)
		before := frames.calls
		if err := engine.Process(f.ctx, jobs.Job{Payload: payload}); err != nil || frames.calls != before {
			t.Fatal("disabled execution", flag, err)
		}
		if _, count, err := service.Repo.ScheduleBatch(f.ctx, 0); err != nil || count != 0 {
			t.Fatal("disabled sweep", flag, count, err)
		}
		f.exec(t, `UPDATE native_server_settings SET `+flag+`=true`)
	}
	// TMDB arriving retires the old reference; only GC may remove the file later.
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,season_number,language,payload,state,fetched_at)VALUES('season',250001,0,'zh-CN','{"episodes":[{"number":1,"stillPath":"/synthetic.jpg"}]}','ready',now())`)
	if err := service.Maintain(f.ctx); err != nil {
		t.Fatal(err)
	}
	if s, err := service.Repo.Current(f.ctx, f.itemID); err != nil || s != nil {
		t.Fatal("obsolete local reference retained", err)
	}
}
func TestNativeStillRuntimeImagesProjectionFallbackAndCooldown(t *testing.T) {
	f := newStillFixture(t)
	service, root := stillRuntime(t, f)
	l := rawStillClaim(t, f)
	image, err := service.Publish(f.ctx, l, stillJPEG(t))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := service.Repo.Succeed(f.ctx, l, image); err != nil || !ok {
		t.Fatal(ok, err)
	}
	id := strconv.FormatInt(f.itemID, 10)
	expected := nativelibrary.StillImageTag(l.Fingerprint, image.Digest)
	var requests atomic.Int64
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 0}); err != nil {
			t.Error(err)
		}
	}))
	defer tmdb.Close()
	resolver := httpapi.NewNativeImageResolver(f.pool, nil, nil)
	resolver.ImageBaseURL = tmdb.URL
	resolver.Stills = service
	accounts := &nativeauth.Store{DB: f.pool, Secret: "synthetic-image-key"}
	login, err := accounts.Login(f.ctx, "browse.member", "Browse123", contracts.DeviceInfo{DeviceID: "image-device"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := embyapi.NewServer(embyapi.ServerOptions{Settings: settings.NewNativeServerRepository(f.pool), Images: resolver, Auth: accounts, Permissions: func() contracts.PermissionChecker { return f.permissions() }})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, tag string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/emby/Items/"+id+"/Images/Primary?tag="+tag, nil)
		req.Header.Set("X-Emby-Token", login.AccessToken)
		server.ServeHTTP(w, req)
		return w
	}
	w := call("GET", expected)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") || w.Header().Get("ETag") != "\""+expected+"\"" || requests.Load() != 0 {
		t.Fatal(w.Code, w.Header(), requests.Load(), w.Body.String())
	}
	if w := call("HEAD", expected); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("HEAD", w.Code)
	}
	if w := call("GET", "old-tag"); w.Code != 200 || strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("stale cache policy", w.Code)
	}
	// Real page projection uses exactly one additional batched still lookup.
	before := f.trace.queries.Load()
	beforeStills := f.trace.stills.Load()
	items, total, err := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{IDs: []string{id}, Limit: 200})
	if err != nil || total != 1 || len(items) != 1 || items[0].ImageTags["Primary"] != expected || f.trace.queries.Load()-before > 12 || f.trace.stills.Load()-beforeStills != 1 {
		t.Fatal("projection tag mismatch", total, err)
	}
	// A refreshed season still must beat an asset's persisted older TMDB path
	// in both projection paths, exactly as it does in the image resolver.
	f.exec(t, `UPDATE media_assets SET media_identity=jsonb_set(media_identity,'{episodeDetails,0,stillPath}','"/old-asset.jpg"') WHERE id=$1`, f.source.asset)
	// New TMDB data wins immediately and retires the local reference.
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,season_number,language,payload,state,fetched_at)VALUES('season',250001,0,'zh-CN','{"posterPath":"/season.jpg","episodes":[{"number":1,"stillPath":"/episode.jpg"}]}','ready',now())`)
	w = call("GET", expected)
	if w.Code != 200 || w.Header().Get("ETag") != "\""+nativelibrary.ImageTag("/episode.jpg")+"\"" || strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("TMDB not preferred", w.Code, w.Header(), w.Body.String())
	}
	if s, err := service.Repo.Current(f.ctx, f.itemID); err != nil || s != nil {
		t.Fatal("TMDB switch did not retire", err)
	}
	assertTag := func(expected string) {
		t.Helper()
		before := f.trace.stills.Load()
		items, total, err := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{IDs: []string{id}, Limit: 200})
		if err != nil || total != 1 || len(items) != 1 || items[0].ImageTags["Primary"] != expected || f.trace.stills.Load()-before != 1 {
			t.Fatal("refreshed still projection disagrees with endpoint", total, err)
		}
		seriesID := strconv.FormatInt(f.integer(t, `SELECT id FROM native_item_ids WHERE entity_type='series' AND internal_key='tmdb:250001'`), 10)
		before = f.trace.stills.Load()
		episodes, total, err := (catalog.HierarchyRepository{DB: f.pool}).ListEpisodes(f.ctx, nativelibrary.EpisodeQuery{SeriesID: seriesID, Visible: nativelibrary.VisibleLibraries{RequestedID: "g:still-tv", AllowedIDs: []string{"g:still-tv"}}, Limit: 200})
		if err != nil || total != 1 || len(episodes) != 1 || episodes[0].ImageTags["Primary"] != expected || f.trace.stills.Load()-before != 1 {
			t.Fatal("hierarchy still projection disagrees with endpoint", total, err)
		}
	}
	assertTag(nativelibrary.ImageTag("/episode.jpg"))
	// An explicit episode snapshot wins over both the season and the asset.
	f.exec(t, `INSERT INTO native_tmdb_details(tmdb_type,tmdb_id,season_number,episode_number,language,payload,state,fetched_at)VALUES('episode',250001,0,1,'zh-CN','{"stillPath":"/newest-episode.jpg"}','ready',now())`)
	assertTag(nativelibrary.ImageTag("/newest-episode.jpg"))
	if w := call("GET", expected); w.Code != 200 || w.Header().Get("ETag") != "\""+nativelibrary.ImageTag("/newest-episode.jpg")+"\"" {
		t.Fatal("episode snapshot not preferred", w.Code, w.Header())
	}
	// Remove still: fallback to the season poster, without immediate regeneration.
	f.exec(t, `UPDATE media_assets SET media_identity=media_identity#-'{episodeDetails,0,stillPath}' WHERE id=$1`, f.source.asset)
	f.exec(t, `UPDATE native_tmdb_details SET payload='{"posterPath":"/season.jpg"}' WHERE tmdb_id=250001`)
	w = call("GET", expected)
	if w.Code != 200 || w.Header().Get("ETag") != "\""+nativelibrary.ImageTag("/season.jpg")+"\"" {
		t.Fatal("poster fallback", w.Code, w.Header())
	}
	if f.state(t) != "skipped" {
		t.Fatal("cooldown bypassed")
	}
	f.exec(t, `UPDATE media_stills SET evicted_at=clock_timestamp()-interval '25 hours' WHERE item_id=$1`, f.itemID)
	w = call("GET", expected)
	if w.Code != 200 || f.state(t) != "pending" {
		t.Fatal("cooldown not requeued", w.Code, f.state(t))
	}
	// A DB reference whose file disappeared must still fall back, never return 500.
	next := rawStillClaim(t, f)
	image, err = service.Publish(f.ctx, next, stillJPEG(t))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := service.Repo.Succeed(f.ctx, next, image); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := os.Remove(filepath.Join(root, image.RelativePath)); err != nil {
		t.Fatal(err)
	}
	if w := call("GET", expected); w.Code != 200 {
		t.Fatal("missing file did not degrade", w.Code, w.Body.String())
	}
	f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=$1`, f.source.file)
	if w := call("GET", expected); w.Code != 404 {
		t.Fatal("unexposed image leaked", w.Code)
	}
}

func TestNativeStillRuntimeStartupBudgetAndDisabledLazyFiles(t *testing.T) {
	f := newStillFixture(t)
	service, root := stillRuntime(t, f)
	f.exec(t, `UPDATE native_server_settings SET enabled=false`)
	ctx, cancel := context.WithTimeout(f.ctx, 150*time.Millisecond)
	defer cancel()
	if err := service.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("disabled startup created cache root", err)
	}
	vault, err := secrets.New(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	manager := integrations.NewManager(integrations.Repository{DB: f.pool, Vault: vault}, config.Config{PlaybackResolveTimeout: time.Minute})
	defer manager.Close()
	if _, err := stillrunner.New(manager, slog.New(slog.NewTextHandler(io.Discard, nil)), root, 0); err == nil {
		t.Fatal("invalid worst-case budget did not propagate")
	}
	movie := f.media(t, 250099, "Synthetic no capture movie", "2026", `{}`)
	a := f.asset(t, movie, "g:test-a", time.Now(), "mkv")
	f.exec(t, `UPDATE native_server_settings SET enabled=true`)
	if queued, err := service.Repo.EnqueueAsset(f.ctx, a.asset); err != nil || queued {
		t.Fatal("movie scheduled for screenshot", queued, err)
	}
}

// Authorized artwork is global per logical episode ID even when the current user can only
// browse an older version. Never derive its tag from a per-library asset alone.
func TestNativeStillRuntimeAssetImageTagAcrossLibraries(t *testing.T) {
	f := newStillFixture(t)
	f.library(t, "g:still-foreign", "tvshows", false, false)
	show := f.text(t, `SELECT media_id::text FROM media_asset_bindings WHERE asset_id=$1`, f.source.asset)
	other := f.episode(t, show, "g:still-foreign", 0, 1, 2)
	f.exec(t, `UPDATE media_assets SET media_identity=jsonb_set(media_identity,'{episodeDetails,0,stillPath}','"/own-asset.jpg"') WHERE id=$1`, f.source.asset)
	f.exec(t, `UPDATE media_assets SET media_identity=jsonb_set(media_identity,'{episodeDetails,0,stillPath}','"/global-asset.jpg"'),created_at=now() WHERE id=$1`, other.asset)
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 0}); err != nil {
			t.Error(err)
		}
	}))
	defer tmdb.Close()
	resolver := httpapi.NewNativeImageResolver(f.pool, nil, nil)
	resolver.ImageBaseURL = tmdb.URL
	id := strconv.FormatInt(f.itemID, 10)
	seriesID := strconv.FormatInt(f.integer(t, `SELECT id FROM native_item_ids WHERE entity_type='series' AND internal_key='tmdb:250001'`), 10)
	for _, path := range []string{"/global-asset.jpg", "/own-asset.jpg"} {
		if path == "/own-asset.jpg" {
			f.exec(t, `UPDATE cloud_files SET state='missing' WHERE id=$1`, other.file)
		}
		expected := nativelibrary.ImageTag(path)
		before := f.trace.stills.Load()
		items, total, err := f.repo.Query(f.ctx, f.user, f.permissions(), nativelibrary.ItemQuery{IDs: []string{id}, Limit: 200})
		if err != nil || total != 1 || len(items) != 1 || items[0].ImageTags["Primary"] != expected || f.trace.stills.Load()-before != 1 {
			t.Fatal("cross-library global tag mismatch", path, total, err)
		}
		episodes, total, err := (catalog.HierarchyRepository{DB: f.pool}).ListEpisodes(f.ctx, nativelibrary.EpisodeQuery{SeriesID: seriesID, Visible: nativelibrary.VisibleLibraries{RequestedID: "g:still-tv", AllowedIDs: []string{"g:still-tv"}}, Limit: 200})
		if err != nil || total != 1 || len(episodes) != 1 || episodes[0].ImageTags["Primary"] != expected {
			t.Fatal("cross-library hierarchy tag mismatch", path, total, err)
		}
		image, err := resolver.ResolveImage(f.ctx, embyapi.ImageRequest{ItemID: id, Type: "Primary", User: &f.user, Permissions: f.permissions()})
		if err != nil || image.Tag != expected {
			t.Fatal("cross-library endpoint tag mismatch", path, err)
		}
	}
}
