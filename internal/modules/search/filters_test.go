package search

import (
	"fmt"
	"media-tool/internal/modules/sites"
	"strings"
	"testing"
)

func TestFrozenResourceFilters(t *testing.T) {
	zero := float64(0)
	r := Resource{Title: "Owned.Show.2026.S01E03.4K.WEB.DL.x265-GRP", AccountID: "owned", Size: 1 << 30, Seeders: 5, DownloadFactor: &zero}
	if !(Filter{Resolution: "2160p", Codec: "h265", Source: "web-dl", FreeOnly: true, MinSeeders: 2, MinSize: 1 << 29, AccountID: "owned"}).Match(r) {
		t.Fatal(attrs(r.Title))
	}
	for _, f := range []Filter{{Resolution: "1080p"}, {Codec: "h264"}, {AccountID: "other"}, {MinSeeders: 6}, {MaxSize: 10}, {Resolution: "unknown"}} {
		if f.Match(r) {
			t.Fatal(f)
		}
	}
	r.Title = "No Attributes"
	if !(Filter{Resolution: "unknown"}).Match(r) {
		t.Fatal("unknown")
	}
	r.DownloadFactor = nil
	if (Filter{FreeOnly: true}).Match(r) {
		t.Fatal("missing free factor")
	}
	if (Filter{MinSize: 10, MaxSize: 1}).Validate() == nil {
		t.Fatal("invalid range")
	}
}

func TestPostgresFiltersCoverWholeSnapshotAndPreserveResourceIdentity(t *testing.T) {
	ctx, engine, account, newRun := snapshotSearchFixture(t)
	runID := newRun()
	free := float64(0)
	list := []sites.Resource{}
	for i := range 65 {
		resolution := "1080p"
		if i < 12 {
			resolution = "2160p"
		}
		list = append(list, sites.Resource{TorrentID: fmt.Sprint(i), Title: fmt.Sprintf("Owned.Film.2026.%s.WEB-DL.x265-%02d", resolution, i), Size: int64(i+1) << 30, Seeders: i, DownloadFactor: &free, DownloadURL: fmt.Sprintf("https://owned.invalid/download.php?id=%d", i)})
	}
	if err := engine.SaveResources(ctx, runID, account, list); err != nil {
		t.Fatal(err)
	}
	full, err := engine.GetFiltered(ctx, runID, 1, 50, Filter{})
	if err != nil || full.Total != 65 || len(full.Resources) != 50 {
		t.Fatal(full.Total, len(full.Resources), err)
	}
	filtered, err := engine.GetFiltered(ctx, runID, 1, 5, Filter{Resolution: "2160p", Codec: "h265", FreeOnly: true})
	if err != nil || filtered.Total != 12 || len(filtered.Resources) != 5 {
		t.Fatal(filtered.Total, len(filtered.Resources), err)
	}
	if filtered.Resources[0].TorrentID != "11" || filtered.Resources[0].ID == "" {
		t.Fatal("resource binding changed", filtered.Resources[0])
	}
	last, err := engine.GetFiltered(ctx, runID, 3, 5, Filter{Resolution: "2160p"})
	if err != nil || last.Total != 12 || len(last.Resources) != 2 {
		t.Fatal(last.Total, len(last.Resources), err)
	}
	for _, sort := range []string{"seeders_asc", "seeders_desc", "size_asc", "size_desc"} {
		first, err := engine.GetFiltered(ctx, runID, 1, 5, Filter{Sort: sort, Keyword: "owned film"})
		second, err2 := engine.GetFiltered(ctx, runID, 2, 5, Filter{Sort: sort, Keyword: "owned film"})
		wantFirst, wantSecond := "0", "5"
		if strings.HasSuffix(sort, "desc") {
			wantFirst, wantSecond = "64", "59"
		}
		if err != nil || err2 != nil || first.Total != 65 || second.Total != 65 || first.Resources[0].TorrentID != wantFirst || second.Resources[0].TorrentID != wantSecond {
			t.Fatal(sort, first, second, err, err2)
		}
	}

}

func TestKeywordAndSortValidation(t *testing.T) {
	r := Resource{Title: "Owned.Show.2026.1080p", Metadata: map[string]any{"subtitle": "中文 标题"}}
	for _, f := range []Filter{{Keyword: "OWNED show"}, {Keyword: "中文 标题"}, {Keyword: "  show  ", Sort: "size_asc"}} {
		if f.Validate() != nil || !f.Match(r) {
			t.Fatal(f)
		}
	}
	if (Filter{Keyword: "other"}).Match(r) {
		t.Fatal("unmatched keyword")
	}
	for _, f := range []Filter{{Sort: "size desc;drop table"}, {Keyword: "bad\x00"}, {Keyword: strings.Repeat("x", 513)}} {
		if f.Validate() == nil {
			t.Fatal(f)
		}
	}
}

func TestMultiSelectionFiltersORWithinGroupANDCrossGroups(t *testing.T) {
	r := Resource{Title: "Owned Film 2026 2160p WEB-DL x265", AccountID: "b"}
	f := Filter{Resolutions: []string{"1080p", "2160p"}, Sources: []string{"web-dl", "bluray"}, Codecs: []string{"h264", "h265"}, AccountIDs: []string{"a", "b"}}
	if f.Validate() != nil || !f.Match(r) {
		t.Fatal("selected resource rejected")
	}
	f.Sources = []string{"bluray", "hdtv"}
	if f.Match(r) {
		t.Fatal("OR leaked across groups")
	}
	f.Sources = nil
	f.AccountIDs = []string{"a"}
	if f.Match(r) {
		t.Fatal("site selection ignored")
	}
	if (Filter{Sources: []string{""}}).Validate() == nil {
		t.Fatal("empty list item accepted")
	}
}

func TestPostgresMultiSelectionCoversCompleteSnapshot(t *testing.T) {
	ctx, engine, account, newRun := snapshotSearchFixture(t)
	run := newRun()
	list := []sites.Resource{}
	for i := 0; i < 65; i++ {
		resolution := "1080p"
		if i%2 == 0 {
			resolution = "2160p"
		}
		list = append(list, sites.Resource{TorrentID: fmt.Sprint(i), Title: "Owned.2026." + resolution + ".WEB-DL.x265", Size: 1 << 30, Seeders: i, DownloadURL: "https://owned.invalid/download?id=" + fmt.Sprint(i)})
	}
	if e := engine.SaveResources(ctx, run, account, list); e != nil {
		t.Fatal(e)
	}
	result, e := engine.GetFiltered(ctx, run, 2, 50, Filter{Resolutions: []string{"1080p", "2160p"}, Sources: []string{"web-dl", "bluray"}, Codecs: []string{"h265"}})
	if e != nil || result.Total != 65 || len(result.Resources) != 15 {
		t.Fatal(result.Total, len(result.Resources), e)
	}
}

func TestPromotionSortWholeSnapshotUnknownLast(t *testing.T) {
	ctx, e, account, newRun := snapshotSearchFixture(t)
	run := newRun()
	free, half, normal := 0.0, 0.5, 1.0
	list := []sites.Resource{{TorrentID: "unknown", Title: "Unknown", Size: 1, Seeders: 100, DownloadURL: "https://owned.invalid/1"}, {TorrentID: "half", Title: "Half", Size: 1, DownloadFactor: &half, DownloadURL: "https://owned.invalid/2"}, {TorrentID: "free", Title: "Free", Size: 1, DownloadFactor: &free, DownloadURL: "https://owned.invalid/3"}, {TorrentID: "normal", Title: "Normal", Size: 1, DownloadFactor: &normal, DownloadURL: "https://owned.invalid/4"}}
	if err := e.SaveResources(ctx, run, account, list); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		sort string
		want []string
	}{{"promotion_asc", []string{"free", "half", "normal", "unknown"}}, {"promotion_desc", []string{"normal", "half", "free", "unknown"}}} {
		got := []string{}
		for p := 1; p <= 2; p++ {
			out, err := e.GetFiltered(ctx, run, p, 2, Filter{Sort: test.sort})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range out.Resources {
				got = append(got, r.TorrentID)
			}
		}
		for i := range got {
			if got[i] != test.want[i] {
				t.Fatal(test.sort, got)
			}
		}
	}
}
