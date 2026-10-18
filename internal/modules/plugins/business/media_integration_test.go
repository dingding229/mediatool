package business

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"github.com/jackc/pgx/v5"
	"io"
	"media-tool/internal/modules/filemanager"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/storage"
	dbutil "media-tool/internal/platform/database"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func mediaFixture(t *testing.T) (AcquisitionHost, context.Context) {
	t.Helper()
	h, ctx := hostFixture(t)
	tables := []string{"storage_connections", "cloud_files", "media_assets", "media_items", "media_asset_bindings", "strm_entries", "playback_grants", "managed_media_artifacts", "media_artifact_journal", "media_archive_policy", "media_archive_cleanup_requests", "media_archive_cleanup_operations", "directory_classification", "native_server_settings", "media_user_legacy_grants", "media_user_library_grants", "native_item_ids", "native_libraries", "native_asset_libraries", "native_tmdb_details", "library_candidates", "library_hidden_history", "library_emby_checks", "library_scans", "library_scan_seen", "library_scan_directories", "file_operations", "file_batches", "file_batch_items", "file_batch_parents", "strm_repair_batches", "strm_repair_items"}
	for _, table := range tables {
		if _, e := h.DB.Exec(ctx, `CREATE TABLE `+pgx.Identifier{table}.Sanitize()+`(LIKE public.`+pgx.Identifier{table}.Sanitize()+` INCLUDING ALL)`); e != nil {
			t.Fatal(table, e)
		}
	}
	for _, sql := range []string{`INSERT INTO directory_classification(singleton)VALUES(true)`, `INSERT INTO media_archive_policy(singleton)VALUES(true)`, `INSERT INTO native_server_settings(server_id)VALUES('00000000000000000000000000000000')`} {
		if _, e := h.DB.Exec(ctx, sql); e != nil {
			t.Fatal(e)
		}
	}
	h.Connections.Config.STRMOutputDir = t.TempDir()
	h.Connections.Config.PublicURL = "https://stable.fixture"
	return h, ctx
}
func requireMedia[T any](t *testing.T, h AcquisitionHost, method string, in any, rev ...int64) T {
	t.Helper()
	v, e := mediaCall(t, h, method, in, rev...)
	if e != nil {
		t.Fatalf("%s: %v", method, e)
	}
	return copyDTO[T](v)
}
func TestMediaFileFacadePreviewApplyBatchBoundsSelectorsAndEscape(t *testing.T) {
	h, ctx := mediaFixture(t)
	root := t.TempDir()
	outside := t.TempDir()
	if e := os.WriteFile(filepath.Join(outside, "secret"), []byte("SECRET"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(outside, filepath.Join(root, "escape")); e != nil {
		t.Fatal(e)
	}
	r, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "local", Name: "workspace", Enabled: true, Settings: map[string]any{"rootPath": root}})
	if e != nil {
		t.Fatal(e)
	}
	page := b.PageRequest{Page: 1, Size: 200}
	for n, prefix := range []string{"fs.media", "storage"} {
		t.Run(prefix, func(t *testing.T) {
			roots := requireMedia[b.Page[b.Root]](t, h, prefix+".roots", page)
			if len(roots.Items) != 1 || roots.Items[0].ID != r.ID {
				t.Fatal(roots)
			}
			requireMedia[b.MediaFile](t, h, prefix+".stat", b.FileQuery{RootID: r.ID, Path: ".", PageRequest: b.PageRequest{}})
			requireMedia[b.FilePage](t, h, prefix+".list", b.FileQuery{RootID: r.ID, Path: ".", PageRequest: page})
			for _, p := range []string{"../secret", "escape/secret", "/etc/passwd", "a/../../secret", "escape"} {
				if _, e := mediaCall(t, h, prefix+".stat", b.FileQuery{RootID: r.ID, Path: p, PageRequest: page}); e == nil {
					t.Fatal("escape accepted", p)
				}
			}
			v := b.FileInput{RootID: r.ID, Operation: "mkdir", Destination: ".", Name: fmt.Sprintf("Folder%d", n)}
			preview := prefix + ".preview"
			if prefix == "storage" {
				preview = "storage.mkdir"
			}
			plan := requireMedia[b.FilePlan](t, h, preview, v)
			if len(plan.Targets) == 0 {
				t.Fatal("missing confirmation targets")
			}
			requireMedia[b.FilePlan](t, h, prefix+".get", b.ID{ID: plan.ID})
			if _, e := mediaCall(t, h, prefix+".apply", b.Approval{ID: plan.ID, Digest: strings.Repeat("a", 64)}, plan.Revision); e == nil {
				t.Fatal("wrong evidence")
			}
			requireMedia[b.FilePlan](t, h, prefix+".apply", b.Approval{ID: plan.ID, Digest: plan.Digest}, plan.Revision)
			name := fmt.Sprintf("Source%d.mkv", n)
			if e := os.WriteFile(filepath.Join(root, name), []byte("owned-media"), 0600); e != nil {
				t.Fatal(e)
			}
			f := requireMedia[b.MediaFile](t, h, prefix+".stat", b.FileQuery{RootID: r.ID, Path: name, PageRequest: page})
			input := b.FileInput{RootID: r.ID, Operation: "move", Source: name, Destination: v.Name, ExpectedFileID: f.ID, ExpectedSize: &f.Size}
			preview = prefix + ".preview"
			if prefix == "storage" {
				preview = "storage.relocate"
			}
			plan = requireMedia[b.FilePlan](t, h, preview, input)
			raw, _ := json.Marshal(input)
			spec, _ := b.Lookup(preview)
			_, e := h.Call(ctx, contracts.HostCall{Method: preview, Args: raw}, contracts.Permission{Scope: spec.Scope, Resources: map[string][]string{"rootIds": {jobs.UUID()}}}, func() error { return nil })
			mediaCode(t, e, contracts.PermissionDenied)
			requireMedia[b.FilePlan](t, h, prefix+".apply", b.Approval{ID: plan.ID, Digest: plan.Digest}, plan.Revision)
			inputs := b.FileInputs{Items: []b.FileInput{{RootID: r.ID, Operation: "mkdir", Name: fmt.Sprintf("Batch%d", n), Destination: "."}}}
			batch := requireMedia[b.FileBatch](t, h, prefix+".previewBatch", inputs)
			requireMedia[b.FileBatch](t, h, prefix+".approveBatch", b.Approval{ID: batch.ID, Digest: batch.Digest})
			payload, _ := json.Marshal(map[string]string{"batchId": batch.ID})
			if e := (filemanager.Engine{DB: h.DB, Connections: h.Connections}).ProcessBatch(ctx, jobs.Job{Payload: payload}); e != nil {
				t.Fatal(e)
			}
		})
	}
	requireMedia[b.FilePlan](t, h, "storage.preview", b.FileInput{RootID: r.ID, Operation: "mkdir", Destination: ".", Name: "IndependentPreview"})
	// A revoked write must roll back the native preview, and denied effects must
	// never rename bytes even when preview evidence is still current.
	v := b.FileInput{RootID: r.ID, Operation: "mkdir", Name: "Denied", Destination: "."}
	raw, _ := json.Marshal(v)
	denied := dbutil.WithCommitGuard(ctx, func(context.Context, pgx.Tx) error { return contracts.NewError(contracts.PermissionDenied) })
	_, e = h.Call(denied, contracts.HostCall{Method: "fs.media.preview", Args: raw}, contracts.Permission{Scope: "fs.media:write"}, func() error { return nil })
	if e == nil {
		t.Fatal("revoked native commit succeeded")
	}
	var count int
	if e = h.DB.QueryRow(ctx, `SELECT count(*)FROM file_operations WHERE plan->'steps'->0->>'target'='Denied'`).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
	plan := requireMedia[b.FilePlan](t, h, "fs.media.preview", v)
	denied = dbutil.WithEffectGuard(ctx, func() error { return contracts.NewError(contracts.PermissionDenied) })
	result, e := (filemanager.Engine{DB: h.DB, Connections: h.Connections}).Execute(denied, plan.ID, plan.Digest, true)
	if e == nil && result.State == "succeeded" {
		t.Fatal("revoked filesystem effect")
	}
	if _, e = os.Stat(filepath.Join(root, "Denied")); !os.IsNotExist(e) {
		t.Fatal("unexpected effect", e)
	}
}

type mediaTransport func(*http.Request) (*http.Response, error)

func (f mediaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func seedMedia(t *testing.T, h *AcquisitionHost, ctx context.Context) (string, string) {
	t.Helper()
	conn, file, asset, candidate := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO storage_connections(id,name,provider)VALUES($1,'fixture','clouddrive2')`, []any{conn}},
		{`INSERT INTO cloud_files(id,storage_id,full_path,provider_file_id,size_bytes)VALUES($1,$2,'/115/Media/Owned.2026.mkv','file-1',4)`, []any{file, conn}},
		{`INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, []any{asset, file}},
		{`INSERT INTO library_candidates(id,file_id,asset_id)VALUES($1,$2,$3)`, []any{candidate, file, asset}},
		{`INSERT INTO media_items(id,media_type,tmdb_id,title,release_year,metadata)VALUES($1,'movie',42,'Owned','2026','{}')`, []any{jobs.UUID()}},
		{`INSERT INTO media_asset_bindings(asset_id,media_id)SELECT $1,id FROM media_items WHERE tmdb_id=42`, []any{asset}},
	} {
		if _, e := h.DB.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	// Same safe native identity observer as library's existing integration tests.
	h.Observe = func(context.Context, string, string) (storage.File, error) {
		return storage.File{ID: "file-1", Path: "/115/Media/Owned.2026.mkv", Name: "Owned.2026.mkv", Size: 4, Hashes: map[uint32]string{}}, nil
	}
	_, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "metadata", Enabled: true, Secrets: map[string]string{"token": "NEVER_EXPOSE_TOKEN"}})
	if e != nil {
		t.Fatal(e)
	}
	h.Connections.OutboundHTTP = &http.Client{Transport: mediaTransport(func(r *http.Request) (*http.Response, error) {
		var value any
		id, _ := strconv.Atoi(path.Base(r.URL.Path))
		if id == 0 {
			id = 42
		}
		details := map[string]any{"id": id, "title": "Owned", "original_title": "Owned", "release_date": "2026-01-01", "overview": "owned plot", "genres": []any{}, "images": map[string]any{}, "credits": map[string]any{}}
		if strings.Contains(r.URL.Path, "search") {
			value = map[string]any{"results": []any{details}}
		} else if strings.HasSuffix(r.URL.Path, "/images") || strings.HasSuffix(r.URL.Path, "/credits") || strings.HasSuffix(r.URL.Path, "/release_dates") {
			value = map[string]any{}
		} else {
			value = details
		}
		raw, _ := json.Marshal(value)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	return asset, candidate
}
func TestMediaLibrarySnapshotsRepairAndSTRMImmutability(t *testing.T) {
	h, ctx := mediaFixture(t)
	asset, candidate := seedMedia(t, &h, ctx)
	le := library.Engine{DB: h.DB, Connections: h.Connections}
	root := h.Connections.Config.STRMOutputDir
	tx, e := h.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	link, key, e := (playbackgrants.Service{DB: h.DB, Vault: h.Vault, PublicURL: h.Connections.Config.PublicURL}).Prepare(ctx, tx, asset, "/115/Media/Owned.2026.mkv")
	if e != nil {
		t.Fatal(e)
	}
	output := "Movies/Owned/Owned.strm"
	if _, e = tx.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,$4)`, jobs.UUID(), asset, output, playbackgrants.SecretFreeKeyRef(key)); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	body := []byte(link + "\n")
	journal := library.Journal{DB: h.DB, Vault: h.Vault}
	if e = journal.Write(ctx, root, output, asset, "asset:"+asset, "strm", body); e != nil {
		t.Fatal(e)
	}
	before := sha256.Sum256(body)
	var grantBefore string
	if e = h.DB.QueryRow(ctx, `SELECT md5(to_jsonb(g)::text)FROM playback_grants g WHERE asset_id=$1`, asset).Scan(&grantBefore); e != nil {
		t.Fatal(e)
	}
	page := b.PageRequest{Page: 1, Size: 200}
	for _, method := range []string{"library.assets", "strm.entries", "library.scans"} {
		requireMedia[json.RawMessage](t, h, method, page)
	}
	requireMedia[json.RawMessage](t, h, "library.archive.list", b.PageRequest{Page: 1, Size: 100})
	for _, method := range []string{"library.candidates", "library.history"} {
		requireMedia[json.RawMessage](t, h, method, b.LibraryPage{PageRequest: page})
	}
	for _, method := range []string{"library.recognize", "library.organize.preview", "library.suggest"} {
		requireMedia[json.RawMessage](t, h, method, b.ID{ID: candidate})
	}
	requireMedia[b.Receipt](t, h, "library.refresh", b.ID{ID: asset})
	requireMedia[b.Receipt](t, h, "metadata.refresh", b.ID{ID: asset})
	requireMedia[[]b.Media](t, h, "metadata.search", b.MetadataQuery{MediaType: "movie", Query: "Owned"})
	requireMedia[b.Media](t, h, "metadata.details", b.MetadataQuery{MediaType: "movie", ID: 42})
	if _, e = h.DB.Exec(ctx, `UPDATE native_tmdb_details SET payload='{"name":"Owned","overview":"token=NEVER_EXPOSE_TOKEN"}',state='ready',fetched_at=now()WHERE tmdb_id=42`); e != nil {
		t.Fatal(e)
	}
	stored := requireMedia[b.StoredMetadata](t, h, "metadata.storedDetails", b.MetadataQuery{MediaType: "movie", ID: 42})
	if strings.Contains(stored.Overview, "NEVER_EXPOSE_TOKEN") {
		t.Fatal(stored)
	}
	config := requireMedia[b.Classification](t, h, "classification.get", b.Empty{})
	requireMedia[b.Classification](t, h, "classification.update", config, config.Revision)
	// Repair only restores the exact encrypted original and token; the worker can
	// never submit a replacement body. Wrong digest/unknown input fields fail.
	plan := requireMedia[b.RepairPlan](t, h, "strm.repair.preview", b.IDs{IDs: []string{asset}})
	requireMedia[b.RepairPlan](t, h, "strm.diagnostics", b.ID{ID: plan.ID})
	if e = os.Remove(filepath.Join(root, output)); e != nil {
		t.Fatal(e)
	}
	plan = requireMedia[b.RepairPlan](t, h, "strm.repair.preview", b.IDs{IDs: []string{asset}})
	if _, e = mediaCall(t, h, "strm.repair.approve", b.Approval{ID: plan.ID, Digest: strings.Repeat("b", 64)}); e == nil {
		t.Fatal("changed repair evidence")
	}
	requireMedia[b.RepairPlan](t, h, "strm.repair.approve", b.Approval{ID: plan.ID, Digest: plan.Digest})
	payload, _ := json.Marshal(map[string]string{"batchId": plan.ID})
	if e = le.ProcessSTRMRepair(ctx, jobs.Job{Payload: payload}); e != nil {
		t.Fatal(e)
	}
	for _, method := range []string{"strm.repair.approve", "strm.repair.retry"} {
		requireMedia[b.RepairPlan](t, h, method, b.Approval{ID: plan.ID, Digest: plan.Digest})
	}
	raw, e := os.ReadFile(filepath.Join(root, output))
	if e != nil || sha256.Sum256(raw) != before {
		t.Fatal("STRM bytes changed", e)
	}
	var grantAfter string
	if e = h.DB.QueryRow(ctx, `SELECT md5(to_jsonb(g)::text)FROM playback_grants g WHERE asset_id=$1`, asset).Scan(&grantAfter); e != nil || grantBefore != grantAfter {
		t.Fatal("grant changed", e)
	}
	rawInput, _ := json.Marshal(map[string]any{"ids": []string{asset}, "body": "https://attacker.invalid"})
	_, e = h.Call(ctx, contracts.HostCall{Method: "strm.repair.preview", Args: rawInput}, contracts.Permission{Scope: "strm.repair"}, func() error { return nil })
	mediaCode(t, e, contracts.InvalidParams)
	for _, method := range []string{"strm.write", "strm.readOwned", "storage.resolve", "storage.upload"} {
		_, e = h.Call(ctx, contracts.HostCall{Method: method, Args: []byte(`{}`)}, contracts.Permission{}, nil)
		mediaCode(t, e, contracts.Unsupported)
	}
	// No filesystem root, raw journal ciphertext or stable resolver key in DTOs.

}

func TestMediaScanControlHistoryAndArchiveApproval(t *testing.T) {
	h, ctx := mediaFixture(t)
	asset, _ := seedMedia(t, &h, ctx)
	cloud, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "primary", Enabled: true, Settings: map[string]any{"endpoint": "http://127.0.0.1:9", "cloudRoot": "/115/Media"}, Secrets: map[string]string{"token": "CLOUD_SECRET"}})
	if e != nil {
		t.Fatal(e)
	}
	receipt := requireMedia[b.Receipt](t, h, "library.scan.start", b.ScanStart{RootID: cloud.ID, Path: "."})
	le := library.Engine{DB: h.DB, Connections: h.Connections}
	scan, e := le.ScanByID(ctx, receipt.ID)
	if e != nil {
		t.Fatal(e)
	}
	requireMedia[b.Receipt](t, h, "library.scan.control", b.Control{ID: scan.ID, Action: "cancel"}, scan.Revision)
	scan, e = le.ScanByID(ctx, scan.ID)
	if e != nil {
		t.Fatal(e)
	}
	requireMedia[b.Receipt](t, h, "library.history.hide", b.HistorySelections{Items: []b.HistorySelection{{ID: scan.ID, Kind: "scan", UpdatedAt: scan.UpdatedAt}}})
	journal := library.Journal{DB: h.DB, Vault: h.Vault}
	root := h.Connections.Config.STRMOutputDir
	// Create a managed, unchanged active STRM and an obsolete generated NFO.
	if e = journal.Write(ctx, root, "Owned.strm", asset, "asset:"+asset, "strm", []byte("https://stable.fixture/resolve\n")); e != nil {
		t.Fatal(e)
	}
	if _, e = h.DB.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,'Owned.strm','fixture-reference')`, jobs.UUID(), asset); e != nil {
		t.Fatal(e)
	}
	if e = journal.Write(ctx, root, "Old.nfo", asset, "asset:"+asset, "nfo", []byte("owned")); e != nil {
		t.Fatal(e)
	}
	tx, e := h.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	write, e := journal.Stage(ctx, tx, root, "New.nfo", asset, "asset:"+asset, "nfo", []byte("replacement"))
	if e != nil {
		t.Fatal(e)
	}
	archive, e := journal.StageArchive(ctx, tx, root, "Old.nfo", asset, "nfo", []string{write})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{write, archive} {
		if e = journal.Apply(ctx, id); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = h.DB.Exec(ctx, `UPDATE media_artifact_journal SET updated_at=now()-interval '40 days'WHERE id=$1`, archive); e != nil {
		t.Fatal(e)
	}
	plan := requireMedia[b.ArchivePlan](t, h, "library.archive.preview", b.IDs{IDs: []string{archive}})
	if len(plan.Items) != 1 || !plan.PhysicalDeletion {
		t.Fatal(plan)
	}
	if _, e = mediaCall(t, h, "library.archive.confirmCleanup", b.ArchiveApproval{IDs: []string{archive}, PlanDigest: strings.Repeat("b", 64)}); e == nil {
		t.Fatal("changed cleanup evidence")
	}
	result := requireMedia[[]b.CleanupResult](t, h, "library.archive.confirmCleanup", b.ArchiveApproval{IDs: []string{archive}, PlanDigest: plan.PlanDigest})
	if len(result) != 1 || result[0].State != "removed" {
		t.Fatal(result)
	}
	if raw, e := os.ReadFile(filepath.Join(root, "Owned.strm")); e != nil || string(raw) != "https://stable.fixture/resolve\n" {
		t.Fatal("active STRM changed", e)
	}
}

func TestMediaOrganizeAndCorrectPreserveNativeIdentityEvidence(t *testing.T) {
	h, ctx := mediaFixture(t)
	_, candidate := seedMedia(t, &h, ctx)
	cloud, e := h.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "primary", Enabled: true, Settings: map[string]any{"endpoint": "http://127.0.0.1:9", "cloudRoot": "/115/Media"}, Secrets: map[string]string{"token": "CLOUD_SECRET"}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.DB.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'primary','clouddrive2')`, cloud.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = h.DB.Exec(ctx, `UPDATE cloud_files SET storage_id=$1`, cloud.ID); e != nil {
		t.Fatal(e)
	}
	var file string
	if e = h.DB.QueryRow(ctx, `SELECT file_id::text FROM library_candidates WHERE id=$1`, candidate).Scan(&file); e != nil {
		t.Fatal(e)
	}
	recognition := requireMedia[b.Recognition](t, h, "library.organize.preview", b.ID{ID: candidate})
	in := b.ConfirmIdentity{ID: candidate, MediaType: "movie", TMDBID: 42, Revision: recognition.Candidate.Revision, FileDigest: recognition.FileDigest}
	noEvidence := in
	noEvidence.FileDigest = ""
	if _, e = mediaCall(t, h, "library.organize.confirm", noEvidence, in.Revision); e == nil {
		t.Fatal("no preview evidence")
	}
	result := requireMedia[b.Receipt](t, h, "library.organize.confirm", in, in.Revision)
	if result.ID != file {
		t.Fatal(result)
	}
	var revision int64
	if e = h.DB.QueryRow(ctx, `SELECT revision FROM media_assets WHERE id=$1`, file).Scan(&revision); e != nil {
		t.Fatal(e)
	}
	// No output migration: native Correct keeps the original STRM path/body/key.
	var output string
	if e = h.DB.QueryRow(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path LIMIT 1`, file).Scan(&output); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(filepath.Join(h.Connections.Config.STRMOutputDir, output))
	if e != nil {
		t.Fatal(e)
	}
	var grant string
	if e = h.DB.QueryRow(ctx, `SELECT encode(token_digest,'hex')FROM playback_grants WHERE asset_id=$1`, file).Scan(&grant); e != nil {
		t.Fatal(e)
	}
	correct := b.ConfirmIdentity{ID: file, MediaType: "movie", TMDBID: 43, Revision: revision}
	// Previews may reject conflicting current canonical layout. A rejected native
	// preview never permits apply; use same-identity preview for the success path.
	preview := correct
	preview.TMDBID = 42
	preview.MigrateOutputs = true
	plan := requireMedia[b.IdentityPlan](t, h, "library.correct.preview", preview, revision)
	if plan.PlanDigest == "" {
		t.Fatal(plan)
	}
	requireMedia[b.Receipt](t, h, "library.correct.apply", correct, revision)
	after, e := os.ReadFile(filepath.Join(h.Connections.Config.STRMOutputDir, output))
	if e != nil || string(before) != string(after) {
		t.Fatal("STRM changed during correction", e)
	}
	var now string
	if e = h.DB.QueryRow(ctx, `SELECT encode(token_digest,'hex')FROM playback_grants WHERE asset_id=$1`, file).Scan(&now); e != nil || now != grant {
		t.Fatal("grant changed", e)
	}
}
