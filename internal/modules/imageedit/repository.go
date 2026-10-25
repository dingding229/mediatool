package imageedit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadataedit"
	"media-tool/internal/platform/artifacts"
	cleanup "media-tool/internal/platform/cleanup"
	database "media-tool/internal/platform/database"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Repository struct {
	DB   *pgxpool.Pool
	Root string
}
type Image struct {
	ID  string `json:"id,omitempty"`
	Tag string `json:"tag,omitempty"`
	URL string `json:"url"`
}
type Record struct {
	Target    metadataedit.Target   `json:"target"`
	TargetKey string                `json:"targetKey"`
	Revision  int64                 `json:"revision"`
	Base      map[string]Image      `json:"base"`
	Effective map[string]Image      `json:"effective"`
	Overrides map[string]string     `json:"overrides"`
	Matched   bool                  `json:"matched"`
	Unmatched []metadataedit.Target `json:"unmatched"`
	Choices   []metadataedit.Target `json:"choices"`
}
type Change struct {
	TargetKey string `json:"targetKey"`
	Revision  int64  `json:"revision"`
	Kind      string `json:"kind"`
}
type query interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func ValidKind(k string) bool { return k == "Primary" || k == "Backdrop" || k == "Logo" }
func (r Repository) root() string {
	if r.Root != "" {
		return r.Root
	}
	return DefaultRoot
}
func targetID(t metadataedit.Target) string {
	s := t.Kind + "_" + t.MediaID
	if t.Kind == "season" || t.Kind == "episode" {
		s += fmt.Sprintf("_%d", t.Season)
	}
	if t.Kind == "episode" {
		s += fmt.Sprintf("_%d", t.Episode)
	}
	return s
}
func URL(id, kind, tag string) string {
	return "/api/v1/media/library/managed/" + url.PathEscape(id) + "/images/" + kind + "?tag=" + url.QueryEscape(tag)
}
func (r Repository) Read(ctx context.Context, id string) (Record, error) {
	if r.DB == nil {
		return Record{}, metadataedit.ErrUnavailable
	}
	t, e := metadataedit.ResolveTarget(ctx, r.DB, id, false)
	if e != nil {
		return Record{}, e
	}
	out, e := read(ctx, r.DB, t)
	if e == nil && t.Kind == "episode" {
		var raw []byte
		e = r.DB.QueryRow(ctx, `SELECT CASE WHEN jsonb_array_length(episodes)>0 THEN episodes ELSE jsonb_build_array(episode) END FROM media_asset_bindings WHERE asset_id::text=$1 AND state='ready'`, id).Scan(&raw)
		if errors.Is(e, pgx.ErrNoRows) {
			e = nil
		} else if e == nil {
			var numbers []int
			e = json.Unmarshal(raw, &numbers)
			for _, n := range numbers {
				x := t
				x.Episode = n
				out.Choices = append(out.Choices, x)
			}
		}
	}
	return out, e
}
func read(ctx context.Context, q query, t metadataedit.Target) (out Record, err error) {
	out = Record{Target: t, TargetKey: t.Key(), Base: map[string]Image{}, Effective: map[string]Image{}, Overrides: map[string]string{}, Unmatched: []metadataedit.Target{}, Choices: []metadataedit.Target{}}
	var images, root, season, ep []byte
	err = q.QueryRow(ctx, `SELECT coalesce(o.images,'{}'),coalesce(o.revision,0),jsonb_build_object('posterPath',coalesce(nullif(d.payload->>'posterPath',''),m.metadata->>'posterPath'),'backdropPath',coalesce(nullif(d.payload->>'backdropPath',''),m.metadata->>'backdropPath'),'logoPath',d.payload->>'logoPath'),coalesce(sd.payload,'{}'),`+metadataedit.LeafSQL("sd.payload", "a.media_identity", "$3::int", "$4::int")+`,EXISTS(SELECT 1 FROM media_asset_bindings b WHERE b.media_id=m.id AND b.state='ready' AND ($2 IN('movie','series') OR b.season=$3 AND ($2='season' OR b.episode=$4 OR b.episodes @> to_jsonb($4::int)))) FROM media_items m
 LEFT JOIN media_image_overrides o ON o.media_id=m.id AND o.entity_type=$2 AND o.season_number=$3 AND o.episode_number=$4
 LEFT JOIN native_tmdb_details d ON d.tmdb_type=m.media_type AND d.tmdb_id=m.tmdb_id AND d.season_number IS NULL AND d.fetched_at IS NOT NULL AND d.language=`+locale+`
 LEFT JOIN native_tmdb_details sd ON sd.tmdb_type='season' AND sd.tmdb_id=m.tmdb_id AND sd.season_number=$3 AND sd.fetched_at IS NOT NULL AND sd.language=`+locale+`
 LEFT JOIN LATERAL(SELECT a.media_identity FROM media_asset_bindings b JOIN media_assets a ON a.id=b.asset_id WHERE b.media_id=m.id AND b.state='ready' AND b.season=$3 AND(b.episode=$4 OR b.episodes @> to_jsonb($4::int)) ORDER BY a.created_at DESC,a.id LIMIT 1)a ON true WHERE m.id=$1`, t.MediaID, t.Kind, t.Season, t.Episode).Scan(&images, &out.Revision, &root, &season, &ep, &out.Matched)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(images, &out.Overrides); err != nil {
		return out, err
	}
	var d, sd, ed map[string]json.RawMessage
	for _, v := range []struct {
		raw []byte
		to  *map[string]json.RawMessage
	}{{root, &d}, {season, &sd}, {ep, &ed}} {
		if err = json.Unmarshal(v.raw, v.to); err != nil {
			return out, err
		}
	}
	text := func(m map[string]json.RawMessage, k string) string {
		var s string
		_ = json.Unmarshal(m[k], &s)
		return s
	}
	paths := map[string]string{"Primary": text(d, "posterPath"), "Backdrop": text(d, "backdropPath"), "Logo": text(d, "logoPath")}
	if t.Kind == "season" || t.Kind == "episode" {
		if p := text(sd, "posterPath"); p != "" {
			paths["Primary"] = p
		}
	}
	if t.Kind == "episode" {
		if p := text(ed, "stillPath"); p != "" {
			paths["Primary"] = p
		}
	}
	for k, p := range paths {
		if p != "" {
			out.Base[k] = Image{URL: "/api/v1/media/tmdb/image?path=" + url.QueryEscape(p)}
			out.Effective[k] = out.Base[k]
		}
	}
	rows, err := q.Query(ctx, `SELECT f.id::text,f.digest,k.key FROM media_image_overrides o CROSS JOIN LATERAL jsonb_each_text(o.images) k JOIN media_image_files f ON f.id=k.value::uuid WHERE o.media_id=$1 AND o.entity_type=$2 AND o.season_number=$3 AND o.episode_number=$4`, t.MediaID, t.Kind, t.Season, t.Episode)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var im Image
		var k string
		if err = rows.Scan(&im.ID, &im.Tag, &k); err != nil {
			rows.Close()
			return out, err
		}
		im.URL = URL(targetID(t), k, im.Tag)
		out.Effective[k] = im
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = q.Query(ctx, `SELECT o.media_id::text,o.entity_type,o.season_number,o.episode_number FROM media_image_overrides o WHERE o.media_id=$1 AND o.images<>'{}' AND NOT EXISTS(SELECT 1 FROM media_asset_bindings b WHERE b.media_id=o.media_id AND b.state='ready' AND(o.entity_type IN('movie','series') OR b.season=o.season_number AND(o.entity_type='season' OR b.episode=o.episode_number OR b.episodes @> to_jsonb(o.episode_number)))) ORDER BY o.entity_type,o.season_number,o.episode_number LIMIT 50`, t.MediaID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var u metadataedit.Target
		if err = rows.Scan(&u.MediaID, &u.Kind, &u.Season, &u.Episode); err != nil {
			break
		}
		out.Unmatched = append(out.Unmatched, u)
	}
	e := rows.Err()
	rows.Close()
	if err == nil {
		err = e
	}
	return out, err
}

const locale = `coalesce((SELECT nullif(settings->>'language','')FROM integrations WHERE kind='tmdb' AND enabled ORDER BY created_at LIMIT 1),'zh-CN')`

// Save serializes reference publication and collection across app processes. Pixel
// work and remote downloads happen before this short critical section.
func (r Repository) Save(ctx context.Context, id, actor string, c Change, pic *Normalized, source string) (Record, error) {
	if !ValidKind(c.Kind) || c.Revision < 0 {
		return Record{}, ErrInvalid
	}
	if r.DB == nil {
		return Record{}, metadataedit.ErrUnavailable
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return Record{}, e
	}
	createdFile := ""
	commitAttempted := false
	defer func() {
		cleanup.Rollback(ctx, tx)
		if createdFile != "" && !commitAttempted {
			cleanup.Report(r.remove(createdFile), "uncommitted image cleanup")
		}
	}()
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(840035)`); e != nil {
		return Record{}, e
	}
	t, e := metadataedit.ResolveTarget(ctx, tx, id, true)
	if e != nil {
		return Record{}, e
	}
	if c.TargetKey != t.Key() {
		return Record{}, metadataedit.ErrConflict
	}
	_, e = tx.Exec(ctx, `INSERT INTO media_image_overrides(media_id,entity_type,season_number,episode_number)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, t.MediaID, t.Kind, t.Season, t.Episode)
	if e != nil {
		return Record{}, e
	}
	var before []byte
	var rev int64
	e = tx.QueryRow(ctx, `SELECT images,revision FROM media_image_overrides WHERE media_id=$1 AND entity_type=$2 AND season_number=$3 AND episode_number=$4 FOR UPDATE`, t.MediaID, t.Kind, t.Season, t.Episode).Scan(&before, &rev)
	if e != nil {
		return Record{}, e
	}
	if rev != c.Revision {
		return Record{}, metadataedit.ErrConflict
	}
	var values map[string]string
	if e = json.Unmarshal(before, &values); e != nil {
		return Record{}, e
	}
	previousID := values[c.Kind]
	if pic == nil {
		delete(values, c.Kind)
	} else {
		if source != "upload" && source != "tmdb" {
			return Record{}, ErrInvalid
		}
		fid := jobs.UUID()
		filename := fid + "." + pic.Extension
		sum := sha256.Sum256(pic.Body)
		digest := hex.EncodeToString(sum[:])
		if e = r.ensureRoot(); e != nil {
			return Record{}, e
		}
		if e = artifacts.Create(r.root(), filename, pic.Body); e != nil {
			return Record{}, e
		}
		createdFile = filename
		// Retain on an ambiguous commit. Scoped collection reconciles this copy later.
		_, e = tx.Exec(ctx, `INSERT INTO media_image_files(id,digest,filename,content_type,width,height,size_bytes,source)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, fid, digest, filename, pic.ContentType, pic.Width, pic.Height, len(pic.Body), source)
		if e != nil {
			return Record{}, e
		}
		values[c.Kind] = fid
	}
	after, e := json.Marshal(values)
	if e != nil {
		return Record{}, e
	}
	_, e = tx.Exec(ctx, `UPDATE media_image_overrides SET images=$5,revision=revision+1,updated_at=now(),updated_by=$6 WHERE media_id=$1 AND entity_type=$2 AND season_number=$3 AND episode_number=$4`, t.MediaID, t.Kind, t.Season, t.Episode, after, actor)
	if e != nil {
		return Record{}, e
	}
	audit, e := json.Marshal(map[string]any{"target": t, "before": json.RawMessage(before), "after": values, "revision": rev + 1})
	if e != nil {
		return Record{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES($1,'media.images.save',$2)`, actor, audit); e != nil {
		return Record{}, e
	}
	out, e := read(ctx, tx, t)
	if e != nil {
		return out, e
	}
	commitAttempted = true
	if e = database.Commit(ctx, tx); e != nil {
		return out, e
	}
	if previousID != "" {
		cleanup.Report(r.Collect(ctx, previousID), "local image reference collection")
	}
	return out, nil
}
func (r Repository) ensureRoot() error {
	root := r.root()
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || filepath.Base(root) != "images" || filepath.Base(filepath.Dir(root)) != "native-media" {
		return ErrInvalid
	}
	if e := os.MkdirAll(root, 0750); e != nil {
		return e
	}
	st, e := os.Lstat(root)
	if e != nil {
		return e
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return ErrInvalid
	}
	return nil
}
func (r Repository) File(ctx context.Context, id string) ([]byte, string, string, error) {
	var filename, ct, tag string
	e := r.DB.QueryRow(ctx, `SELECT filename,content_type,digest FROM media_image_files WHERE id=$1`, id).Scan(&filename, &ct, &tag)
	if e != nil {
		return nil, "", "", e
	}
	if !safeFile(filename) {
		return nil, "", "", ErrInvalid
	}
	root, e := os.OpenRoot(r.root())
	if e != nil {
		return nil, "", "", e
	}
	defer cleanup.Close(root, "image root")
	f, e := root.Open(filename)
	if e != nil {
		return nil, "", "", e
	}
	defer cleanup.Close(f, "local image")
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() {
		return nil, "", "", ErrInvalid
	}
	body, e := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if e != nil || len(body) > MaxBytes {
		return nil, "", "", ErrInvalid
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != tag {
		return nil, "", "", ErrInvalid
	}
	return body, ct, tag, nil
}
func safeFile(s string) bool {
	return filepath.Base(s) == s && !strings.ContainsAny(s, "/\\\x00") && (strings.HasSuffix(s, ".jpg") || strings.HasSuffix(s, ".png")) && validUUID(strings.TrimSuffix(strings.TrimSuffix(s, ".jpg"), ".png"))
}

// Collect only removes unreferenced registered copies, plus old unpublished files
// in this program's exact directory. The same DB lock protects concurrent Save.
func (r Repository) Collect(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer cleanup.Rollback(ctx, tx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(840035)`); e != nil {
		return e
	}
	rows, e := tx.Query(ctx, `SELECT f.id::text,f.filename FROM media_image_files f WHERE f.id=ANY($1::uuid[]) AND NOT EXISTS(SELECT 1 FROM media_image_overrides o WHERE o.images @> jsonb_build_object('Primary',f.id::text) OR o.images @> jsonb_build_object('Backdrop',f.id::text) OR o.images @> jsonb_build_object('Logo',f.id::text)) LIMIT 100`, ids)
	if e != nil {
		return e
	}
	type pair struct{ id, name string }
	var dead []pair
	for rows.Next() {
		var p pair
		if e = rows.Scan(&p.id, &p.name); e != nil {
			rows.Close()
			return e
		}
		dead = append(dead, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if len(dead) == 0 {
		return nil
	}
	root, e := os.OpenRoot(r.root())
	if e != nil {
		return e
	}
	defer cleanup.Close(root, "image GC root")
	for _, p := range dead {
		if !safeFile(p.name) {
			return ErrInvalid
		}
		if e = root.Remove(p.name); e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if _, e = tx.Exec(ctx, `DELETE FROM media_image_files WHERE id=$1`, p.id); e != nil {
			return e
		}
	}
	return database.Commit(ctx, tx)
}
