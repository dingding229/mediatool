package acquisition

import (
	"encoding/json"
	"media-tool/internal/modules/jobs"
	"strings"
	"testing"
)

func TestSyncKeepsZeroHonestErrorsVisibleAndNeverAddsTorrent(t *testing.T) {
	f := newSourceFixture(t)
	initial, err := f.engine.List(f.ctx)
	if err != nil || len(initial) != 1 || initial[0].Progress != nil {
		t.Fatal(initial, err)
	}
	body, _ := json.Marshal(map[string]string{"taskId": f.taskID})
	if err = f.engine.Add(f.ctx, jobs.Job{Payload: body}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	adds := f.adds
	f.mu.Unlock()
	if err = f.engine.Sync(f.ctx); err != nil {
		t.Fatal(err)
	}
	list, err := f.engine.List(f.ctx)
	if err != nil || len(list) != 1 || list[0].Progress == nil || *list[0].Progress != 0 || list[0].SyncedAt == nil || list[0].QBState != "downloading" {
		t.Fatal(list, err)
	}
	f.engine.DB.Exec(f.ctx, `UPDATE download_tasks SET details=details||'{"extraIdentityEvidence":"retained"}'::jsonb WHERE id=$1`, f.taskID)
	f.mu.Lock()
	f.present = false
	f.mu.Unlock()
	if err = f.engine.Sync(f.ctx); err == nil {
		t.Fatal("missing torrent not reported")
	}
	list, err = f.engine.List(f.ctx)
	if err != nil || list[0].SyncError == "" || list[0].Progress == nil || *list[0].Progress != 0 || list[0].SyncedAt == nil {
		t.Fatal(list, err)
	}
	var raw string
	f.engine.DB.QueryRow(f.ctx, `SELECT details::text FROM download_tasks WHERE id=$1`, f.taskID).Scan(&raw)
	if !strings.Contains(raw, "extraIdentityEvidence") {
		t.Fatal("unrelated details lost")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.adds != adds {
		t.Fatal("refresh re-added absent torrent")
	}
}
func TestSearchContextIdentityRequiresExplicitCorrection(t *testing.T) {
	f := newSourceFixture(t)
	f.engine.DB.Exec(f.ctx, `UPDATE search_runs SET media_context='{"id":2,"mediaType":"movie","title":"Another"}' WHERE id=$1`, f.runID)
	if _, err := f.engine.StartFromRun(f.ctx, f.runID, f.resourceID, "movie", 1); err == nil {
		t.Fatal("silent search identity mismatch accepted")
	}
	if _, err := f.engine.StartFromRunIdentity(f.ctx, f.runID, f.resourceID, "movie", 1, true); err != nil {
		t.Fatal("explicit correction denied", err)
	}
}
