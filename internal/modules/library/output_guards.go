package library

import (
	"context"
	"github.com/jackc/pgx/v5"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"regexp"
)

// Shared UUID validation is also used by STRM repair, upload recovery and grants.
var archiveUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ValidArchiveID(id string) bool { return archiveUUID.MatchString(id) }

// Output recovery must verify current STRM bytes, not only a database ready flag.
func activeArchiveOutputs(ctx context.Context, tx pgx.Tx, asset, root string) error {
	rows, err := tx.Query(ctx, `SELECT s.output_path,m.content_digest,m.owner_scope,m.asset_id::text FROM strm_entries s LEFT JOIN managed_media_artifacts m ON m.relative_path=s.output_path AND m.artifact_kind='strm' WHERE s.asset_id=$1 ORDER BY s.output_path`, asset)
	if err != nil {
		return err
	}
	type output struct {
		p            string
		digest       []byte
		scope, owner *string
	}
	list := []output{}
	for rows.Next() {
		var o output
		if err = rows.Scan(&o.p, &o.digest, &o.scope, &o.owner); err != nil {
			rows.Close()
			return err
		}
		list = append(list, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(list) == 0 || len(list) > 100 {
		return ErrArtifactDependency
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer deferutil.Close(fs, "Read/descriptor teardown only; primary operations handle their own I/O result")
	for _, o := range list {
		if o.scope == nil || o.owner == nil || *o.scope != "asset:"+asset || *o.owner != asset {
			return ErrArtifactDependency
		}
		raw, err := readRegular(fs, o.p)
		if err != nil || !digestEqual(raw, o.digest) {
			return ErrArtifactDependency
		}
	}
	return nil
}
