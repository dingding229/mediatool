package migrations_test

import (
	"errors"
	managedcatalog "media-tool/internal/modules/catalog"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/subtitles"
	"reflect"
	"testing"
)

func TestI40LanguageMigrationNamesOnlyAndRoundTrip(t *testing.T) {
	f := newPlaybackFixture(t, 93)
	before := f.text(t, `SELECT credentials_ciphertext::text FROM integrations WHERE id=$1`, f.storage)
	cases := []struct {
		name, old, language string
		tags                []string
	}{
		{"film.chs.srt", "chs", "chi", []string{"zh-Hans"}},
		{"film.cht.srt", "zho", "chi", []string{"zh-Hant"}},
		{"film.chs&eng.srt", "", "chi", []string{"zh-Hans", "en"}},
		{"film.繁英.srt", "", "chi", []string{"zh-Hant", "en"}},
		{"film.ja.srt", "", "jpn", []string{"ja"}},
		{"film.eng.srt", "chi", "chi", []string{}}, // preserve a previously selected language
		{"film.ko.srt", "eng", "eng", []string{"en"}},
		{"unknown.srt", "", "und", []string{}},
	}
	ids := []string{}
	for _, c := range cases {
		id := jobs.UUID()
		ids = append(ids, id)
		f.exec(t, `INSERT INTO managed_subtitles(id,asset_id,source,source_key,filename,extension,language,title,hidden,revision) VALUES($1::uuid,$2::uuid,'automatic',$1::text,$3,'srt',$4,'preserved',true,7)`, id, f.a.asset, c.name, c.old)
	}
	for round := 0; round < 2; round++ {
		if _, e := f.provider.UpTo(f.ctx, 94); e != nil {
			t.Fatal(e)
		}
		repo := subtitles.Repository{DB: f.pool, Root: t.TempDir()}
		for i, c := range cases {
			v, e := repo.Get(f.ctx, f.a.asset, ids[i])
			if e != nil || v.Language != c.language || !reflect.DeepEqual(v.LanguageTags, c.tags) || v.Revision != 7 || v.Title != "preserved" || !v.Hidden {
				t.Fatalf("%s: %+v %v", c.name, v, e)
			}
		}
		if f.text(t, `SELECT credentials_ciphertext::text FROM integrations WHERE id=$1`, f.storage) != before {
			t.Fatal("credentials changed")
		}
		if f.downloads.Load() != 0 || f.paths.Load() != 0 {
			t.Fatal("migration read provider content")
		}
		if round == 0 {
			if _, e := f.provider.Down(f.ctx); e != nil {
				t.Fatal(e)
			}
		}
	}
}

func TestI40SubtitleCatalogVersionsAndEpisodeScope(t *testing.T) {
	f := newPlaybackFixture(t)
	repo := subtitles.Repository{DB: f.pool, Root: t.TempDir()}
	page, e := repo.Catalog(f.ctx, f.a.asset, "")
	if e != nil || len(page.Versions) != 2 || len(page.Groups) != 2 || page.SelectedAssetID != f.a.asset {
		t.Fatal(page, e)
	}
	if _, e = repo.Catalog(f.ctx, f.a.asset, f.other.asset); !errors.Is(e, subtitles.ErrInvalid) {
		t.Fatal("cross-work choice accepted", e)
	}
	work := f.text(t, `SELECT media_id::text FROM media_asset_bindings WHERE asset_id=$1`, f.a.asset)
	f.exec(t, `UPDATE media_items SET media_type='series' WHERE id=$1`, work)
	f.exec(t, `UPDATE media_asset_bindings SET season=1,episode=CASE WHEN asset_id=$1 THEN 1 ELSE 2 END WHERE media_id=$2`, f.a.asset, work)
	f.probeSnapshot(t, f.a, 600000000, 1920)
	seasons, err := (managedcatalog.Engine{DB: f.pool}).ListParent(f.ctx, "managed", "", "series", "series_"+work, 1, 25)
	if err != nil || len(seasons.Items) != 1 || seasons.Items[0].EpisodeCount == nil || *seasons.Items[0].EpisodeCount != 2 || seasons.Items[0].FileSize == nil {
		t.Fatal("season aggregates", seasons, err)
	}
	episodes, err := (managedcatalog.Engine{DB: f.pool}).ListParent(f.ctx, "managed", "", "series", "season_"+work+"_1", 1, 25)
	if err != nil || len(episodes.Items) != 2 {
		t.Fatal("episode catalogue", episodes, err)
	}
	found := false
	for _, episode := range episodes.Items {
		if episode.ID == f.a.asset {
			found = episode.Resolution == "1080p" && episode.FileSize != nil
		}
	}
	if !found {
		t.Fatal("persisted probe resolution missing", episodes)
	}
	page, e = repo.Catalog(f.ctx, "series_"+work, "")
	if e != nil || len(page.Versions) != 2 || len(page.Groups) != 0 {
		t.Fatal(page, e)
	}
	page, e = repo.Catalog(f.ctx, "season_"+work+"_1", f.a.asset)
	if e != nil || len(page.Groups) != 1 || page.Groups[0].AssetID != f.a.asset {
		t.Fatal("episode leaked", page, e)
	}
	if _, e = repo.Catalog(f.ctx, "season_"+work+"_2", f.a.asset); !errors.Is(e, subtitles.ErrInvalid) {
		t.Fatal("cross-season choice accepted", e)
	}
}
