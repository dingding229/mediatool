package acquisition

import (
	"context"
	"encoding/json"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/platform/mediaext"
	"strings"
	"testing"
)

func TestPrepareTransfersUsesAllPlayableContainersAndExcludesDiscAndSubtitles(t *testing.T) {
	f := newSourceFixture(t)
	ctx, e := f.ctx, f.engine
	// All provider addresses are inert fixtures: preparation must only enqueue,
	// never touch real files, cloud storage or an external downloader.
	if _, err := e.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "Fixture cloud", Enabled: true, Settings: map[string]any{"endpoint": "http://fixture.invalid:19798", "cloudRoot": "/115/Fixture"}, Secrets: map[string]string{"token": "fixture"}}); err != nil {
		t.Fatal(err)
	}
	details, err := json.Marshal(taskDetails{Media: metadata.Item{ID: 1, Title: "Owned Movie", Year: "2026", MediaType: "movie"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.DB.Exec(ctx, `UPDATE download_tasks SET download_state='local_complete',details=$2 WHERE id=$1`, f.taskID, details); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.DB.Exec(context.Background(), `DELETE FROM transfer_tasks WHERE download_file_id IN(SELECT id FROM download_files WHERE task_id=$1)`, f.taskID)
	})
	type file struct {
		id, name string
		video    bool
	}
	files := []file{}
	extensions := append(mediaext.VideoExtensions(), append(mediaext.SubtitleExtensions(), ".iso", ".nfo")...)
	for i, ext := range extensions {
		item := file{id: jobs.UUID(), name: "Owned.Movie.2026." + strings.TrimPrefix(strings.ToUpper(ext), "."), video: i < len(mediaext.VideoExtensions())}
		if _, err = e.DB.Exec(ctx, `INSERT INTO download_files(id,task_id,file_index,relative_path,size_bytes)VALUES($1,$2,$3,$4,100)`, item.id, f.taskID, i, item.name); err != nil {
			t.Fatal(err)
		}
		files = append(files, item)
	}
	if err = e.PrepareTransfers(ctx); err != nil {
		t.Fatal(err)
	}
	for _, item := range files {
		var state string
		var transfers int
		if err = e.DB.QueryRow(ctx, `SELECT f.media_state,(SELECT count(*) FROM transfer_tasks t WHERE t.download_file_id=f.id) FROM download_files f WHERE f.id=$1`, item.id).Scan(&state, &transfers); err != nil {
			t.Fatal(err)
		}
		if item.video && (transfers != 1 || state == "non_media") || !item.video && (transfers != 0 || state != "non_media") {
			t.Errorf("%s: media_state=%s transfers=%d", item.name, state, transfers)
		}
	}
}
