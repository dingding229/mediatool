package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/library"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestPendingImportNeverMixesLegacySparseLayoutAndResumesOnlyAfterExplicitMigration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PG required")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal("isolated PG required")
	}
	cleanup := `TRUNCATE integrations,download_tasks,media_items,media_assets,cloud_files,storage_connections CASCADE`
	db.Exec(ctx, cleanup)
	defer db.Exec(ctx, cleanup)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	root, local := t.TempDir(), t.TempDir()
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{PublicURL: "https://media.fixture", STRMOutputDir: root})
	defer m.Close()
	tmdb, err := m.Repository.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "owned", Enabled: true, Secrets: map[string]string{"token": "owned"}})
	if err != nil {
		t.Fatal(err)
	}
	cloud, err := m.Repository.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "owned", Enabled: true, Settings: map[string]any{"endpoint": "http://127.0.0.1:9", "cloudRoot": "/115"}, Secrets: map[string]string{"token": "owned"}})
	if err != nil {
		t.Fatal(err)
	}
	m.OutboundHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw := `{"id":10,"name":"Owned Show","first_air_date":"2026-01-01","overview":"owned"}`
		if strings.Contains(r.URL.Path, "/season/") {
			raw = `{"season_number":1,"episodes":[{"id":101,"show_id":10,"season_number":1,"episode_number":1,"name":"Owned one","air_date":"2026-01-01"},{"id":103,"show_id":10,"season_number":1,"episode_number":3,"name":"Owned three","air_date":"2026-01-03"}]}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw)), Request: r}, nil
	})}
	resource, task, file, cloudFile, media, transfer := jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID(), jobs.UUID()
	relative := "Owned.S01E01E03.mkv"
	os.WriteFile(filepath.Join(local, relative), []byte("owned"), 0640)
	os.WriteFile(filepath.Join(local, "Owned.S01E01E03.en.srt"), []byte("owned subtitle"), 0640)
	item := metadata.Item{ID: 10, MediaType: "series", Title: "Owned Show", Year: "2026"}
	details, _ := json.Marshal(taskDetails{Media: item, LocalPath: local})
	identity, _ := json.Marshal(item)
	hint, _ := vault.Seal(resource, map[string]string{"url": "https://owned.fixture/torrent"})
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO resources(id,account_id,torrent_id,title,download_hint_ciphertext)VALUES($1,$2,'owned','Owned',$3)`, []any{resource, cloud.ID, hint}},
		{`INSERT INTO download_tasks(id,resource_id,integration_id,download_state,details)VALUES($1,$2,$3,'local_complete',$4)`, []any{task, resource, cloud.ID, details}},
		{`INSERT INTO download_files(id,task_id,file_index,relative_path,size_bytes,local_path,media_state)VALUES($1,$2,0,$3,5,$4,'artifacts_pending')`, []any{file, task, relative, filepath.Join(local, relative)}},
		{`INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, []any{cloud.ID}},
		{`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/115/Owned.mkv',5)`, []any{cloudFile, cloud.ID}},
		{`INSERT INTO media_assets(id,cloud_file_id,media_identity)VALUES($1,$2,$3)`, []any{file, cloudFile, identity}},
		{`INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',10,'Owned Show',$2)`, []any{media, identity}},
		{`INSERT INTO media_asset_bindings(asset_id,media_id,download_file_id,season,episode,episodes)VALUES($1,$2,$1,1,1,'[1,3]')`, []any{file, media}},
		{`INSERT INTO transfer_tasks(id,download_file_id,integration_id,destination,device_id,state)VALUES($1,$2,$3,'/115/Owned.mkv','owned','artifacts_pending')`, []any{transfer, file, cloud.ID}},
	} {
		if _, err = db.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	_ = tmdb
	grants := playbackgrants.Service{DB: db, Vault: vault, PublicURL: m.Config.PublicURL}
	tx, _ := db.Begin(ctx)
	link, grant, err := grants.Prepare(ctx, tx, file, "/115/Owned.mkv")
	if err != nil {
		t.Fatal(err)
	}
	tx.Commit(ctx)
	old := "TV/Owned Show (2026)/Season 01/Owned Show - S01E01E03.strm"
	j := library.Journal{DB: db, Vault: vault}
	if err = j.Write(ctx, root, old, file, "asset:"+file, "strm", []byte(link+"\n")); err != nil {
		t.Fatal(err)
	}
	if err = j.Write(ctx, root, strings.TrimSuffix(old, ".strm")+".nfo", file, "asset:"+file, "nfo", []byte("<episodedetails><title>Owned old</title><season>1</season><episode>1</episode></episodedetails>")); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,$4)`, jobs.UUID(), file, old, playbackgrants.SecretFreeKeyRef(grant)); err != nil {
		t.Fatal(err)
	}
	e := Engine{DB: db, Connections: m}
	if err = e.ImportFile(ctx, transfer); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal("old pending task silently adopted new layout", err)
	}
	var count int
	db.QueryRow(ctx, `SELECT count(*) FROM strm_entries WHERE asset_id=$1`, file).Scan(&count)
	if count != 1 {
		t.Fatal("legacy and new alias layouts mixed")
	}
	var state string
	db.QueryRow(ctx, `SELECT state FROM transfer_tasks WHERE id=$1`, transfer).Scan(&state)
	if state != outputBlockedState {
		t.Fatal("no durable blocked state", state)
	}
	p, err := e.PreviewOutputResume(ctx, transfer)
	if err != nil || !p.NeedsMigration || p.Ready || p.PlanDigest != "" {
		t.Fatal("read-only preflight lost migration requirement", p, err)
	}
	if err = e.ResumeOutputImport(ctx, transfer, p.ControlRevision, strings.Repeat("a", 64)); err == nil {
		t.Fatal("resume accepted before explicit migration")
	}
	le := library.Engine{DB: db, Connections: m}
	input := library.Confirmation{MediaType: "series", TMDBID: 10, Season: 1, Episodes: []int{1, 3}, Revision: 1, MigrateOutputs: true}
	plan, err := le.PreviewIdentityMigration(ctx, file, input)
	if err != nil {
		t.Fatal(err)
	}
	input.PlanDigest = plan.PlanDigest
	if err = le.Correct(ctx, file, input); err != nil {
		t.Fatal(err)
	}
	p, err = e.PreviewOutputResume(ctx, transfer)
	if err != nil || !p.Ready || p.PlanDigest == "" {
		t.Fatal("migrated aliases cannot be explicitly resumed", p, err)
	}
	// Changed managed NFO blocks a stale confirmation, even while STRM is unchanged.
	var nfoPath string
	db.QueryRow(ctx, `SELECT relative_path FROM managed_media_artifacts WHERE asset_id=$1 AND artifact_kind='nfo' AND owner_scope=$2 ORDER BY relative_path LIMIT 1`, file, "asset:"+file).Scan(&nfoPath)
	originalNFO, _ := os.ReadFile(filepath.Join(root, nfoPath))
	os.WriteFile(filepath.Join(root, nfoPath), []byte("manual NFO"), 0640)
	if err = e.ResumeOutputImport(ctx, transfer, p.ControlRevision, p.PlanDigest); err == nil {
		t.Fatal("stale preview ignored changed metadata")
	}
	os.WriteFile(filepath.Join(root, nfoPath), originalNFO, 0640)
	if err = e.ResumeOutputImport(ctx, transfer, p.ControlRevision, p.PlanDigest); err != nil {
		t.Fatal(err)
	}
	if err = e.ResumeOutputImport(ctx, transfer, p.ControlRevision, p.PlanDigest); err != nil {
		t.Fatal("approval replay not idempotent", err)
	}
	var payload []byte
	db.QueryRow(ctx, `SELECT payload FROM jobs WHERE kind='transfer.import.resume'`).Scan(&payload)
	// A modification after approval is rechecked by the actual worker. It pauses,
	// preserving files. Re-approval must create a new revision-keyed job even if
	// restoration yields the same content digest as the first approval.
	firstPayload := append([]byte(nil), payload...)
	os.WriteFile(filepath.Join(root, nfoPath), []byte("manual after approval"), 0640)
	if err = e.RunOutputImport(ctx, jobs.Job{Payload: firstPayload}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal("worker ignored post-approval modification", err)
	}
	os.WriteFile(filepath.Join(root, nfoPath), originalNFO, 0640)
	p, err = e.PreviewOutputResume(ctx, transfer)
	if err != nil || !p.Ready {
		t.Fatal(err)
	}
	if err = e.ResumeOutputImport(ctx, transfer, p.ControlRevision, p.PlanDigest); err != nil {
		t.Fatal("re-approval after explicit repair failed", err)
	}
	var approvals int
	db.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind='transfer.import.resume'`).Scan(&approvals)
	if approvals != 2 {
		t.Fatal("re-approval silently reused an exhausted job", approvals)
	}
	db.QueryRow(ctx, `SELECT payload FROM jobs WHERE kind='transfer.import.resume' ORDER BY created_at DESC LIMIT 1`).Scan(&payload)
	if err = e.RunOutputImport(ctx, jobs.Job{Payload: firstPayload}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal("old approval superseded new control revision", err)
	}
	if err = e.RunOutputImport(ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal("approved import-only recovery failed", err)
	}
	if err = e.RunOutputImport(ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal("completion replay failed", err)
	}
	db.QueryRow(ctx, `SELECT state FROM transfer_tasks WHERE id=$1`, transfer).Scan(&state)
	if state != "imported" {
		t.Fatal("resume did not finish import", state)
	}
	rows, _ := db.Query(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path`, file)
	paths := []string{}
	for rows.Next() {
		var path string
		rows.Scan(&path)
		paths = append(paths, path)
	}
	rows.Close()
	if len(paths) != 2 {
		t.Fatal("resume multiplied aliases")
	}
	for _, path := range paths {
		raw, _ := os.ReadFile(filepath.Join(root, path))
		if string(raw) != link+"\n" {
			t.Fatal("resume rewrote original fixed link")
		}
		if _, err = os.Stat(filepath.Join(root, strings.TrimSuffix(path, ".strm")+".en.srt")); err != nil {
			t.Fatal("subtitle skipped on resume")
		}
	}
	// A same-layout interrupted import reuses committed metadata/URI; do not
	// contact the provider again or overwrite an explicit correction.
	m.OutboundHTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("persisted output retry contacted TMDB")
		return nil, errors.New("must not contact")
	})}
	db.Exec(ctx, `UPDATE transfer_tasks SET state='artifacts_pending'WHERE id=$1`, transfer)
	db.Exec(ctx, `UPDATE download_files SET media_state='artifacts_pending'WHERE id=$1`, file)
	if err = e.ImportFile(ctx, transfer); err != nil {
		t.Fatal("same-layout persisted retry restaged new metadata", err)
	}
	for _, path := range paths {
		raw, _ := os.ReadFile(filepath.Join(root, path))
		if string(raw) != link+"\n" {
			t.Fatal("ordinary retry rewrote fixed URI")
		}
	}
	raw, _ := os.ReadFile(filepath.Join(local, relative))
	if string(raw) != "owned" {
		t.Fatal("seeding source changed")
	}
}
