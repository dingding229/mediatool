package search

import (
	"encoding/json"
	"errors"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/sites"
	"strings"
	"testing"
)

func TestSeriesYearConflictRemainsVisibleAndRequiresExplicitConfirmation(t *testing.T) {
	ctx, e, account, newRun := snapshotSearchFixture(t)
	run := newRun()
	if err := e.SaveResources(ctx, run, account, []sites.Resource{{TorrentID: "10", Title: "The.Office.2019.S01E01.1080p.mkv"}}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(metadata.Item{ID: 10, MediaType: "series", Title: "The Office", OriginalTitle: "The Office", Year: "2005"})
	if _, err := e.DB.Exec(ctx, `UPDATE search_runs SET media_context=$2 WHERE id=$1`, run, raw); err != nil {
		t.Fatal(err)
	}
	out, err := e.GetFiltered(ctx, run, 1, 50, Filter{})
	if err != nil || out.Total != 1 || len(out.Resources) != 1 {
		t.Fatal("year-conflicted candidate hidden", out, err)
	}
	r := out.Resources[0]
	warning, _ := r.Metadata["yearWarning"].(string)
	if !strings.Contains(warning, "2019") || r.Metadata["identityConfirmed"] != false {
		t.Fatal("missing explicit warning", r.Metadata)
	}
	if err = e.MediaResource(ctx, run, r.ID); !errors.Is(err, ErrMediaYearConfirmation) {
		t.Fatal("unconfirmed identity accepted", err)
	}
	if err = e.MediaResource(ctx, run, r.ID, true); err != nil {
		t.Fatal("explicit season-year confirmation rejected", err)
	}
}
