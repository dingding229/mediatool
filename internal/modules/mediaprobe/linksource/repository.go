package linksource

import (
	"context"
	"errors"
	"media-tool/internal/modules/strm"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/nativelibrary/catalog"
)

// SQLRepository performs bounded read-only evidence lookups using precisely the
// catalog's visibility predicate. Missing ownership/digest fails closed in Fetch.
type SQLRepository struct{ DB *pgxpool.Pool }

func (r SQLRepository) Load(ctx context.Context, asset string) (Evidence, error) {
	var e Evidence
	err := r.DB.QueryRow(ctx, `WITH visible AS (`+catalog.ExposableAssetsSQL+`)
 SELECT v.asset_id::text,f.full_path,coalesce(f.pick_code,''),s.output_path,f.size_bytes,v.revision,a.content_digest
 FROM visible v JOIN cloud_files f ON f.id=v.cloud_file_id
 JOIN strm_entries s ON s.asset_id=v.asset_id AND s.output_path<>'' AND lower(s.output_path) LIKE '%.strm'
 LEFT JOIN managed_media_artifacts a ON a.asset_id=v.asset_id AND a.relative_path=s.output_path AND a.artifact_kind='strm' AND a.owner_scope='asset:'||v.asset_id::text
 WHERE v.asset_id=$1 ORDER BY s.output_path LIMIT 1`, asset).Scan(&e.AssetID, &e.Path, &e.PickCode, &e.Output, &e.Size, &e.Revision, &e.Digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, fail(Permanent, NotExposable, 0)
	}
	if err != nil {
		return e, classify(ctx, err)
	}
	return e, nil
}

// RootReader only reads existing regular, owned STRM files. Never repair,
// recreate, rewrite or rotate a grant when a file is missing or has changed.
type RootReader struct{ Root string }

func (r RootReader) Read(ctx context.Context, relative string) ([]byte, error) {
	body, err := strm.ReadOwned(ctx, r.Root, relative)
	if errors.Is(err, strm.ErrUnsafeBody) {
		return nil, fail(Manual, Mismatch, 0)
	}
	if err != nil {
		return nil, fileError(ctx, err)
	}
	return body, nil
}
func fileError(ctx context.Context, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fail(Permanent, FileMissing, 0)
	}
	return classify(ctx, err)
}
