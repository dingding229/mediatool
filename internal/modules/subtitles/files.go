package subtitles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/artifacts"
	deferutil "media-tool/internal/platform/cleanup"
	uuidcheck "media-tool/internal/platform/identity"
	"os"
	"path"
)

func (r Repository) Upload(ctx context.Context, asset, name, charset string, raw []byte) (Entry, error) {
	return r.UploadID(ctx, asset, jobs.UUID(), name, charset, raw)
}
func (r Repository) UploadID(ctx context.Context, asset, id, name, charset string, raw []byte) (Entry, error) {
	if !uuidcheck.UUID(id) {
		return Entry{}, ErrInvalid
	}
	body, ext, detected, err := Normalize(name, charset, raw)
	if err != nil {
		return Entry{}, err
	}
	if err = r.asset(ctx, asset); err != nil {
		return Entry{}, err
	}
	sum := sha256.Sum256(body)
	relative := path.Join(asset, id+"."+ext)
	m := Infer(name)
	if err = m.Validate(); err != nil {
		return Entry{}, err
	}
	entry := Entry{ID: id, AssetID: asset, Source: "uploaded", SourceKey: id, Filename: path.Base(name), Extension: ext, StoragePath: relative, Metadata: m, Overridden: true, Digest: sum[:], Size: int64(len(body)), Encoding: detected}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return entry, ErrUnavailable
	}
	created, commitAttempted := false, false
	defer func() {
		if created && !commitAttempted {
			deferutil.Report(r.remove(relative), "unregistered subtitle upload cleanup")
		}
		deferutil.Rollback(ctx, tx)
	}()
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM media_assets WHERE id=$1 FOR UPDATE`, asset).Scan(&locked); err != nil {
		return entry, ErrNotFound
	}
	old, e := readEntry(ctx, tx, asset, id, false)
	if e == nil {
		if old.Source != "uploaded" || old.Deleting || old.Extension != ext || !bytes.Equal(old.Digest, sum[:]) {
			return entry, ErrConflict
		}
		old.Encoding = detected
		return old, nil
	}
	if e != ErrNotFound {
		return entry, e
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM managed_subtitles WHERE asset_id=$1 AND source='uploaded'`, asset).Scan(&count); err != nil {
		return entry, ErrUnavailable
	}
	if count >= 100 {
		return entry, ErrInvalid
	}
	if err = os.MkdirAll(r.Root, 0750); err != nil {
		return entry, ErrUnavailable
	}
	if st, e := os.Lstat(r.Root); e != nil || st.Mode()&os.ModeSymlink != 0 {
		return entry, ErrUnavailable
	}
	if err = artifacts.Create(r.Root, relative, body); err != nil {
		// A previous commit may have had an unknown outcome. Adopt only this exact
		// request's bounded, identical file; never overwrite it or another asset.
		root, e := os.OpenRoot(r.Root)
		if e != nil {
			return entry, ErrUnavailable
		}
		defer deferutil.Close(root, "subtitle retry root")
		f, e := root.Open(relative)
		if e != nil {
			return entry, ErrUnavailable
		}
		stored, e := io.ReadAll(io.LimitReader(f, MaxBytes+1))
		deferutil.Report(f.Close(), "subtitle retry file")
		if e != nil || !bytes.Equal(stored, body) {
			return entry, ErrConflict
		}
		dir, e := root.Open(asset)
		if e != nil {
			return entry, ErrUnavailable
		}
		syncErr := dir.Sync()
		deferutil.Report(dir.Close(), "subtitle retry directory")
		if syncErr != nil {
			return entry, ErrUnavailable
		}
	} else {
		created = true
	}
	err = tx.QueryRow(ctx, `INSERT INTO managed_subtitles(id,asset_id,source,source_key,filename,extension,storage_path,language,title,is_default,is_forced,overridden,digest,size_bytes)VALUES($1::uuid,$2::uuid,'uploaded',($1::uuid)::text,$3,$4,$5,$6,$7,$8,$9,true,$10,$11) RETURNING stream_index`, id, asset, entry.Filename, ext, relative, m.Language, m.Title, m.Default, m.Forced, sum[:], len(body)).Scan(&entry.Index)
	if err != nil {
		return entry, ErrUnavailable
	}
	if err = audit(ctx, tx, "uploaded", asset, id); err != nil {
		return entry, ErrUnavailable
	}
	commitAttempted = true
	if err = tx.Commit(ctx); err != nil {
		return entry, ErrUnavailable
	}
	return entry, nil
}
func (r Repository) remove(relative string) error {
	root, e := os.OpenRoot(r.Root)
	if e != nil {
		return e
	}
	defer deferutil.Close(root, "subtitle root close")
	e = root.Remove(relative)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	directory, e := root.Open(path.Dir(relative))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	defer deferutil.Close(directory, "subtitle deletion directory")
	return directory.Sync()
}
func (r Repository) Read(ctx context.Context, asset, id string) (Entry, []byte, error) {
	entry, e := r.Get(ctx, asset, id)
	if e != nil {
		return entry, nil, e
	}
	if entry.Source != "uploaded" || entry.Hidden || entry.Deleting {
		return entry, nil, ErrNotFound
	}
	// The only accepted path is this registry row's random upload filename.
	if entry.StoragePath != path.Join(entry.AssetID, entry.ID+"."+entry.Extension) || ContentType(entry.Extension) == "" {
		return entry, nil, ErrUnavailable
	}
	root, e := os.OpenRoot(r.Root)
	if e != nil {
		return entry, nil, ErrUnavailable
	}
	defer deferutil.Close(root, "subtitle root close")
	f, e := root.Open(entry.StoragePath)
	if e != nil {
		return entry, nil, ErrNotFound
	}
	defer deferutil.Close(f, "uploaded subtitle close")
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() != entry.Size {
		return entry, nil, ErrUnavailable
	}
	body, e := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	sum := sha256.Sum256(body)
	if e != nil || len(body) > MaxBytes || !bytes.Equal(sum[:], entry.Digest) {
		return entry, nil, ErrUnavailable
	}
	return entry, body, nil
}
func (r Repository) Delete(ctx context.Context, asset, id string, revision int64) error {
	if r.DB == nil {
		return ErrUnavailable
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return ErrUnavailable
	}
	defer deferutil.Rollback(ctx, tx)
	v, e := readEntry(ctx, tx, asset, id, true)
	if e != nil {
		return e
	}
	if v.Source != "uploaded" {
		return ErrInvalid
	}
	if v.Revision != revision {
		return ErrConflict
	}
	if v.StoragePath != path.Join(v.AssetID, v.ID+"."+v.Extension) {
		return ErrUnavailable
	}
	// Tombstone first: failed disk cleanup stays visible to administrators for
	// explicit retry, but can never be served to players. No provider operations.
	if _, e = tx.Exec(ctx, `UPDATE managed_subtitles SET deleting=true WHERE asset_id=$1 AND id=$2`, asset, id); e != nil {
		return ErrUnavailable
	}
	if !v.Deleting {
		if e = audit(ctx, tx, "deleted", asset, id); e != nil {
			return ErrUnavailable
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return ErrUnavailable
	}
	if e = r.remove(v.StoragePath); e != nil {
		return ErrUnavailable
	}
	_, e = r.DB.Exec(ctx, `DELETE FROM managed_subtitles WHERE asset_id=$1 AND id=$2 AND deleting`, asset, id)
	if e != nil {
		return ErrUnavailable
	}
	return nil
}
