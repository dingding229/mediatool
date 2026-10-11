package filemanager

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func fixture(t *testing.T) (context.Context, Engine, Root, *Local) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	t.Cleanup(cancel)
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var name string
	if db.QueryRow(ctx, `SELECT current_database()`).Scan(&name) != nil || name != "media_tool_test" {
		t.Fatal("isolated database required")
	}
	cleanup := `TRUNCATE integrations,storage_connections,subscriptions,download_tasks,jobs,media_items CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(context.Background(), cleanup) })
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	directory := t.TempDir()
	c, err := repo.Save(ctx, "", integrations.Input{Kind: "local", Name: "Owned local directory", Enabled: true, Settings: map[string]any{"rootPath": directory}})
	if err != nil {
		t.Fatal(err)
	}
	manager := integrations.NewManager(repo, config.Config{STRMOutputDir: t.TempDir()})
	t.Cleanup(manager.Close)
	e := Engine{DB: db, Connections: manager}
	root := Root{ID: c.ID, Name: c.Name, Kind: "local", Path: directory, Revision: c.Revision}
	local, err := OpenLocal(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { local.Close() })
	return ctx, e, root, local
}
func TestPostgresFilesPreviewExecuteReplayTrashAndRestore(t *testing.T) {
	ctx, e, root, _ := fixture(t)
	os.WriteFile(filepath.Join(root.Path, "Owned.mkv"), []byte("video"), 0o600)
	os.WriteFile(filepath.Join(root.Path, "Owned.zh.srt"), []byte("subtitle"), 0o600)
	p, err := e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "organize", Source: "Owned.mkv", MediaType: "movie", Title: "Owned", Year: "2026", Template: "{title} ({year}){ext}", IncludeSidecars: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root.Path, "电影")); !os.IsNotExist(err) {
		t.Fatal("preview mutated files")
	}
	if _, err = e.Execute(ctx, p.ID, "wrong", true); !errors.Is(err, ErrChanged) {
		t.Fatal("forged proof accepted")
	}
	out, err := e.Execute(ctx, p.ID, p.Digest, true)
	if err != nil || out.State != "succeeded" {
		t.Fatal(out.State, err)
	}
	again, err := e.Execute(ctx, p.ID, p.Digest, true)
	if err != nil || again.CompletedSteps != out.CompletedSteps {
		t.Fatal("completed operation replayed", err)
	}
	video := p.Items[0].Target
	trash, err := e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "trash", Source: video})
	if err != nil {
		t.Fatal(err)
	}
	if out, err = e.Execute(ctx, trash.ID, trash.Digest, true); err != nil || out.State != "succeeded" {
		t.Fatal("trash move failed", out, err)
	}
	restore, err := e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "move", Source: trash.Items[0].Target, Destination: "."})
	if err != nil {
		t.Fatal(err)
	}
	if out, err = e.Execute(ctx, restore.ID, restore.Digest, true); err != nil || out.State != "succeeded" {
		t.Fatal("restore failed", err)
	}
	data, _ := os.ReadFile(filepath.Join(root.Path, "Owned (2026).mkv"))
	if string(data) != "video" {
		t.Fatal("file bytes changed")
	}
	var audited int
	if e.DB.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='files.execute'`).Scan(&audited) != nil || audited < 3 {
		t.Fatal("confirmations not audited")
	}
}
func TestPostgresFilesRejectChangedSourceSeedingOutputAndExpiredPreview(t *testing.T) {
	ctx, e, root, _ := fixture(t)
	filename := filepath.Join(root.Path, "Owned.mkv")
	os.WriteFile(filename, []byte("before"), 0o600)
	p, err := e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "rename", Source: "Owned.mkv", Name: "New.mkv"})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filename, []byte("changed"), 0o600)
	if _, err = e.Execute(ctx, p.ID, p.Digest, true); !errors.Is(err, ErrChanged) {
		t.Fatal("changed source moved", err)
	}
	p, err = e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "rename", Source: "Owned.mkv", Name: "New.mkv"})
	if err != nil {
		t.Fatal(err)
	}
	e.DB.Exec(ctx, `UPDATE file_operations SET expires_at=now()-interval '1 second' WHERE id=$1`, p.ID)
	if _, err = e.Execute(ctx, p.ID, p.Digest, true); !errors.Is(err, ErrChanged) {
		t.Fatal("expired preview executed", err)
	}
	if _, err = e.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "qbittorrent", Name: "Owned seeding guard", Enabled: false, Settings: map[string]any{"localPath": root.Path}}); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "trash", Source: "Owned.mkv"}); !errors.Is(err, ErrProtected) {
		t.Fatal("seeding source not protected", err)
	}
	e.DB.Exec(ctx, `DELETE FROM integrations WHERE kind='qbittorrent'`)
	e.Connections.Config.STRMOutputDir = root.Path
	if _, err = e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "rename", Source: "Owned.mkv", Name: "New.mkv"}); !errors.Is(err, ErrProtected) {
		t.Fatal("managed output directory not protected", err)
	}
}
func TestPostgresFilesUnknownCommitReconcilesWithoutAnotherMutation(t *testing.T) {
	ctx, e, root, local := fixture(t)
	os.WriteFile(filepath.Join(root.Path, "Owned.mkv"), []byte("owned"), 0o600)
	p, err := e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "rename", Source: "Owned.mkv", Name: "New.mkv"})
	if err != nil {
		t.Fatal(err)
	}
	if err = local.Relocate(ctx, "Owned.mkv", "New.mkv"); err != nil {
		t.Fatal(err)
	}
	if _, err = e.DB.Exec(ctx, `UPDATE file_operations SET state='needs_reconcile' WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	out, err := e.Reconcile(ctx, p.ID)
	if err != nil || out.State != "succeeded" || out.CompletedSteps != 1 {
		t.Fatal("committed rename was not recognized", out, err)
	}
	out, err = e.Reconcile(ctx, p.ID)
	if err != nil || out.State != "succeeded" {
		t.Fatal("reconcile not idempotent")
	}
	if _, err = os.Stat(filepath.Join(root.Path, "New.mkv")); err != nil {
		t.Fatal(err)
	}
}

type memoryBackend struct {
	files map[string]File
	moves int
}

func (m *memoryBackend) Close() error { return nil }
func (m *memoryBackend) Find(_ context.Context, p string) (File, error) {
	f, ok := m.files[p]
	if !ok {
		return f, ErrMissing
	}
	return f, nil
}
func (m *memoryBackend) List(_ context.Context, p string) ([]File, error) {
	out := []File{}
	for _, f := range m.files {
		if filepath.Dir(f.Path) == p {
			out = append(out, f)
		}
	}
	return out, nil
}
func (m *memoryBackend) Mkdir(_ context.Context, p string) error {
	m.files[p] = File{ID: "dir:" + p, Name: filepath.Base(p), Path: p, Directory: true}
	return nil
}
func (m *memoryBackend) Relocate(_ context.Context, source, target string) error {
	f, ok := m.files[source]
	if !ok {
		return ErrMissing
	}
	if _, ok := m.files[target]; ok {
		return ErrConflict
	}
	m.moves++
	delete(m.files, source)
	f.Path = target
	f.Name = filepath.Base(target)
	m.files[target] = f
	return nil
}
func TestPostgresCloudRelocationPreservesAssetGrantAndRepairsIndex(t *testing.T) {
	ctx, e, _, _ := fixture(t)
	repo := e.Connections.Repository
	record, err := repo.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "Owned cloud", Enabled: true, Settings: map[string]any{"endpoint": "http://owned.invalid:19798", "cloudRoot": "/115/Owned"}, Secrets: map[string]string{"token": "PRIVATE-FIXTURE"}})
	if err != nil {
		t.Fatal(err)
	}
	provider := &memoryBackend{files: map[string]File{".": {ID: "root", Name: "Owned", Path: ".", Directory: true}, "old.mkv": {ID: "provider42", Name: "old.mkv", Path: "old.mkv", Size: 5}}}
	e.Open = func(_ context.Context, root Root) (Backend, error) { return provider, nil }
	file, asset := "00000000-0000-4000-8000-000000000042", "00000000-0000-4000-8000-000000000043"
	for _, s := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO storage_connections(id,name,provider)VALUES($1,'Owned','clouddrive2')`, []any{record.ID}},
		{`INSERT INTO cloud_files(id,storage_id,provider_file_id,full_path,size_bytes)VALUES($1,$2,'provider42','/115/Owned/old.mkv',5)`, []any{file, record.ID}},
		{`INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$2)`, []any{asset, file}},
		{`INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES('00000000-0000-4000-8000-000000000044',$1,'unchanged.strm','unchanged-file-key')`, []any{asset}},
	} {
		if _, err = e.DB.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	p, err := e.Preview(ctx, Input{ConnectionID: record.ID, Operation: "organize", Source: "old.mkv", MediaType: "movie", Title: "Movie", Name: "Custom.mkv"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := e.Execute(ctx, p.ID, p.Digest, true)
	if err != nil || out.State != "succeeded" {
		t.Fatal(out.State, err)
	}
	var actual, id, key string
	if err = e.DB.QueryRow(ctx, `SELECT f.full_path,a.id::text,s.key_reference FROM cloud_files f JOIN media_assets a ON a.cloud_file_id=f.id JOIN strm_entries s ON s.asset_id=a.id WHERE f.id=$1`, file).Scan(&actual, &id, &key); err != nil || actual != "/115/Owned/电影/Movie/Custom.mkv" || id != asset || key != "unchanged-file-key" {
		t.Fatal("asset path/identity protection failed", actual, err)
	}
	provider.files["replacement.mkv"] = File{ID: "new-provider", Name: "replacement.mkv", Path: "replacement.mkv", Size: 5}
	e.DB.Exec(ctx, `UPDATE cloud_files SET full_path='/115/Owned/replacement.mkv' WHERE id=$1`, file)
	if _, err = e.Preview(ctx, Input{ConnectionID: record.ID, Operation: "rename", Source: "replacement.mkv", Name: "Another.mkv"}); !errors.Is(err, ErrChanged) {
		t.Fatal("replacement file rebound existing asset", err)
	}
	var bytes []byte
	if err = e.DB.QueryRow(ctx, `SELECT plan FROM file_operations WHERE id=$1`, p.ID).Scan(&bytes); err != nil || !json.Valid(bytes) || strings.Contains(string(bytes), "PRIVATE-FIXTURE") {
		t.Fatal("credential in file plan")
	}
}
