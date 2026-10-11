package library

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestIdentityMigrationKeepsFileURLArchivesOwnedOutputsAndPreservesSharedNFO(t *testing.T) {
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
	var databaseName string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&databaseName)
	if databaseName != "media_tool_test" {
		t.Fatal("isolated media_tool_test required")
	}
	_, err = db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	root := t.TempDir()
	m := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, config.Config{PublicURL: "https://media.fixture", STRMOutputDir: root})
	defer m.Close()
	c, err := m.Repository.Save(ctx, "", integrations.Input{Kind: "tmdb", Name: "owned identity fixture", Enabled: true, Secrets: map[string]string{"token": "owned-metadata"}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM integrations WHERE id=$1`, c.ID)
	m.OutboundHTTP = &http.Client{Transport: httpFixture(func(r *http.Request) (*http.Response, error) {
		id, _ := strconv.Atoi(path.Base(r.URL.Path))
		if strings.Contains(r.URL.Path, "/season/") {
			fields := strings.Split(r.URL.Path, "/")
			show, _ := strconv.Atoi(fields[3])
			season := id
			episodeRows := []map[string]any{}
			for _, n := range []int{1, 2, 3, 4} {
				episodeRows = append(episodeRows, map[string]any{"id": show*1000 + season*10 + n, "show_id": show, "season_number": season, "episode_number": n, "name": "Owned episode " + strconv.Itoa(n), "overview": "owned episode plot", "air_date": "2026-01-01", "runtime": 25})
			}
			raw, _ := json.Marshal(map[string]any{"season_number": season, "episodes": episodeRows})
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
		}
		var data map[string]any
		if strings.HasPrefix(r.URL.Path, "/3/tv/") {
			data = map[string]any{"id": id, "name": "Owned Series " + strconv.Itoa(id), "first_air_date": "2026-01-01", "overview": "owned show"}
		} else {
			data = map[string]any{"id": id, "title": "Owned Film " + strconv.Itoa(id), "release_date": "2026-01-01", "overview": "owned film"}
		}
		raw, _ := json.Marshal(data)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	storage, file, asset := jobs.UUID(), jobs.UUID(), jobs.UUID()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, []any{storage}},
		{`INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/115/Owned/Film.mkv',1)`, []any{file, storage}},
		{`INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, []any{asset, file}},
	} {
		if _, err = db.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	grants := playbackgrants.Service{DB: db, Vault: vault, PublicURL: m.Config.PublicURL}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	link, grant, err := grants.Prepare(ctx, tx, asset, "/115/Owned/Film.mkv")
	if err != nil {
		t.Fatal(err)
	}
	tx.Commit(ctx)
	output := "Movies/Owned Film (2026)/Film.strm"
	entry := jobs.UUID()
	if _, err = db.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,$4)`, entry, asset, output, playbackgrants.SecretFreeKeyRef(grant)); err != nil {
		t.Fatal(err)
	}
	j := Journal{DB: db, Vault: vault}
	contents := map[string][]byte{output: []byte(link + "\n"), strings.TrimSuffix(output, ".strm") + ".nfo": []byte("<movie><title>Old title</title></movie>"), strings.TrimSuffix(output, ".strm") + ".zh.srt": []byte("owned text subtitle"), strings.TrimSuffix(output, ".strm") + ".en.idx": []byte("owned bitmap index"), strings.TrimSuffix(output, ".strm") + ".en.sub": []byte("owned bitmap data")}
	for p, raw := range contents {
		kind := "subtitle"
		if strings.HasSuffix(p, ".strm") {
			kind = "strm"
		} else if strings.HasSuffix(p, ".nfo") {
			kind = "nfo"
		}
		if err = j.Write(ctx, root, p, asset, "asset:"+asset, kind, raw); err != nil {
			t.Fatal(err)
		}
	}
	e := Engine{DB: db, Connections: m}
	in := Confirmation{MediaType: "series", TMDBID: 10, Season: 1, Episodes: []int{1}, Revision: 1}
	if err = e.Correct(ctx, asset, in); err == nil {
		t.Fatal("type migration happened without opt-in")
	}
	in.MigrateOutputs = true
	preview, err := e.PreviewIdentityMigration(ctx, asset, in)
	if err != nil {
		t.Fatal(err)
	}
	in.PlanDigest = preview.PlanDigest
	var previewRevision int64
	db.QueryRow(ctx, `SELECT revision FROM media_assets WHERE id=$1`, asset).Scan(&previewRevision)
	if previewRevision != 1 {
		t.Fatal("preview changed binding")
	}
	edited := strings.TrimSuffix(output, ".strm") + ".zh.srt"
	os.WriteFile(filepath.Join(root, edited), []byte("manual subtitle"), 0640)
	if got := e.Correct(ctx, asset, in); !errors.Is(got, ErrArtifactConflict) {
		t.Fatal("manual subtitle was not rejected as a conflict", got)
	}
	var revision int64
	db.QueryRow(ctx, `SELECT revision FROM media_assets WHERE id=$1`, asset).Scan(&revision)
	if revision != 1 {
		t.Fatal("failed preflight changed identity")
	}
	os.WriteFile(filepath.Join(root, edited), contents[edited], 0640)
	stale := in
	stale.PlanDigest = strings.Repeat("0", 64)
	if err = e.Correct(ctx, asset, stale); err == nil {
		t.Fatal("missing/stale preview accepted")
	}
	if err = e.Correct(ctx, asset, in); err != nil {
		t.Fatal(err)
	}
	var current string
	db.QueryRow(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1`, asset).Scan(&current)
	if !strings.HasPrefix(current, "TV/Owned Series 10 (2026)/Season 01/") {
		t.Fatal("wrong destination", current)
	}
	raw, err := os.ReadFile(filepath.Join(root, current))
	if err != nil || string(raw) != link+"\n" {
		t.Fatal("fixed URL changed", err)
	}
	if got, err := grants.ExistingURL(ctx, asset); err != nil || got != link {
		t.Fatal("file grant changed", err)
	}
	var archiveCount int
	db.QueryRow(ctx, `SELECT count(*) FROM media_artifact_journal WHERE asset_id=$1 AND operation='archive' AND state='applied'`, asset).Scan(&archiveCount)
	if archiveCount != len(contents) {
		t.Fatal("incomplete archive manifest", archiveCount)
	}
	for old := range contents {
		if _, err = os.Stat(filepath.Join(root, old)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("old playable output not retired", old)
		}
	}
	for _, suffix := range []string{".zh.srt", ".en.idx", ".en.sub"} {
		if _, err = os.Stat(filepath.Join(root, strings.TrimSuffix(current, ".strm")+suffix)); err != nil {
			t.Fatal("subtitle manifest dropped", suffix)
		}
	}
	oldShow := path.Join(path.Dir(path.Dir(current)), "tvshow.nfo")
	showBytes, _ := os.ReadFile(filepath.Join(root, oldShow))
	in.TMDBID = 20
	in.Season = 2
	in.Episodes = []int{3}
	in.Revision = 2
	preview, err = e.PreviewIdentityMigration(ctx, asset, in)
	if err != nil {
		t.Fatal(err)
	}
	in.PlanDigest = preview.PlanDigest
	if err = e.Correct(ctx, asset, in); err != nil {
		t.Fatal("cross-series migration", err)
	}
	afterShow, _ := os.ReadFile(filepath.Join(root, oldShow))
	if string(afterShow) != string(showBytes) {
		t.Fatal("shared old show NFO was altered/removed")
	}
	db.QueryRow(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1`, asset).Scan(&current)
	if !strings.HasPrefix(current, "TV/Owned Series 20 (2026)/Season 02/") {
		t.Fatal("cross-show output remains old", current)
	}
	in.MediaType = "movie"
	in.TMDBID = 30
	in.Revision = 3
	in.Episodes = nil
	preview, err = e.PreviewIdentityMigration(ctx, asset, in)
	if err != nil {
		t.Fatal(err)
	}
	in.PlanDigest = preview.PlanDigest
	if err = e.Correct(ctx, asset, in); err != nil {
		t.Fatal("series to movie migration", err)
	}
	db.QueryRow(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1`, asset).Scan(&current)
	if !strings.HasPrefix(current, "Movies/Owned Film 30 (2026)/") {
		t.Fatal("movie output incorrect", current)
	}
	raw, _ = os.ReadFile(filepath.Join(root, current))
	if string(raw) != link+"\n" {
		t.Fatal("final URL changed")
	}
	// Real sparse projections must survive both fan-out and collapse, not multiply
	// entries every time an identity migration is repeated.
	in.MediaType = "series"
	in.TMDBID = 40
	in.Season = 1
	in.Episodes = []int{1, 3}
	in.Revision = 4
	preview, err = e.PreviewIdentityMigration(ctx, asset, in)
	if err != nil {
		t.Fatal(err)
	}
	generated := 0
	for _, op := range preview.Operations {
		if op.Kind == "strm" && op.Operation == "write" {
			generated++
		}
	}
	if generated != 2 {
		t.Fatal("sparse migration not fully previewed", generated)
	}
	var countBefore int
	db.QueryRow(ctx, `SELECT count(*) FROM strm_entries WHERE asset_id=$1`, asset).Scan(&countBefore)
	if countBefore != 1 {
		t.Fatal("preview wrote alias entries")
	}
	in.PlanDigest = preview.PlanDigest
	if err = e.Correct(ctx, asset, in); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1 ORDER BY output_path`, asset)
	if err != nil {
		t.Fatal(err)
	}
	sparse := []string{}
	for rows.Next() {
		var p string
		rows.Scan(&p)
		sparse = append(sparse, p)
	}
	rows.Close()
	if len(sparse) != 2 || !strings.Contains(sparse[0], "S01E01") || !strings.Contains(sparse[1], "S01E03") {
		t.Fatal("sparse consumer aliases wrong", sparse)
	}
	for _, output := range sparse {
		raw, _ := os.ReadFile(filepath.Join(root, output))
		if string(raw) != link+"\n" {
			t.Fatal("sparse projection reconstructed/changed file URI")
		}
		for _, suffix := range []string{".zh.srt", ".en.idx", ".en.sub"} {
			if _, err = os.Stat(filepath.Join(root, strings.TrimSuffix(output, ".strm")+suffix)); err != nil {
				t.Fatal("alias subtitle manifest lost", err)
			}
		}
	}
	in.MigrateOutputs = false
	in.Revision = 5
	in.Episodes = []int{1, 2, 3}
	if err = e.Correct(ctx, asset, in); err == nil {
		t.Fatal("coverage changed without explicit layout confirmation")
	}
	in.MigrateOutputs = true
	changed := strings.TrimSuffix(sparse[1], ".strm") + ".zh.srt"
	os.WriteFile(filepath.Join(root, changed), []byte("manual alias subtitle"), 0640)
	if _, err = e.PreviewIdentityMigration(ctx, asset, in); !errors.Is(err, ErrArtifactConflict) {
		t.Fatal("changed alias subtitle not preserved", err)
	}
	os.WriteFile(filepath.Join(root, changed), []byte("owned text subtitle"), 0640)
	preview, err = e.PreviewIdentityMigration(ctx, asset, in)
	if err != nil {
		t.Fatal(err)
	}
	in.PlanDigest = preview.PlanDigest
	if err = e.Correct(ctx, asset, in); err != nil {
		t.Fatal("collapse two aliases into contiguous projection", err)
	}
	var finalCount int
	db.QueryRow(ctx, `SELECT count(*) FROM strm_entries WHERE asset_id=$1`, asset).Scan(&finalCount)
	if finalCount != 1 {
		t.Fatal("alias collapse multiplied entries", finalCount)
	}
	db.QueryRow(ctx, `SELECT output_path FROM strm_entries WHERE asset_id=$1`, asset).Scan(&current)
	if !strings.Contains(current, "S01E01-E03") {
		t.Fatal("contiguous target incorrect", current)
	}
	raw, _ = os.ReadFile(filepath.Join(root, current))
	if string(raw) != link+"\n" {
		t.Fatal("collapse changed original URI")
	}

}
