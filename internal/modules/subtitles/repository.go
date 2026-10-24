package subtitles

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/mediaprobe/probeinfo"
	deferutil "media-tool/internal/platform/cleanup"
	uuidcheck "media-tool/internal/platform/identity"
	"path"
	"strconv"
	"strings"
)

type Repository struct {
	DB   *pgxpool.Pool
	Root string
}

// This is the original B3 same-directory/name-boundary query; both native
// provider resolution and the registry discovery use this single definition.
const AutoSQL = `SELECT f.id::text,f.full_path,coalesce(f.pick_code,''),f.size_bytes,f.updated_at FROM media_assets a JOIN cloud_files v ON v.id=a.cloud_file_id JOIN cloud_files f ON f.storage_id=v.storage_id AND regexp_replace(f.full_path,'/[^/]+$','')=regexp_replace(v.full_path,'/[^/]+$','') WHERE a.id=$1::uuid AND f.state='present' AND f.id<>v.id AND ('.'||lower(regexp_replace(f.full_path,'^.*\.','')))=ANY($3::text[]) AND left(lower(f.full_path),length(regexp_replace(v.full_path,'\.[^/.]+$',''))+1)=lower(regexp_replace(v.full_path,'\.[^/.]+$',''))||'.' AND ($2::text='' OR f.id::text=$2) ORDER BY f.full_path,f.id LIMIT 201`
const columns = `id::text,asset_id::text,source,source_key,stream_index,filename,extension,storage_path,language,title,is_default,is_forced,hidden,overridden,digest,size_bytes,revision,deleting`

func dest(v *Entry) []any {
	return []any{&v.ID, &v.AssetID, &v.Source, &v.SourceKey, &v.Index, &v.Filename, &v.Extension, &v.StoragePath, &v.Language, &v.Title, &v.Default, &v.Forced, &v.Hidden, &v.Overridden, &v.Digest, &v.Size, &v.Revision, &v.Deleting}
}

type rowReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readEntry(ctx context.Context, q rowReader, asset, id string, lock bool) (Entry, error) {
	var v Entry
	if !uuidcheck.UUID(asset) || !uuidcheck.UUID(id) {
		return v, ErrInvalid
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	e := q.QueryRow(ctx, `SELECT `+columns+` FROM managed_subtitles WHERE asset_id=$1 AND id=$2`+suffix, asset, id).Scan(dest(&v)...)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, ErrUnavailable
	}
	return v, nil
}
func (r Repository) Get(ctx context.Context, asset, id string) (Entry, error) {
	if r.DB == nil {
		return Entry{}, ErrUnavailable
	}
	return readEntry(ctx, r.DB, asset, id, false)
}
func (r Repository) asset(ctx context.Context, id string) error {
	if !uuidcheck.UUID(id) {
		return ErrInvalid
	}
	if r.DB == nil {
		return ErrUnavailable
	}
	var ok bool
	e := r.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_assets a JOIN media_asset_bindings b ON b.asset_id=a.id JOIN media_items m ON m.id=b.media_id WHERE a.id=$1 AND (m.media_type='movie' OR m.media_type='series' AND b.episode IS NOT NULL))`, id).Scan(&ok)
	if e != nil {
		return ErrUnavailable
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}
func (r Repository) Save(ctx context.Context, asset, id string, m Metadata) (Entry, error) {
	if r.DB == nil {
		return Entry{}, ErrUnavailable
	}
	if e := m.Validate(); e != nil {
		return Entry{}, e
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return Entry{}, ErrUnavailable
	}
	defer deferutil.Rollback(ctx, tx)
	v, e := readEntry(ctx, tx, asset, id, true)
	if e != nil {
		return v, e
	}
	if v.Revision != m.Revision {
		return v, ErrConflict
	}
	if v.Deleting || v.Source == "embedded" && m.Hidden {
		return v, ErrInvalid
	}
	v.Metadata = m
	v.Overridden = true
	e = tx.QueryRow(ctx, `UPDATE managed_subtitles SET language=$3,title=$4,is_default=$5,is_forced=$6,hidden=$7,overridden=true,revision=revision+1 WHERE asset_id=$1 AND id=$2 RETURNING revision`, asset, id, m.Language, m.Title, m.Default, m.Forced, m.Hidden).Scan(&v.Revision)
	if e != nil {
		return v, ErrUnavailable
	}
	if e = audit(ctx, tx, "metadata_saved", asset, id); e != nil {
		return v, ErrUnavailable
	}
	if e = tx.Commit(ctx); e != nil {
		return v, ErrUnavailable
	}
	return v, nil
}
func key(v Entry) string { return v.AssetID + ":" + v.Source + ":" + v.SourceKey }

// Groups does bounded batch discovery and only inserts previously unseen
// identities. nextval is never used for existing rows and indices never recycle.
func (r Repository) Groups(ctx context.Context, assets []string, extensions []string) (map[string][]Entry, error) {
	out := map[string][]Entry{}
	if r.DB == nil {
		return out, ErrUnavailable
	}
	if len(assets) == 0 {
		return out, nil
	}
	if len(assets) > 100 {
		return out, ErrInvalid
	}
	for _, id := range assets {
		if !uuidcheck.UUID(id) {
			return out, ErrInvalid
		}
	}
	sql := strings.Replace(AutoSQL, "SELECT f.id::text,", "SELECT a.id::text,v.full_path,f.id::text,", 1)
	sql = strings.Replace(sql, "a.id=$1::uuid", "a.id=ANY($1::uuid[])", 1)
	sql = strings.Replace(sql, "ORDER BY f.full_path,f.id LIMIT 201", "ORDER BY a.id,f.full_path,f.id LIMIT 20001", 1)
	rows, e := r.DB.Query(ctx, sql, assets, "", extensions)
	if e != nil {
		return out, ErrUnavailable
	}
	candidates := map[string]Entry{}
	counts := map[string]int{}
	for rows.Next() {
		var v Entry
		var video, full, pick string
		var size int64
		var at any
		if e = rows.Scan(&v.AssetID, &video, &v.SourceKey, &full, &pick, &size, &at); e != nil {
			rows.Close()
			return out, ErrUnavailable
		}
		if !Matches(video, full) {
			continue
		}
		v.Source = "automatic"
		v.Filename = bounded(path.Base(full), 256)
		v.Extension = strings.TrimPrefix(strings.ToLower(path.Ext(full)), ".")
		v.Metadata = Infer(v.Filename)
		candidates[key(v)] = v
		counts[v.AssetID]++
		if counts[v.AssetID] > 200 {
			rows.Close()
			return out, ErrUnavailable
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, ErrUnavailable
	}
	rows, e = r.DB.Query(ctx, `WITH snapshots AS (SELECT a.id,p.result,octet_length(p.result::text) bytes FROM media_assets a JOIN media_probe_results p ON p.cloud_file_id=a.cloud_file_id WHERE a.id=ANY($1::uuid[]) AND p.probed_at IS NOT NULL AND p.result<>'{}'::jsonb) SELECT id::text,CASE WHEN sum(bytes) OVER()<=33554432 THEN result ELSE NULL END FROM snapshots`, assets)
	if e != nil {
		return out, ErrUnavailable
	}
	for rows.Next() {
		var asset string
		var raw []byte
		if e = rows.Scan(&asset, &raw); e != nil {
			rows.Close()
			return out, ErrUnavailable
		}
		if raw == nil {
			rows.Close()
			return out, ErrUnavailable
		}
		var p probeinfo.Result
		if len(raw) > 4<<20 || json.Unmarshal(raw, &p) != nil || p.Validate() != nil {
			continue
		}
		for _, stream := range p.Streams {
			if stream.Type != "Subtitle" {
				continue
			}
			v := Entry{AssetID: asset, Source: "embedded", SourceKey: strconv.Itoa(stream.Index), Index: stream.Index, Filename: bounded(probeinfo.DisplayTitle(stream), 256), Extension: stream.Codec, Metadata: Metadata{Language: bounded(stream.Language, 16), Title: bounded(probeinfo.DisplayTitle(stream), 200), Default: stream.IsDefault, Forced: stream.IsForced, Revision: 1}}
			candidates[key(v)] = v
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, ErrUnavailable
	}
	type seed struct {
		Asset, Source, Key, Name, Ext, Language, Title string
		Index                                          int
		Default, Forced                                bool
	}
	seeds := []seed{}
	for _, v := range candidates {
		seeds = append(seeds, seed{v.AssetID, v.Source, v.SourceKey, v.Filename, v.Extension, v.Language, v.Title, v.Index, v.Default, v.Forced})
	}
	raw, _ := json.Marshal(seeds)
	if _, e = r.DB.Exec(ctx, `INSERT INTO managed_subtitles(asset_id,source,source_key,filename,extension,language,title,is_default,is_forced,stream_index)
 SELECT (v->>'Asset')::uuid,v->>'Source',v->>'Key',v->>'Name',v->>'Ext',v->>'Language',v->>'Title',(v->>'Default')::boolean,(v->>'Forced')::boolean,
 CASE WHEN v->>'Source'='embedded' THEN (v->>'Index')::integer ELSE nextval('managed_subtitle_index') END
 FROM jsonb_array_elements($1::jsonb) v WHERE NOT EXISTS(SELECT 1 FROM managed_subtitles s WHERE s.asset_id=(v->>'Asset')::uuid AND s.source=v->>'Source' AND s.source_key=v->>'Key')
 ORDER BY v->>'Asset',v->>'Source',v->>'Key' ON CONFLICT DO NOTHING`, raw); e != nil {
		return out, ErrUnavailable
	}
	rows, e = r.DB.Query(ctx, `WITH wanted AS(SELECT (v->>'Asset')::uuid a,v->>'Source' kind,v->>'Key' k FROM jsonb_array_elements($2::jsonb) v)
 SELECT `+columns+` FROM managed_subtitles WHERE asset_id=ANY($1::uuid[]) AND source='uploaded'
 UNION ALL SELECT m.`+strings.ReplaceAll(columns, ",", ",m.")+` FROM managed_subtitles m JOIN wanted w ON m.asset_id=w.a AND m.source=w.kind AND m.source_key=w.k ORDER BY asset_id,stream_index`, assets, raw)
	if e != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var v Entry
		if e = rows.Scan(dest(&v)...); e != nil {
			return out, ErrUnavailable
		}
		if v.Source != "uploaded" {
			base, ok := candidates[key(v)]
			if !ok {
				continue
			}
			v.Filename = base.Filename
			v.Extension = base.Extension
			if !v.Overridden {
				revision := v.Revision
				v.Metadata = base.Metadata
				v.Revision = revision
			}
		}
		out[v.AssetID] = append(out[v.AssetID], v)
	}
	if rows.Err() != nil {
		return out, ErrUnavailable
	}
	return out, nil
}
