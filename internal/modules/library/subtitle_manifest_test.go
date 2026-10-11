package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestSubtitleManifestRequiresIndexPairsAndStagesAllSupportedSidecars(t *testing.T) {
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
	db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`)
	defer db.Exec(ctx, `TRUNCATE cloud_files,storage_connections,media_assets CASCADE`)
	storage, file, asset := jobs.UUID(), jobs.UUID(), jobs.UUID()
	db.Exec(ctx, `INSERT INTO storage_connections(id,name,provider)VALUES($1,'owned','clouddrive2')`, storage)
	db.Exec(ctx, `INSERT INTO cloud_files(id,storage_id,full_path,size_bytes)VALUES($1,$2,'/115/owned',1)`, file, storage)
	db.Exec(ctx, `INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, asset, file)
	vault, _ := secrets.New(strings.Repeat("a", 64))
	source, target := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(source, "video.en.idx"), []byte("owned index"), 0640)
	os.WriteFile(filepath.Join(source, "video.zh.srt"), []byte("owned text"), 0640)
	if err = CopySubtitles(ctx, db, vault, source, "video.mkv", target, "Movie/new.strm", asset); err == nil {
		t.Fatal("incomplete VobSub pair accepted")
	}
	var count int
	db.QueryRow(ctx, `SELECT count(*) FROM media_artifact_journal WHERE asset_id=$1`, asset).Scan(&count)
	if count != 0 {
		t.Fatal("incomplete manifest partially staged")
	}
	os.WriteFile(filepath.Join(source, "video.en.sub"), []byte("owned bitmap data"), 0640)
	os.WriteFile(filepath.Join(source, "video.forced.sup"), []byte("owned PGS"), 0640)
	os.WriteFile(filepath.Join(source, "other.srt"), []byte("not selected"), 0640)
	if err = CopySubtitles(ctx, db, vault, source, "video.mkv", target, "Movie/new.strm", asset); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{"en.idx", "en.sub", "zh.srt", "forced.sup"} {
		if _, err = os.Stat(filepath.Join(target, "Movie/new."+ext)); err != nil {
			t.Fatal("missing subtitle", ext)
		}
	}
	if _, err = os.Stat(filepath.Join(target, "Movie/newother.srt")); err == nil {
		t.Fatal("unrelated sidecar copied")
	}
	raw, _ := os.ReadFile(filepath.Join(source, "video.en.sub"))
	if string(raw) != "owned bitmap data" {
		t.Fatal("source altered")
	}
	// All subtitle aliases stage as one bounded manifest; none are partially
	// written when another alias has invalid ownership or a duplicate path.
	aliases := []string{"TV/Owned/Season 01/Owned - S01E01.strm", "TV/Owned/Season 01/Owned - S01E03.strm"}
	if err = CopySubtitlesOutputs(ctx, db, vault, source, "video.mkv", target, aliases, asset); err != nil {
		t.Fatal(err)
	}
	for _, output := range aliases {
		for _, suffix := range []string{"en.idx", "en.sub", "zh.srt", "forced.sup"} {
			p := strings.TrimSuffix(output, ".strm") + "." + suffix
			if _, err = os.Stat(filepath.Join(target, p)); err != nil {
				t.Fatal("sparse alias subtitle missing", err)
			}
		}
	}
	if err = CopySubtitlesOutputs(ctx, db, vault, source, "video.mkv", target, []string{aliases[0], aliases[0]}, asset); err == nil {
		t.Fatal("duplicate projection allowed")
	}

}
