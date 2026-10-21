package migrations_test

import (
	"context"
	nativecatalog "media-tool/internal/modules/nativelibrary/catalog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	managed "media-tool/internal/modules/catalog"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativelibrary"
)

type b3QueryTrace struct {
	t    *testing.T
	sql  string
	args []any
}

func (tr *b3QueryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if (strings.HasPrefix(d.SQL, "WITH entries AS") && strings.Contains(d.SQL, " LIMIT ")) || strings.Contains(d.SQL, ",filtered AS NOT MATERIALIZED(SELECT * FROM item_rows WHERE") {
		tr.sql = d.SQL
		tr.args = append([]any{}, d.Args...)
	}
	return ctx
}
func (tr *b3QueryTrace) TraceQueryEnd(_ context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	if d.Err != nil {
		tr.t.Log(d.Err)
	}
}
func TestB3TenThousandEpisodesParentPagingUsesIndex(t *testing.T) {
	f := newBrowseFixture(t)
	if _, e := f.provider.UpTo(f.ctx, 74); e != nil {
		t.Fatal(e)
	}
	seedBrowseMovies(t, f, 10000)
	show := jobs.UUID()
	f.exec(t, `INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'series',1999999,'B3 10000 episodes','{"officialRating":"G"}')`, show)
	f.exec(t, `UPDATE media_asset_bindings b SET media_id=$1,season=(i-1)/100+1,episode=(i-1)%100+1,episodes=jsonb_build_array((i-1)%100+1) FROM native_browse_seed s WHERE s.asset=b.asset_id`, show)
	f.exec(t, `UPDATE native_libraries SET collection_type='tvshows' WHERE id='g:test-a'`)
	if _, e := f.pool.Exec(f.ctx, `ANALYZE media_asset_bindings; ANALYZE media_items; ANALYZE media_assets`); e != nil {
		t.Fatal(e)
	}
	cfg := f.pool.Config()
	trace := &b3QueryTrace{t: t}
	cfg.ConnConfig.Tracer = trace
	p, e := pgxpool.NewWithConfig(f.ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	engine := managed.Engine{DB: p}
	list := func(parent string, page int) managed.Page {
		t.Helper()
		v, e := engine.ListParent(f.ctx, "managed", "", "", parent, page, 24)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	top := list("", 1)
	if top.Total != 1 || top.Items[0].ID != "series_"+show || top.Items[0].Type != "Series" {
		t.Fatal(top)
	}
	seasons := list(top.Items[0].ID, 5)
	if seasons.Total != 100 || len(seasons.Items) != 4 || *seasons.Items[0].Season != 97 {
		t.Fatal(seasons)
	}
	episodes := list("season_"+show+"_100", 5)
	if episodes.Total != 100 || len(episodes.Items) != 4 || *episodes.Items[0].Episode != 97 || *episodes.Items[3].Episode != 100 {
		t.Fatal(episodes)
	}
	var plan string
	if e = p.QueryRow(f.ctx, "EXPLAIN (ANALYZE,FORMAT JSON) "+trace.sql, trace.args...).Scan(&plan); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(plan, "media_asset_bindings_parent_episode_idx") {
		t.Fatal("actual parent paging did not use index", plan)
	}
	t.Log("10000 episodes: actual managed season query used media_asset_bindings_parent_episode_idx")
	// Native parent browsing uses the same persisted hierarchy, not current-page grouping.
	seriesKey, _ := nativelibrary.SeriesKey(1999999)
	seasonKey, _ := nativelibrary.SeasonKey(1999999, 100)
	ids, e := nativelibrary.EnsureIDs(f.ctx, nativecatalog.IDQueries{DB: f.pool}, []nativelibrary.Key{seriesKey, seasonKey})
	if e != nil {
		t.Fatal(e)
	}
	q := nativelibrary.ItemQuery{ParentID: strconv.FormatInt(ids[seasonKey], 10), Limit: 24, StartIndex: 96, SortBy: []string{"IndexNumber"}}
	native := nativecatalog.BrowseRepository{DB: p}
	at := time.Now()
	items, total, e := native.Query(f.ctx, f.user, f.permissions(), q)
	if e != nil {
		t.Fatal(e)
	}
	t.Log("10000 episode native season page", time.Since(at))
	if e = p.QueryRow(f.ctx, "EXPLAIN (ANALYZE,FORMAT JSON) "+trace.sql, trace.args...).Scan(&plan); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(plan, "media_asset_bindings_parent_episode_idx") {
		t.Fatal("native season query did not use parent index", plan)
	}
	if total != 100 || len(items) != 4 || *items[0].IndexNumber != 97 {
		t.Fatal("native season page", total, len(items))
	}

}
