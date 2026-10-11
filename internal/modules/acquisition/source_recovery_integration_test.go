package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"media-tool/internal/modules/downloads"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

type sourceFixture struct {
	ctx                       context.Context
	engine                    Engine
	accountID                 string
	runID, resourceID, taskID string
	torrent                   []byte
	hash                      string
	mu                        sync.Mutex
	present                   bool
	unknownAddResponse        bool
	qbStarted                 chan struct{}
	qbRelease                 chan struct{}
	adds                      int
	torrentGets               int
	endpoint                  string
}

func newSourceFixture(t *testing.T) *sourceFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	t.Cleanup(cancel)
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("isolated media_tool_test required", err)
	}
	cleanup := `TRUNCATE subscriptions,integrations,resources,site_definitions,search_runs,download_tasks,outbox_events,jobs,media_items CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(context.Background(), cleanup) })
	f := &sourceFixture{ctx: ctx}
	filename := "Owned.Movie.2026.mkv"
	f.torrent = []byte(fmt.Sprintf("d4:infod6:lengthi100e4:name%d:%s12:piece lengthi16384e6:pieces20:%see", len(filename), filename, strings.Repeat("a", 20)))
	meta, err := downloads.ParseTorrent(f.torrent)
	if err != nil {
		t.Fatal(err)
	}
	f.hash = meta.InfoHash
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/download.php":
			f.torrentGets++
			w.Header().Set("Content-Type", "application/x-bittorrent")
			w.Write(f.torrent)
		case "/api/v2/torrents/info":
			if f.qbStarted != nil {
				select {
				case f.qbStarted <- struct{}{}:
				default:
				}
				select {
				case <-f.qbRelease:
				case <-r.Context().Done():
					return
				}
			}
			if f.present {
				json.NewEncoder(w).Encode([]map[string]any{{"hash": f.hash, "save_path": "/downloads", "name": filename, "progress": 0, "state": "downloading"}})
			} else {
				fmt.Fprint(w, "[]")
			}
		case "/api/v2/torrents/add":
			f.adds++
			f.present = true
			if f.unknownAddResponse {
				w.WriteHeader(503)
				fmt.Fprint(w, "outcome unknown")
				return
			}
			fmt.Fprint(w, "Ok.")
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	f.endpoint = server.URL
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	manager := integrations.NewManager(repo, config.Config{})
	t.Cleanup(func() { manager.Close() })
	manager.OutboundHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"id":%s,"title":"Owned Movie","original_title":"Owned Movie","release_date":"2026-01-01"}`, path.Base(r.URL.Path))))}, nil
	})}
	save := func(input integrations.Input) integrations.Record {
		t.Helper()
		record, err := repo.Save(ctx, "", input)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	account := save(integrations.Input{Kind: "ptsite", Name: "Owned PT", Enabled: true, Settings: map[string]any{"endpoint": server.URL, "definitionId": "owned_source"}})
	f.accountID = account.ID
	save(integrations.Input{Kind: "tmdb", Name: "Owned metadata", Enabled: true, Secrets: map[string]string{"token": "fixture"}})
	save(integrations.Input{Kind: "qbittorrent", Name: "Owned downloader", Enabled: true, Settings: map[string]any{"endpoint": server.URL, "downloadPath": "/downloads", "localPath": t.TempDir()}})
	definition := fmt.Sprintf(`{"id":"owned_source","name":"Owned","schema":"NexusPHP","domain":%q,"requests":{}}`, server.URL)
	if _, err = db.Exec(ctx, `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition)VALUES('owned_source','Owned','NexusPHP','fixture',$1,'{}')`, json.RawMessage(definition)); err != nil {
		t.Fatal(err)
	}
	searcher := search.Engine{DB: db, Vault: vault, Connections: manager}
	f.engine = Engine{DB: db, Connections: manager, Search: searcher}
	f.runID = f.freshRun(t)
	run, err := searcher.Get(ctx, f.runID, 1, 50)
	if err != nil || len(run.Resources) != 1 {
		t.Fatal(err)
	}
	f.resourceID = run.Resources[0].ID
	f.taskID, err = f.engine.StartFromRun(ctx, f.runID, f.resourceID, "movie", 1)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *sourceFixture) freshRun(t *testing.T) string {
	t.Helper()
	runID, err := f.engine.Search.StartInline(f.ctx, "Owned", []string{f.accountID})
	if err != nil {
		t.Fatal(err)
	}
	free, noHR := float64(0), false
	item := sites.Resource{TorrentID: "42", Title: "Owned Movie (2026)", Size: 100, Seeders: 5, DownloadFactor: &free, HR: &noHR, DownloadURL: f.endpoint + "/download.php?id=42&passkey=PRIVATE-FIXTURE-KEY"}
	if err = f.engine.Search.SaveResources(f.ctx, runID, f.accountID, []sites.Resource{item}); err != nil {
		t.Fatal(err)
	}
	return runID
}
func (f *sourceFixture) pauseOldSource(t *testing.T) {
	t.Helper()
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE search_run_resources SET observed_at=now()-interval '11 minutes' WHERE run_id=$1`, f.runID); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"taskId": f.taskID})
	if err := f.engine.Add(f.ctx, jobs.Job{Payload: payload}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal("stale source did not require operator", err)
	}
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE jobs SET state='paused' WHERE idempotency_key=$1`, "download.add:"+f.taskID); err != nil {
		t.Fatal(err)
	}
}
func sourceApproval(preview SourcePreview) SourceReconfirmInput {
	return SourceReconfirmInput{SearchRunID: preview.SearchRunID, ControlRevision: preview.ControlRevision, PlanDigest: preview.PlanDigest, ConfirmSourceBinding: true}
}

func TestPostgresSourcePreviewApprovalReplaysOriginalTaskWithoutDuplicateAdd(t *testing.T) {
	f := newSourceFixture(t)
	f.pauseOldSource(t)
	newRun := f.freshRun(t)
	preview, err := f.engine.SourcePreview(f.ctx, f.taskID, newRun)
	if err != nil || !preview.Eligible || preview.Mode != "same_identity" || preview.CandidateHash != f.hash {
		t.Fatal("valid same-hash preview unavailable", preview, err)
	}
	raw, _ := json.Marshal(preview)
	if strings.Contains(string(raw), "PRIVATE-FIXTURE-KEY") || strings.Contains(string(raw), "/download.php") {
		t.Fatal("private hint in source preview")
	}
	var beforeState string
	if err = f.engine.DB.QueryRow(f.ctx, `SELECT state FROM jobs WHERE idempotency_key=$1`, "download.add:"+f.taskID).Scan(&beforeState); err != nil || beforeState != "paused" {
		t.Fatal("preview queued a job", err)
	}
	accepted, err := f.engine.ReconfirmSource(f.ctx, f.taskID, sourceApproval(preview))
	if err != nil || accepted.AlreadyApplied {
		t.Fatal("approval failed", err)
	}
	duplicate, err := f.engine.ReconfirmSource(f.ctx, f.taskID, sourceApproval(preview))
	if err != nil || !duplicate.AlreadyApplied {
		t.Fatal("approval replay not idempotent", err)
	}
	payload, _ := json.Marshal(map[string]string{"taskId": f.taskID})
	if err = f.engine.Add(f.ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if err = f.engine.Add(f.ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	adds := f.adds
	f.mu.Unlock()
	if adds != 1 {
		t.Fatal("approval/replay created multiple qB torrents", adds)
	}
	var tasks, jobsCount int
	f.engine.DB.QueryRow(f.ctx, `SELECT count(*)FROM download_tasks`).Scan(&tasks)
	f.engine.DB.QueryRow(f.ctx, `SELECT count(*)FROM jobs WHERE kind='download.add'`).Scan(&jobsCount)
	if tasks != 1 || jobsCount != 1 {
		t.Fatal("new task/job created instead of original recovery", tasks, jobsCount)
	}
}

func TestPostgresSourceApprovalRejectsChangedHashStaleProofAndActiveLease(t *testing.T) {
	f := newSourceFixture(t)
	f.pauseOldSource(t)
	newRun := f.freshRun(t)
	preview, err := f.engine.SourcePreview(f.ctx, f.taskID, newRun)
	if err != nil || !preview.Eligible {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.torrent = []byte(strings.Replace(string(f.torrent), strings.Repeat("a", 20), strings.Repeat("b", 20), 1))
	f.mu.Unlock()
	changed, err := f.engine.SourcePreview(f.ctx, f.taskID, newRun)
	if err != nil || changed.Eligible || len(changed.Reasons) == 0 {
		t.Fatal("changed torrent identity accepted", err)
	}
	if _, err = f.engine.ReconfirmSource(f.ctx, f.taskID, sourceApproval(preview)); !errors.Is(err, ErrSourceRecoveryConflict) {
		t.Fatal("old proof accepted changed torrent", err)
	}
	f.mu.Lock()
	f.torrent = []byte(strings.Replace(string(f.torrent), strings.Repeat("b", 20), strings.Repeat("a", 20), 1))
	f.mu.Unlock()
	if _, err = f.engine.DB.Exec(f.ctx, `UPDATE jobs SET state='running',lease_token=gen_random_uuid(),lease_until=now()+interval '1 minute',locked_by='fixture' WHERE idempotency_key=$1`, "download.add:"+f.taskID); err != nil {
		t.Fatal(err)
	}
	active, err := f.engine.SourcePreview(f.ctx, f.taskID, newRun)
	if err != nil || active.Eligible {
		t.Fatal("active lease can be rebound", err)
	}
}

func TestPostgresDuplicateTaskDoesNotChangeMediaHashOrPausedSource(t *testing.T) {
	f := newSourceFixture(t)
	if _, err := f.engine.StartExpectedFromRun(f.ctx, f.runID, f.resourceID, "movie", 2, f.hash, nil, nil, "", 0); !errors.Is(err, ErrDownloadIdentityConflict) {
		t.Fatal("different media silently reused task", err)
	}
	if _, err := f.engine.StartExpectedFromRun(f.ctx, f.runID, f.resourceID, "movie", 1, strings.Repeat("f", 40), nil, nil, "", 0); !errors.Is(err, ErrDownloadIdentityConflict) {
		t.Fatal("different expected hash silently reused task", err)
	}
	f.pauseOldSource(t)
	newRun := f.freshRun(t)
	if _, err := f.engine.StartFromRun(f.ctx, newRun, f.resourceID, "movie", 1); !errors.Is(err, ErrSourceRecoveryRequired) {
		t.Fatal("duplicate creation bypassed source confirmation", err)
	}
}

func TestPostgresInitialSourceConfirmationAndReplayScopeAreExplicit(t *testing.T) {
	f := newSourceFixture(t)
	f.pauseOldSource(t)
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE download_tasks SET details=details-'expectedHash' WHERE id=$1`, f.taskID); err != nil {
		t.Fatal(err)
	}
	run := f.freshRun(t)
	preview, err := f.engine.SourcePreview(f.ctx, f.taskID, run)
	if err != nil || !preview.Eligible || preview.Mode != "initial_identity" {
		t.Fatal("initial identity preview unavailable", err)
	}
	input := sourceApproval(preview)
	if _, err = f.engine.ReconfirmSource(f.ctx, f.taskID, input); !errors.Is(err, ErrSourceRecoveryConflict) {
		t.Fatal("initial identity accepted without its extra consent", err)
	}
	input.ConfirmInitialIdentity = true
	if _, err = f.engine.ReconfirmSource(f.ctx, f.taskID, input); err != nil {
		t.Fatal(err)
	}
	input.SearchRunID = f.runID
	if _, err = f.engine.ReconfirmSource(f.ctx, f.taskID, input); !errors.Is(err, ErrSourceRecoveryConflict) {
		t.Fatal("receipt claimed a different run binding", err)
	}
}

func TestPostgresSourcePreviewRejectsChangedSubscriptionAndExpiredNewSnapshot(t *testing.T) {
	f := newSourceFixture(t)
	f.pauseOldSource(t)
	run := f.freshRun(t)
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE search_run_resources SET observed_at=now()-interval '11 minutes' WHERE run_id=$1`, run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.engine.SourcePreview(f.ctx, f.taskID, run); !errors.Is(err, search.ErrSnapshotExpired) {
		t.Fatal("expired replacement source accepted", err)
	}
	run = f.freshRun(t)
	subscription, subRun := jobs.UUID(), jobs.UUID()
	identity := `{"id":1,"mediaType":"movie","title":"Owned Movie","originalTitle":"Owned Movie","year":"2026"}`
	rules := fmt.Sprintf(`{"accountIds":[%q],"keyword":"Owned Movie","freeOnly":true}`, f.accountID)
	if _, err := f.engine.DB.Exec(f.ctx, `INSERT INTO subscriptions(id,title,media_type,identity,rules,enabled)VALUES($1,'Owned','movie',$2,$3,false)`, subscription, json.RawMessage(identity), json.RawMessage(rules)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.engine.DB.Exec(f.ctx, `INSERT INTO subscription_runs(id,subscription_id,revision,search_run_id)VALUES($1,$2,1,$3)`, subRun, subscription, run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE download_tasks SET details=jsonb_set(details,'{subscriptionRunId}',to_jsonb($2::text)) WHERE id=$1`, f.taskID, subRun); err != nil {
		t.Fatal(err)
	}
	preview, err := f.engine.SourcePreview(f.ctx, f.taskID, run)
	if err != nil || preview.Eligible {
		t.Fatal("disabled subscription silently resumed", err)
	}
}

func TestPostgresConcurrentSourceApprovalDoesNotResetOriginalJobTwice(t *testing.T) {
	f := newSourceFixture(t)
	f.pauseOldSource(t)
	run := f.freshRun(t)
	preview, err := f.engine.SourcePreview(f.ctx, f.taskID, run)
	if err != nil || !preview.Eligible {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan SourceReconfirmResult, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			result, err := f.engine.ReconfirmSource(f.ctx, f.taskID, sourceApproval(preview))
			results <- result
			failures <- err
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-failures; err != nil {
			t.Fatal("concurrent same approval should return an applied receipt", err)
		}
	}
	first, second := <-results, <-results
	if first.AlreadyApplied == second.AlreadyApplied {
		t.Fatal("exactly one approval should be the mutating request")
	}
	var revision int64
	var jobsCount int
	if err = f.engine.DB.QueryRow(f.ctx, `SELECT source_control_revision FROM download_tasks WHERE id=$1`, f.taskID).Scan(&revision); err != nil || revision != preview.ControlRevision+1 {
		t.Fatal("approval mutation repeated", revision, err)
	}
	f.engine.DB.QueryRow(f.ctx, `SELECT count(*) FROM jobs WHERE kind='download.add'`).Scan(&jobsCount)
	if jobsCount != 1 {
		t.Fatal("concurrent approval created jobs", jobsCount)
	}
}

func TestPostgresUnknownAddResponseReconcilesKnownHashWithoutSecondSubmission(t *testing.T) {
	f := newSourceFixture(t)
	f.mu.Lock()
	f.unknownAddResponse = true
	f.mu.Unlock()
	payload, _ := json.Marshal(map[string]string{"taskId": f.taskID})
	if err := f.engine.Add(f.ctx, jobs.Job{Payload: payload}); err == nil {
		t.Fatal("unknown response reported as a confirmed add")
	}
	var hash, state string
	if err := f.engine.DB.QueryRow(f.ctx, `SELECT coalesce(info_hash,''),download_state FROM download_tasks WHERE id=$1`, f.taskID).Scan(&hash, &state); err != nil || hash != f.hash || state != "pending" {
		t.Fatal("identity was not pinned before uncertain submission", hash, state, err)
	}
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE search_run_resources SET observed_at=now()-interval '11 minutes' WHERE run_id=$1`, f.runID); err != nil {
		t.Fatal(err)
	}
	if err := f.engine.Add(f.ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal("known downloaded hash should reconcile without an expired-source fetch", err)
	}
	f.mu.Lock()
	adds := f.adds
	f.mu.Unlock()
	if adds != 1 {
		t.Fatal("unknown outcome caused duplicate remote add", adds)
	}
	if err := f.engine.DB.QueryRow(f.ctx, `SELECT download_state FROM download_tasks WHERE id=$1`, f.taskID).Scan(&state); err != nil || state != "downloading" {
		t.Fatal("remote success not reconciled", state, err)
	}
}

func TestPostgresSourceConfigurationEditDuringPreviewCannotApproveOldEvidence(t *testing.T) {
	f := newSourceFixture(t)
	f.pauseOldSource(t)
	run := f.freshRun(t)
	f.mu.Lock()
	f.qbStarted = make(chan struct{}, 1)
	f.qbRelease = make(chan struct{})
	started, release := f.qbStarted, f.qbRelease
	f.mu.Unlock()
	result := make(chan error, 1)
	go func() { _, err := f.engine.SourcePreview(f.ctx, f.taskID, run); result <- err }()
	select {
	case <-started:
	case <-f.ctx.Done():
		t.Fatal("qB observation did not start")
	}
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, f.accountID); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-result; !errors.Is(err, search.ErrSnapshotChanged) {
		t.Fatal("preview ignored a concurrent source revision change", err)
	}
	var required bool
	if err := f.engine.DB.QueryRow(f.ctx, `SELECT source_recovery_required FROM download_tasks WHERE id=$1`, f.taskID).Scan(&required); err != nil || !required {
		t.Fatal("preview wrote recovery state", err)
	}
}

func TestPostgresOldAcceptedReceiptDoesNotResumeANewerPausedGeneration(t *testing.T) {
	f := newSourceFixture(t)
	f.pauseOldSource(t)
	run := f.freshRun(t)
	preview, err := f.engine.SourcePreview(f.ctx, f.taskID, run)
	if err != nil || !preview.Eligible {
		t.Fatal(err)
	}
	input := sourceApproval(preview)
	if _, err = f.engine.ReconfirmSource(f.ctx, f.taskID, input); err != nil {
		t.Fatal(err)
	}
	if _, err = f.engine.DB.Exec(f.ctx, `UPDATE search_run_resources SET observed_at=now()-interval '11 minutes' WHERE run_id=$1`, run); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"taskId": f.taskID})
	if err = f.engine.Add(f.ctx, jobs.Job{Payload: payload}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal("new generation did not pause", err)
	}
	if _, err = f.engine.DB.Exec(f.ctx, `UPDATE jobs SET state='paused',attempts=3 WHERE idempotency_key=$1`, "download.add:"+f.taskID); err != nil {
		t.Fatal(err)
	}
	receipt, err := f.engine.ReconfirmSource(f.ctx, f.taskID, input)
	if err != nil || !receipt.AlreadyApplied || !receipt.RecoveryStillRequired {
		t.Fatal("old receipt reset the newer source pause", receipt, err)
	}
	var state string
	var attempts int
	if err = f.engine.DB.QueryRow(f.ctx, `SELECT state,attempts FROM jobs WHERE idempotency_key=$1`, "download.add:"+f.taskID).Scan(&state, &attempts); err != nil || state != "paused" || attempts != 3 {
		t.Fatal("old approval replay reset queue work", state, attempts, err)
	}
}

func TestPostgresSameRunHintRewriteDuringPreviewCannotBeMixedWithOldTorrentBytes(t *testing.T) {
	f := newSourceFixture(t)
	f.pauseOldSource(t)
	run := f.freshRun(t)
	f.mu.Lock()
	f.qbStarted = make(chan struct{}, 1)
	f.qbRelease = make(chan struct{})
	started, release := f.qbStarted, f.qbRelease
	f.mu.Unlock()
	result := make(chan error, 1)
	go func() { _, err := f.engine.SourcePreview(f.ctx, f.taskID, run); result <- err }()
	select {
	case <-started:
	case <-f.ctx.Done():
		t.Fatal("qB observation did not start")
	}
	item := sites.Resource{TorrentID: "42", Title: "Owned Movie (2026)", Size: 100, Seeders: 5, DownloadURL: f.endpoint + "/download.php?id=42&passkey=CHANGED-FIXTURE-KEY"}
	if err := f.engine.Search.SaveResources(f.ctx, run, f.accountID, []sites.Resource{item}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-result; !errors.Is(err, ErrSourceRecoveryConflict) {
		t.Fatal("old file bytes approved with rewritten same-run hint", err)
	}
}

func TestPostgresUnreadableTaskDetailsAreNotOverwrittenByFailureReporting(t *testing.T) {
	f := newSourceFixture(t)
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE download_tasks SET details='[]'::jsonb WHERE id=$1`, f.taskID); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"taskId": f.taskID})
	if err := f.engine.Add(f.ctx, jobs.Job{Payload: payload}); err == nil {
		t.Fatal("invalid task details reported success")
	}
	var raw []byte
	if err := f.engine.DB.QueryRow(f.ctx, `SELECT details FROM download_tasks WHERE id=$1`, f.taskID).Scan(&raw); err != nil || string(raw) != "[]" {
		t.Fatal("failure reporting replaced unreadable original data", string(raw), err)
	}
}

func TestPostgresSelectedDownloadProofRejectsSameRunReplacementBeforeAdd(t *testing.T) {
	f := newSourceFixture(t)
	var raw []byte
	if err := f.engine.DB.QueryRow(f.ctx, `SELECT details FROM download_tasks WHERE id=$1`, f.taskID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var detail taskDetails
	if json.Unmarshal(raw, &detail) != nil || detail.SourceSnapshotDigest == "" {
		t.Fatal("new task did not pin exact source evidence")
	}
	// Supported writers cannot re-open a consumed search run.
	item := sites.Resource{TorrentID: "42", Title: "Owned", DownloadURL: f.endpoint + "/download.php?id=42&passkey=DIFFERENT-KEY"}
	if err := f.engine.Search.SaveResources(f.ctx, f.runID, f.accountID, []sites.Resource{item}); !errors.Is(err, search.ErrSnapshotChanged) {
		t.Fatal("consumed run allowed mutable replacement", err)
	}
	// Even an out-of-band metadata change does not silently become this task's
	// prechecked result. There is no qB submission and the dedicated gate is set.
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE search_run_resources SET public_snapshot=jsonb_set(public_snapshot,'{title}','"Changed after selection"') WHERE run_id=$1`, f.runID); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"taskId": f.taskID})
	if err := f.engine.Add(f.ctx, jobs.Job{Payload: payload}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal("changed selected proof was submitted", err)
	}
	f.mu.Lock()
	adds := f.adds
	f.mu.Unlock()
	if adds != 0 {
		t.Fatal("changed proof caused add", adds)
	}
}

func TestPostgresLegacyRunTaskWithoutDigestRequiresExplicitSourceApproval(t *testing.T) {
	f := newSourceFixture(t)
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE download_tasks SET details=details-'sourceSnapshotDigest' WHERE id=$1`, f.taskID); err != nil {
		t.Fatal(err)
	}
	var originalJob string
	var originalControl int64
	if err := f.engine.DB.QueryRow(f.ctx, `SELECT j.id::text,d.source_control_revision FROM jobs j JOIN download_tasks d ON j.idempotency_key='download.add:'||d.id::text WHERE d.id=$1`, f.taskID).Scan(&originalJob, &originalControl); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	beforeFetches := f.torrentGets
	f.mu.Unlock()
	payload, _ := json.Marshal(map[string]string{"taskId": f.taskID})
	for i := 0; i < 2; i++ {
		if err := f.engine.Add(f.ctx, jobs.Job{Payload: payload}); !errors.Is(err, jobs.ErrOperatorRequired) {
			t.Fatal("legacy run-bound task fetched a source without explicit approval", err)
		}
	}
	f.mu.Lock()
	fetches, adds := f.torrentGets, f.adds
	f.mu.Unlock()
	if fetches != beforeFetches || adds != 0 {
		t.Fatal("legacy missing proof performed a network fetch or add", fetches-beforeFetches, adds)
	}
	var required bool
	var control int64
	if err := f.engine.DB.QueryRow(f.ctx, `SELECT source_recovery_required,source_control_revision FROM download_tasks WHERE id=$1`, f.taskID).Scan(&required, &control); err != nil || !required || control != originalControl+1 {
		t.Fatal("legacy source gate/retry revision wrong", required, control, err)
	}
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE jobs SET state='paused' WHERE id=$1`, originalJob); err != nil {
		t.Fatal(err)
	}
	fresh := f.freshRun(t)
	preview, err := f.engine.SourcePreview(f.ctx, f.taskID, fresh)
	if err != nil || !preview.Eligible || preview.CandidateHash != f.hash {
		t.Fatal("legacy task lacks safe same-resource preview", preview.Eligible, err)
	}
	approval := sourceApproval(preview)
	if preview.Mode == "initial_identity" {
		approval.ConfirmInitialIdentity = true
	}
	accepted, err := f.engine.ReconfirmSource(f.ctx, f.taskID, approval)
	if err != nil || accepted.AlreadyApplied || accepted.RecoveryStillRequired {
		t.Fatal("legacy source confirmation failed", err)
	}
	duplicate, err := f.engine.ReconfirmSource(f.ctx, f.taskID, approval)
	if err != nil || !duplicate.AlreadyApplied {
		t.Fatal("legacy approval receipt not idempotent", err)
	}
	var raw []byte
	if err = f.engine.DB.QueryRow(f.ctx, `SELECT details FROM download_tasks WHERE id=$1`, f.taskID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var details taskDetails
	if err = json.Unmarshal(raw, &details); err != nil {
		t.Fatal(err)
	}
	digest, err := f.engine.Search.SourceSnapshotDigest(f.ctx, fresh, f.resourceID)
	if err != nil || details.SearchRunID != fresh || details.SourceSnapshotDigest != digest {
		t.Fatal("legacy confirmation did not bind exact fresh proof", err)
	}
	for i := 0; i < 2; i++ {
		if err = f.engine.Add(f.ctx, jobs.Job{Payload: payload}); err != nil {
			t.Fatal("approved legacy task did not resume", err)
		}
	}
	f.mu.Lock()
	adds = f.adds
	f.mu.Unlock()
	var tasks, jobsCount int
	var retainedJob string
	if err = f.engine.DB.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM download_tasks),(SELECT count(*) FROM jobs WHERE kind='download.add'),id::text FROM jobs WHERE idempotency_key=$1`, "download.add:"+f.taskID).Scan(&tasks, &jobsCount, &retainedJob); err != nil || tasks != 1 || jobsCount != 1 || retainedJob != originalJob || adds != 1 {
		t.Fatal("legacy recovery replaced original task/job or duplicated add", tasks, jobsCount, adds, err)
	}
}

func TestPostgresLegacyDigestlessKnownHashReconcilesWithoutFreshSource(t *testing.T) {
	f := newSourceFixture(t)
	f.mu.Lock()
	f.unknownAddResponse = true
	f.mu.Unlock()
	payload, _ := json.Marshal(map[string]string{"taskId": f.taskID})
	if err := f.engine.Add(f.ctx, jobs.Job{Payload: payload}); err == nil {
		t.Fatal("uncertain remote response was reported as confirmed")
	}
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE download_tasks SET details=details-'sourceSnapshotDigest' WHERE id=$1`, f.taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.engine.DB.Exec(f.ctx, `UPDATE search_run_resources SET observed_at=now()-interval '11 minutes' WHERE run_id=$1`, f.runID); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	beforeFetches := f.torrentGets
	f.mu.Unlock()
	if err := f.engine.Add(f.ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal("legacy known remote identity should reconcile without source recovery", err)
	}
	f.mu.Lock()
	fetches, adds := f.torrentGets, f.adds
	f.mu.Unlock()
	if fetches != beforeFetches || adds != 1 {
		t.Fatal("legacy known hash refetched expired source or repeated add", fetches-beforeFetches, adds)
	}
	var state string
	var required bool
	if err := f.engine.DB.QueryRow(f.ctx, `SELECT download_state,source_recovery_required FROM download_tasks WHERE id=$1`, f.taskID).Scan(&state, &required); err != nil || state != "downloading" || required {
		t.Fatal("legacy receipt was not reconciled", state, required, err)
	}
}
