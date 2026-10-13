package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPostgresSubscriptionPreviewRSSBaselineAndAutomaticDedup(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("isolated media_tool_test required")
	}
	cleanup := `TRUNCATE subscriptions,integrations,resources,site_definitions,search_runs,download_tasks,outbox_events,jobs,media_items CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), cleanup)
	feedID := "1"
	feedMode := "valid"
	torrent := "d4:infod6:lengthi100e4:name20:Owned.Movie.2026.mkv12:piece lengthi16384e6:pieces20:" + strings.Repeat("a", 20) + "ee"
	// Correct byte length is derived from the owned filename.
	torrent = strings.Replace(torrent, "name20:", fmt.Sprintf("name%d:", len("Owned.Movie.2026.mkv")), 1)
	pt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			io.WriteString(w, `<a href="logout">Logout</a>`)
		case "/torrents.php":
			fmt.Fprintf(w, `<table><tr class="resource"><td><a data-id="1">Owned Movie (2026)</a><a class="download" href="/download.php?id=1">Download</a></td><td class="size">100 B</td><td class="seeds">5</td></tr></table>`)
		case "/rss":
			if feedMode == "login" {
				fmt.Fprint(w, `<html><body>Login required; private-rss must not enter errors</body></html>`)
				return
			}
			if feedMode == "unit3d" {
				fmt.Fprintf(w, `<rss version="2.0"><channel><item><title>Owned Movie (2026)</title><guid>%s</guid><contentlength>100</contentlength><link>http://%s/torrent/download/%s.FIXTURE-PATH-KEY</link></item></channel></rss>`, feedID, r.Host, feedID)
				return
			}
			fmt.Fprintf(w, `<rss><channel><item><title>Owned Movie (2026)</title><guid>stable-%s</guid><link>http://%s/details.php?id=%s</link><enclosure url="http://%s/download.php?id=%s&amp;passkey=private-rss" length="100"/></item></channel></rss>`, feedID, r.Host, feedID, r.Host, feedID)
		case "/download.php":
			w.Header().Set("Content-Type", "application/x-bittorrent")
			io.WriteString(w, torrent)
		default:
			w.WriteHeader(404)
		}
	}))
	defer pt.Close()
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	manager := integrations.NewManager(repo, config.Config{})
	defer manager.Close()
	manager.OutboundHTTP = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":1,"title":"Owned Movie","original_title":"Owned Movie","release_date":"2026-01-01"}`))}, nil
	})}
	save := func(in integrations.Input) integrations.Record {
		t.Helper()
		record, err := repo.Save(ctx, "", in)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	account := save(integrations.Input{Kind: "ptsite", Name: "owned feed account", Enabled: true, Settings: map[string]any{"endpoint": pt.URL, "definitionId": "owned_subscriptions"}, Secrets: map[string]string{"cookie": "own=fixture", "rssUrl": pt.URL + "/rss?passkey=private-rss"}})
	save(integrations.Input{Kind: "tmdb", Name: "owned metadata", Enabled: true, Settings: map[string]any{}, Secrets: map[string]string{"token": "metadata-fixture"}})
	local := t.TempDir()
	save(integrations.Input{Kind: "qbittorrent", Name: "owned downloader", Enabled: true, Settings: map[string]any{"endpoint": pt.URL, "downloadPath": "/downloads", "localPath": local}})
	definition := fmt.Sprintf(`{"templateVersion":1,"id":"owned_subscriptions","name":"Owned","schema":"NexusPHP","domain":%q,"meta":{"has_hr":false},"requests":{"user_basic_info":{"parser":"XPath","method":"GET","path":"/","fields":{"is_login":{"selector":"//a[contains(@href,'logout')]","filters":[{"name":"not_blank"}]}}},"search":{"parser":"CssSelector","method":"GET","path":"/torrents.php","list":{"selector":"tr.resource"},"fields":{"id":{"selector":"a[data-id]","attribute":"data-id"},"title":{"selector":"a[data-id]"},"size":{"selector":"td.size","filters":[{"name":"byte_size"}]},"seeders":{"selector":"td.seeds"},"download":{"selector":"a.download","attribute":"href"},"downloadvolumefactor":{"filters":[{"name":"constant","args":0}]}}}}}`, pt.URL)
	if _, err = db.Exec(ctx, `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition,source_repository)VALUES('owned_subscriptions','Owned','NexusPHP','owned-test',$1,'{}','dingding229/mediatool-site')`, json.RawMessage(definition)); err != nil {
		t.Fatal(err)
	}
	engine := Engine{DB: db, Connections: manager}
	input := Input{MediaType: "movie", TMDBID: 1, Source: "search", IntervalMinutes: 30, Rules: Rules{AccountIDs: []string{account.ID}, Keyword: "Owned Movie", FreeOnly: true, MinSeeders: 1}}
	record, err := engine.Save(ctx, "", input)
	if err != nil {
		t.Fatal(err)
	}
	previewID, err := engine.Queue(ctx, record.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"runId": previewID})
	if err = engine.Process(ctx, jobs.Job{Payload: body, Attempts: 1, MaxAttempts: 5}); err != nil {
		t.Fatal(err)
	}
	run, err := engine.Run(ctx, previewID)
	if err != nil || len(run.Decisions) != 1 || run.Decisions[0].Action != "would_download" {
		t.Fatal(run, err)
	}
	var tasks int
	db.QueryRow(ctx, `SELECT count(*)FROM download_tasks`).Scan(&tasks)
	if tasks != 0 {
		t.Fatal("preview created download")
	}
	input.Enabled = true
	input.Revision = record.Revision
	record, err = engine.Save(ctx, record.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	runID, _ := engine.Queue(ctx, record.ID, false)
	body, _ = json.Marshal(map[string]string{"runId": runID})
	if err = engine.Process(ctx, jobs.Job{Payload: body, Attempts: 1, MaxAttempts: 5}); err != nil {
		t.Fatal(err)
	}
	run, _ = engine.Run(ctx, runID)
	if run.Decisions[0].Action != "download_queued" {
		t.Fatal(run)
	}
	again, _ := engine.Queue(ctx, record.ID, false)
	body, _ = json.Marshal(map[string]string{"runId": again})
	if err = engine.Process(ctx, jobs.Job{Payload: body, Attempts: 1, MaxAttempts: 5}); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(ctx, `SELECT count(*)FROM download_tasks`).Scan(&tasks)
	if tasks != 1 {
		t.Fatal("subscription repeated media download", tasks)
	}
	input.Source = "rss"
	input.Revision = record.Revision
	input.Rules.MinSeeders = 0
	input.Rules.FreeOnly = false
	input.Rules.AllowUnknownHR = true
	record, err = engine.Save(ctx, record.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	// HTTP 200 login XML must fail, not initialize the history-skipping baseline.
	feedMode = "login"
	invalid, queueErr := engine.Queue(ctx, record.ID, false)
	if queueErr != nil {
		t.Fatal(queueErr)
	}
	body, _ = json.Marshal(map[string]string{"runId": invalid})
	if err = engine.Process(ctx, jobs.Job{Payload: body, Attempts: 1, MaxAttempts: 1}); !errors.Is(err, sites.ErrFeed) {
		t.Fatal("login page accepted as empty RSS baseline", err)
	}
	invalidRun, err := engine.Run(ctx, invalid)
	if err != nil || invalidRun.State != "failed" {
		t.Fatal("invalid feed run not failed", err)
	}
	var initializedBefore, seenBefore int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM subscription_rss_accounts WHERE subscription_id=$1),(SELECT count(*) FROM subscription_rss_seen WHERE subscription_id=$1)`, record.ID).Scan(&initializedBefore, &seenBefore); err != nil || initializedBefore != 0 || seenBefore != 0 {
		t.Fatal("invalid feed mutated baseline", initializedBefore, seenBefore, err)
	}
	var safeMessage, siteState string
	if err = db.QueryRow(ctx, `SELECT last_message FROM subscriptions WHERE id=$1`, record.ID).Scan(&safeMessage); err != nil || !strings.Contains(safeMessage, "未更新订阅基线") || strings.Contains(safeMessage, "private-rss") {
		t.Fatal("invalid feed message not actionable/redacted", err)
	}
	if err = db.QueryRow(ctx, `SELECT s.state FROM search_site_results s JOIN subscription_runs r ON s.run_id=r.search_run_id WHERE r.id=$1 AND s.account_id=$2`, invalid, account.ID).Scan(&siteState); err != nil || siteState != "failed" {
		t.Fatal("RSS account result remained pending/succeeded", siteState, err)
	}
	feedMode = "valid"
	first, _ := engine.Queue(ctx, record.ID, false)
	body, _ = json.Marshal(map[string]string{"runId": first})
	if err = engine.Process(ctx, jobs.Job{Payload: body, Attempts: 1, MaxAttempts: 5}); err != nil {
		t.Fatal(err)
	}
	firstRun, _ := engine.Run(ctx, first)
	if len(firstRun.Decisions) != 0 {
		t.Fatal("initial RSS history was downloaded")
	}
	var initialized int
	db.QueryRow(ctx, `SELECT count(*)FROM subscription_rss_accounts WHERE subscription_id=$1`, record.ID).Scan(&initialized)
	if initialized != 1 {
		t.Fatal("RSS baseline not persisted")
	}
	feedID = "2"
	feedMode = "unit3d"
	later, _ := engine.Queue(ctx, record.ID, false)
	body, _ = json.Marshal(map[string]string{"runId": later})
	if err = engine.Process(ctx, jobs.Job{Payload: body, Attempts: 1, MaxAttempts: 5}); err != nil {
		t.Fatal(err)
	}
	laterRun, _ := engine.Run(ctx, later)
	if len(laterRun.Decisions) != 1 || laterRun.Decisions[0].Action != "skipped" {
		t.Fatal("new RSS item not evaluated against media coverage", laterRun)
	}
	var resourceID string
	var public, encrypted []byte
	if err = db.QueryRow(ctx, `SELECT id::text,public_metadata,download_hint_ciphertext FROM resources WHERE account_id=$1 AND torrent_id='2'`, account.ID).Scan(&resourceID, &public, &encrypted); err != nil {
		t.Fatal("path-based feed item missing from resources", err)
	}
	if strings.Contains(string(public), "FIXTURE-PATH-KEY") || strings.Contains(string(public), "/torrent/download/") || strings.Contains(string(encrypted), "FIXTURE-PATH-KEY") {
		t.Fatal("path key stored in public metadata/plaintext")
	}
	hint, err := vault.Open(resourceID, encrypted)
	if err != nil || hint["url"] != pt.URL+"/torrent/download/2.FIXTURE-PATH-KEY" {
		t.Fatal("download path key was lost instead of encrypted", err)
	}
	repeated, err := engine.Queue(ctx, record.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]string{"runId": repeated})
	if err = engine.Process(ctx, jobs.Job{Payload: body, Attempts: 1, MaxAttempts: 5}); err != nil {
		t.Fatal(err)
	}
	repeatedRun, err := engine.Run(ctx, repeated)
	if err != nil || len(repeatedRun.Decisions) != 0 {
		t.Fatal("path-based RSS item evaluated twice", err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM download_tasks`).Scan(&tasks); err != nil || tasks != 1 {
		t.Fatal("RSS created duplicate download tasks", tasks, err)
	}
	// A later login/challenge response must not clear an established baseline,
	// consume unseen history, rewrite download hints or create a task.
	feedMode = "login"
	badAfterBaseline, err := engine.Queue(ctx, record.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]string{"runId": badAfterBaseline})
	if err = engine.Process(ctx, jobs.Job{Payload: body, Attempts: 1, MaxAttempts: 1}); !errors.Is(err, sites.ErrFeed) {
		t.Fatal("later invalid feed did not fail", err)
	}
	var stillInitialized, stillSeen, staged int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM subscription_rss_accounts WHERE subscription_id=$1),(SELECT count(*) FROM subscription_rss_seen WHERE subscription_id=$1),(SELECT count(*) FROM subscription_rss_run_items WHERE run_id=$2)`, record.ID, badAfterBaseline).Scan(&stillInitialized, &stillSeen, &staged); err != nil || stillInitialized != 1 || stillSeen != 2 || staged != 0 {
		t.Fatal("later invalid feed changed baseline", stillInitialized, stillSeen, staged, err)
	}
	var retained []byte
	if err = db.QueryRow(ctx, `SELECT download_hint_ciphertext FROM resources WHERE id=$1`, resourceID).Scan(&retained); err != nil || string(retained) != string(encrypted) {
		t.Fatal("invalid feed rewrote existing keyed hint", err)
	}
}
