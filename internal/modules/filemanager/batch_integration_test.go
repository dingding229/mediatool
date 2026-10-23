package filemanager

import (
	"encoding/json"
	"errors"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/settings"
	"os"
	"path/filepath"
	"testing"
)

func TestFileBatchSharedParentsReplayAndSidecars(t *testing.T) {
	ctx, e, root, _ := fixture(t)
	inputs := []Input{}
	for _, name := range []string{"Show.S01E01.mkv", "Show.S01E02.mkv"} {
		os.WriteFile(filepath.Join(root.Path, name), []byte(name), 0600)
		inputs = append(inputs, Input{ConnectionID: root.ID, Operation: "organize", Source: name, Title: "Owned Show", MediaType: "series", Year: "2026", Season: 1, Episode: len(inputs) + 1, IncludeSidecars: true})
	}
	os.WriteFile(filepath.Join(root.Path, "Show.S01E01.zh.srt"), []byte("owned subtitles"), 0600)
	batch, err := e.PreviewBatch(ctx, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.ApproveBatch(ctx, batch.ID, "wrong", true); !errors.Is(err, ErrChanged) {
		t.Fatal("invalid approval", err)
	}
	if _, err = e.ApproveBatch(ctx, batch.ID, batch.Digest, true); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"batchId": batch.ID})
	job := jobs.Job{Payload: payload}
	if err = e.ProcessBatch(ctx, job); err != nil {
		t.Fatal(err)
	}
	result, err := e.Batch(ctx, batch.ID)
	if err != nil || result.State != "succeeded" {
		t.Fatalf("%+v %v", result, err)
	}
	for _, item := range result.Items {
		for _, file := range item.Result.Plan.Items {
			if _, err := os.Stat(filepath.Join(root.Path, file.Target)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = e.ProcessBatch(ctx, job); err != nil {
		t.Fatal("idempotent replay", err)
	}
	var notifications int
	if err = e.DB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND event_type='files.batch.succeeded'`, batch.ID).Scan(&notifications); err != nil || notifications != 1 {
		t.Fatal("duplicate notification", notifications, err)
	}
}
func TestBatchConflictsReadonlyAndUnknownDoNotReplay(t *testing.T) {
	ctx, e, root, _ := fixture(t)
	name := "Owned.mkv"
	os.WriteFile(filepath.Join(root.Path, name), []byte("owned"), 0600)
	input := Input{ConnectionID: root.ID, Operation: "rename", Source: name, Name: "Renamed.mkv"}
	blocked, err := e.PreviewBatch(ctx, []Input{input, input})
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Items[1].State != "blocked" {
		t.Fatal("duplicate source not blocked")
	}
	if _, err = e.ApproveBatch(ctx, blocked.ID, blocked.Digest, true); !errors.Is(err, ErrConflict) {
		t.Fatal("blocked batch approved", err)
	}
	batch, err := e.PreviewBatch(ctx, []Input{input})
	if err != nil {
		t.Fatal(err)
	}
	e.ApproveBatch(ctx, batch.ID, batch.Digest, true)
	e.DB.Exec(ctx, `UPDATE file_batch_items SET state='running' WHERE batch_id=$1`, batch.ID)
	payload, _ := json.Marshal(map[string]string{"batchId": batch.ID})
	if err = e.ProcessBatch(ctx, jobs.Job{Payload: payload}); !errors.Is(err, jobs.ErrOperatorRequired) {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root.Path, name)); err != nil {
		t.Fatal("unknown operation replayed", err)
	}
	result, err := e.ReconcileBatch(ctx, batch.ID)
	if err != nil || result.State != "partial" || result.Items[0].State != "failed" {
		t.Fatal(result, err)
	}
	os.Chmod(root.Path, 0555)
	defer os.Chmod(root.Path, 0755)
	if _, err = e.Preview(ctx, input); !errors.Is(err, ErrReadOnly) {
		t.Fatal("read-only preview allowed", err)
	}
	roots, err := e.Roots(ctx)
	if err != nil || !roots[0].Capabilities.ReadOnly {
		t.Fatal(roots, err)
	}
}
func TestForbiddenAutomaticRoots(t *testing.T) {
	for _, p := range []string{"/", "/proc", "/proc/acpi", "/dev", "/var/run/docker.sock", "/var/lib/docker", "/data/pgdata", "/data/diagnostics", "/data/logs"} {
		if safeLocalRoot(p) {
			t.Fatal("forbidden root", p)
		}
	}
}

func TestBrowserShowsSymlinkNamesButProtectsSystemTargetsAndReadonlyChildren(t *testing.T) {
	ctx, e, root, _ := fixture(t)
	if err := os.Symlink("/", filepath.Join(root.Path, "host-root")); err != nil {
		t.Fatal(err)
	}
	out, err := e.List(ctx, root.ID, ".", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	entries := out["items"].([]File)
	if len(entries) != 1 || entries[0].Capabilities.Read || entries[0].Capabilities.Move {
		t.Fatal("symlink offered readable or writable capability", entries)
	}
	if _, err = e.List(ctx, root.ID, "host-root", 50, 0); !errors.Is(err, ErrProtected) {
		t.Fatal("symlink browsed", err)
	}
	if _, err = OpenLocal(filepath.Join(root.Path, "host-root")); !errors.Is(err, ErrProtected) {
		t.Fatal("symlink root opened", err)
	}
}

func TestFolderTemplateAndDirectoryRenameRecycleArePreviewed(t *testing.T) {
	ctx, e, root, _ := fixture(t)
	record, err := (settings.Repository{DB: e.DB}).Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	old := record.Value
	next := old
	next.MovieTemplate = "{title} ({year})/{title} ({year}) - {videoFormat}"
	if _, err = (settings.Repository{DB: e.DB}).Save(ctx, next, record.Revision); err != nil {
		t.Fatal(err)
	}
	defer func() {
		current, _ := (settings.Repository{DB: e.DB}).Get(ctx)
		(settings.Repository{DB: e.DB}).Save(ctx, old, current.Revision)
	}()
	os.WriteFile(filepath.Join(root.Path, "Owned.Movie.2026.1080p.mkv"), []byte("owned"), 0600)
	p, err := e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "organize", Source: "Owned.Movie.2026.1080p.mkv"})
	if err != nil || p.Items[0].Target != "电影/其他电影/Owned Movie (2026)/Owned Movie (2026) - 1080p.mkv" {
		t.Fatal(p, err)
	}
	os.Mkdir(filepath.Join(root.Path, "OwnedFolder"), 0755)
	os.WriteFile(filepath.Join(root.Path, "OwnedFolder", "owned.txt"), []byte("owned"), 0600)
	p, err = e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "rename", Source: "OwnedFolder", Name: "RenamedFolder"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Execute(ctx, p.ID, p.Digest, true); err != nil {
		t.Fatal(err)
	}
	p, err = e.Preview(ctx, Input{ConnectionID: root.ID, Operation: "trash", Source: "RenamedFolder"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Execute(ctx, p.ID, p.Digest, true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root.Path, p.Items[0].Target, "owned.txt")); err != nil {
		t.Fatal("recycle lost contents", err)
	}
}
