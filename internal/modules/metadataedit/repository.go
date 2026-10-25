package metadataedit

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/identity"
)

type Repository struct{ DB *pgxpool.Pool }
type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// Resolve accepts existing managed identities, plus explicit numbered identities
// for retained unmatched overlays. A save locks the binding against correction.
func resolve(ctx context.Context, q querier, id string, lock bool) (t Target, err error) {
	t.Season, t.Episode = -1, -1
	if identity.UUID(id) {
		suffix := ""
		if lock {
			suffix = " FOR SHARE OF b"
		}
		err = q.QueryRow(ctx, `SELECT m.id::text,m.media_type,coalesce(b.season,-1),CASE WHEN m.media_type='series' THEN coalesce((SELECT min(value::int)FROM jsonb_array_elements_text(b.episodes)),b.episode,-1) ELSE -1 END FROM media_asset_bindings b JOIN media_items m ON m.id=b.media_id WHERE b.asset_id=$1 AND b.state='ready'`+suffix, id).Scan(&t.MediaID, &t.Kind, &t.Season, &t.Episode)
		if t.Kind == "series" {
			t.Kind = "episode"
		} else if t.Kind == "movie" {
			t.Season, t.Episode = -1, -1
		}
	} else {
		parts := strings.Split(id, "_")
		if len(parts) < 2 || !identity.UUID(parts[1]) {
			return t, ErrNotFound
		}
		t.MediaID, t.Kind = parts[1], parts[0]
		want := map[string]int{"movie": 2, "series": 2, "season": 3, "episode": 4}[t.Kind]
		if want == 0 || len(parts) != want {
			return t, ErrNotFound
		}
		if want >= 3 {
			t.Season, err = strconv.Atoi(parts[2])
			if err != nil || t.Season < 0 || t.Season > 10000 {
				return t, ErrNotFound
			}
		}
		if want == 4 {
			t.Episode, err = strconv.Atoi(parts[3])
			if err != nil || t.Episode < 1 || t.Episode > 10000 {
				return t, ErrNotFound
			}
		}
		root := "series"
		if t.Kind == "movie" {
			root = "movie"
		}
		var found bool
		err = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_items WHERE id=$1 AND media_type=$2)`, t.MediaID, root).Scan(&found)
		if err == nil && !found {
			return t, ErrNotFound
		}
	}
	if errors.Is(err, pgx.ErrNoRows) || t.Kind == "episode" && (t.Season < 0 || t.Episode < 1) {
		return t, ErrNotFound
	}
	return t, err
}

const locale = `coalesce((SELECT nullif(settings->>'language','')FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN')`

// LeafSQL prefers the fetched season episode; the confirmed asset snapshot is a
// fallback, never an arbitrary filename. Used by managed and native SQL readers.
func LeafSQL(sd, ingest, season, episode string) string {
	return `coalesce((SELECT v FROM jsonb_array_elements(coalesce(` + sd + `->'episodes','[]'::jsonb))v WHERE v->>'number'=(` + episode + `)::text LIMIT 1),(SELECT v FROM jsonb_array_elements(coalesce(` + ingest + `->'episodeDetails','[]'::jsonb))v WHERE v->>'season'=(` + season + `)::text AND v->>'number'=(` + episode + `)::text LIMIT 1),'{}'::jsonb)`
}

const matchedSQL = `EXISTS(SELECT 1 FROM media_asset_bindings b WHERE b.media_id=o.media_id AND b.state='ready' AND (o.entity_type IN('movie','series') OR b.season=o.season_number AND (o.entity_type='season' OR o.episode_number=b.episode OR b.episodes @> to_jsonb(o.episode_number))))`

func read(ctx context.Context, q querier, t Target, orphanPages ...int) (r Record, err error) {
	r.Target, r.TargetKey, r.Overrides, r.Unmatched = t, t.Key(), Values{}, []Target{}
	base := BaseSQL("m.title", "m.release_year", "m.metadata", "$2::text", "$3::int", "$4::int", "d.payload", "sd.payload", LeafSQL("sd.payload", "a.media_identity", "$3::int", "$4::int"))
	var raw, over []byte
	err = q.QueryRow(ctx, `SELECT `+base+`,coalesce(o.overrides,'{}'),coalesce(o.revision,0),EXISTS(SELECT 1 FROM media_asset_bindings b WHERE b.media_id=m.id AND b.state='ready' AND ($2 IN('movie','series') OR b.season=$3 AND ($2='season' OR b.episode=$4 OR b.episodes @> to_jsonb($4::int)))) FROM media_items m
 LEFT JOIN native_tmdb_details d ON d.tmdb_type=m.media_type AND d.tmdb_id=m.tmdb_id AND d.season_number IS NULL AND d.fetched_at IS NOT NULL AND d.language=`+locale+`
 LEFT JOIN native_tmdb_details sd ON sd.tmdb_type='season' AND sd.tmdb_id=m.tmdb_id AND sd.season_number=$3 AND sd.fetched_at IS NOT NULL AND sd.language=`+locale+`
 LEFT JOIN LATERAL(SELECT a.media_identity FROM media_asset_bindings b JOIN media_assets a ON a.id=b.asset_id WHERE b.media_id=m.id AND b.state='ready' AND b.season=$3 AND (b.episode=$4 OR b.episodes @> to_jsonb($4::int)) ORDER BY a.created_at DESC,a.id LIMIT 1)a ON true
 `+Join("o", "m.id", "$2", "$3", "$4")+` WHERE m.id=$1`, t.MediaID, t.Kind, t.Season, t.Episode).Scan(&raw, &over, &r.Revision, &r.Matched)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(raw, &r.Base); err != nil {
		return r, err
	}
	if err = json.Unmarshal(over, &r.Overrides); err != nil {
		return r, err
	}
	// The editor exposes only supported text fields, never images/provider IDs.
	for k := range r.Base {
		if !known(k) {
			delete(r.Base, k)
		}
	}
	r.Effective = Merge(r.Base, r.Overrides)
	r.Choices = []Target{t}
	r.UnmatchedPage = 1
	if len(orphanPages) > 0 {
		r.UnmatchedPage = orphanPages[0]
	}
	if err = q.QueryRow(ctx, `SELECT count(*) FROM media_metadata_overrides o WHERE media_id=$1 AND overrides<>'{}' AND NOT (`+matchedSQL+`)`, t.MediaID).Scan(&r.UnmatchedTotal); err != nil {
		return r, err
	}
	rows, err := q.Query(ctx, `SELECT media_id::text,entity_type,season_number,episode_number FROM media_metadata_overrides o WHERE media_id=$1 AND overrides<>'{}' AND NOT (`+matchedSQL+`) ORDER BY entity_type,season_number,episode_number LIMIT 50 OFFSET $2`, t.MediaID, (r.UnmatchedPage-1)*50)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var x Target
		if err = rows.Scan(&x.MediaID, &x.Kind, &x.Season, &x.Episode); err != nil {
			return r, err
		}
		r.Unmatched = append(r.Unmatched, x)
	}
	return r, rows.Err()
}
func (r Repository) Read(ctx context.Context, id string) (Record, error) {
	return r.ReadPage(ctx, id, 1)
}
func (r Repository) ReadPage(ctx context.Context, id string, page int) (Record, error) {
	if page < 1 || page > 10000 {
		return Record{}, invalid("未匹配记录页码必须为1～10000")
	}
	if r.DB == nil {
		return Record{}, ErrUnavailable
	}
	t, e := resolve(ctx, r.DB, id, false)
	if e != nil {
		return Record{}, e
	}
	out, e := read(ctx, r.DB, t, page)
	if e != nil {
		return out, e
	}
	if identity.UUID(id) && t.Kind == "episode" {
		var raw []byte
		e = r.DB.QueryRow(ctx, `SELECT CASE WHEN jsonb_array_length(episodes)>0 THEN episodes ELSE jsonb_build_array(episode) END FROM media_asset_bindings WHERE asset_id=$1 AND state='ready'`, id).Scan(&raw)
		if e != nil {
			return out, e
		}
		var numbers []int
		if e = json.Unmarshal(raw, &numbers); e != nil {
			return out, e
		}
		out.Choices = []Target{}
		for _, n := range numbers {
			x := t
			x.Episode = n
			out.Choices = append(out.Choices, x)
		}
	}
	return out, nil
}
func (r Repository) Save(ctx context.Context, id, actor string, p Patch) (out Record, err error) {
	if r.DB == nil {
		return out, ErrUnavailable
	}
	if err = p.Validate(); err != nil {
		return out, err
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer deferutil.Rollback(ctx, tx)
	t, err := resolve(ctx, tx, id, true)
	if err != nil {
		return out, err
	}
	if p.TargetKey != t.Key() {
		return out, ErrConflict
	}
	// Keep the row even after restoring every field. Concurrent first edits are
	// serialized by the PK insert, and the revision cannot suffer an ABA reset.
	_, err = tx.Exec(ctx, `INSERT INTO media_metadata_overrides(media_id,entity_type,season_number,episode_number)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, t.MediaID, t.Kind, t.Season, t.Episode)
	if err != nil {
		return out, err
	}
	var before []byte
	var rev int64
	err = tx.QueryRow(ctx, `SELECT overrides,revision FROM media_metadata_overrides WHERE media_id=$1 AND entity_type=$2 AND season_number=$3 AND episode_number=$4 FOR UPDATE`, t.MediaID, t.Kind, t.Season, t.Episode).Scan(&before, &rev)
	if err != nil {
		return out, err
	}
	if rev != p.Revision {
		return out, ErrConflict
	}
	previous, err := read(ctx, tx, t)
	if err != nil {
		return out, err
	}
	var values Values
	if err = json.Unmarshal(before, &values); err != nil {
		return out, err
	}
	for _, k := range p.Restore {
		delete(values, k)
	}
	for k, v := range p.Set {
		values[k] = v
	}
	after, err := json.Marshal(values)
	if err != nil {
		return out, err
	}
	tag, err := tx.Exec(ctx, `UPDATE media_metadata_overrides SET overrides=$5,revision=revision+1,updated_at=now(),updated_by=$6 WHERE media_id=$1 AND entity_type=$2 AND season_number=$3 AND episode_number=$4 AND revision=$7`, t.MediaID, t.Kind, t.Season, t.Episode, after, actor, rev)
	if err != nil {
		return out, err
	}
	if tag.RowsAffected() != 1 {
		return out, ErrConflict
	}
	audit, err := json.Marshal(map[string]any{"target": t, "before": json.RawMessage(before), "after": values, "beforeEffective": previous.Effective, "afterEffective": Merge(previous.Base, values), "restore": p.Restore, "revision": rev + 1})
	if err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,'media.metadata.save',$2)`, actor, audit); err != nil {
		return out, err
	}
	out, err = read(ctx, tx, t)
	if err != nil {
		return out, err
	}
	err = dbutil.Commit(ctx, tx)
	return out, err
}
