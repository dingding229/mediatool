package search

import (
	"encoding/json"
	"media-tool/internal/modules/sites"
	"testing"
)

func TestPostgresI29BrowseAllRetainsDeliveryIdentityAndPagination(t *testing.T) {
	ctx, e, account, newRun := snapshotSearchFixture(t)
	run := newRun()
	if err := e.SaveResources(ctx, run, account, []sites.Resource{{TorrentID: "1", Title: "Where.the.Mask.Ends.S01.2026.1080p"}, {TorrentID: "2", Title: "Unrelated.Show.S01.2026.1080p"}, {TorrentID: "3", Title: "Unrelated.Movie.2026.1080p"}}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(i29Media())
	if _, err := e.DB.Exec(ctx, `UPDATE search_runs SET media_context=$2 WHERE id=$1`, run, raw); err != nil {
		t.Fatal(err)
	}
	filtered, err := e.GetFiltered(ctx, run, 1, 10, Filter{})
	if err != nil || filtered.Total != 1 || filtered.Excluded != 2 {
		t.Fatal(filtered, err)
	}
	all, err := e.GetFiltered(ctx, run, 1, 2, Filter{IgnoreMediaIdentity: true})
	if err != nil || all.Total != 3 || all.Excluded != 2 || len(all.Resources) != 2 {
		t.Fatal(all, err)
	}
	last, err := e.GetFiltered(ctx, run, 2, 2, Filter{IgnoreMediaIdentity: true})
	if err != nil || last.Total != 3 || len(last.Resources) != 1 {
		t.Fatal(last, err)
	}
	all.Resources = append(all.Resources, last.Resources...)
	for _, r := range all.Resources {
		err := e.MediaResource(ctx, run, r.ID, true)
		if (r.TorrentID == "1") != (err == nil) {
			t.Fatal("browse-all changed delivery evidence", r.TorrentID, err)
		}
	}
	var after []byte
	e.DB.QueryRow(ctx, `SELECT media_context FROM search_runs WHERE id=$1`, run).Scan(&after)
	var identity map[string]any
	json.Unmarshal(after, &identity)
	if identity["id"] != float64(300146) {
		t.Fatal("identity cleared", identity)
	}
}
